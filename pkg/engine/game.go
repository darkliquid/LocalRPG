package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/storage"
	"gopkg.in/yaml.v3"
)

// Session is one open campaign: the resolved paths it lives under, the manifests
// that describe it, and the index its store holds. The durable record is the
// files under Paths, so closing a session loses nothing that matters.
type Session struct {
	Paths    *core.PathResolver
	Manifest *core.GameManifest
	System   *core.SystemManifest
	World    *core.WorldManifest
	Store    *storage.Store
}

// Close releases the campaign's store handle. A store a Pool handed out is
// shared, so its Close is a no-op: closing one session never closes a database
// another session is still reading.
func (s *Session) Close() error {
	if s.Store != nil {
		return s.Store.Close()
	}
	return nil
}

// InitOptions describes a campaign to create. It is a struct rather than a
// parameter list because campaign creation grows new optional fields, and a
// positional signature would break every caller each time one is added.
type InitOptions struct {
	GameID          string
	Name            string
	SystemID        string
	WorldID         string
	PlayerName      string
	PlayerDetails   string
	PlayerCharacter PlayerCharacter
	OpeningPrompt   string
}

// InitGame creates a campaign and returns it open. It validates the three IDs
// before writing anything, checks that the world's requirements suit the system,
// copies the world's template entities into the campaign, opens and syncs the
// index, pins the opening location, writes the player's note, and locks the
// content versions so that a later change to the system or world is detectable.
func InitGame(paths *core.PathResolver, opts InitOptions) (*Session, error) {
	if err := validateInitIDs(opts); err != nil {
		return nil, err
	}

	gameID := opts.GameID
	systemID := opts.SystemID
	worldID := opts.WorldID

	// Verify system & world exist, and that the world's requirements suit the system
	sysManifest, worldManifest, err := loadVerifiedContent(paths, systemID, worldID)
	if err != nil {
		return nil, err
	}

	gameDir := paths.GameDir(gameID)
	gameEntitiesDir := filepath.Join(gameDir, "entities")
	if err := createCampaignDirs(gameDir, gameEntitiesDir, filepath.Join(gameDir, "cache"), filepath.Join(gameDir, "assets")); err != nil {
		return nil, err
	}

	// Create game manifest
	// The player is stored as an entity ID, with the entered name kept beside it
	// for display. Writing the slug is what keeps the orchestrator, the JS bridge,
	// and the GUI state route agreeing on one identifier for the protagonist.
	manifest := newGameManifest(gameID, systemID, worldID, opts)

	// Copy initial template entities from world into game, named <id>.md so the
	// GUI, which derives entity IDs from file names, can reach them.
	if err := copyWorldTemplateEntities(filepath.Join(paths.WorldDir(worldID), "entities"), gameEntitiesDir); err != nil {
		return nil, err
	}

	// Open the canonical store and run initial sync
	store, err := openCampaignStore(paths, gameID, gameDir, gameEntitiesDir)
	if err != nil {
		return nil, err
	}

	// Pin the opening location so every later session agrees on where the
	// campaign begins.
	startLocation, err := pinOpeningLocation(paths, store, manifest)
	if err != nil {
		return nil, err
	}

	// Give the campaign a player note before the manifest is written, so a failure
	// leaves no half-built campaign.
	if err := ensurePlayerNote(paths, store, gameID, opts.PlayerName, opts.PlayerDetails, opts.PlayerCharacter, startLocation); err != nil {
		return nil, fmt.Errorf("create player note: %w", err)
	}

	if err := writeCampaignManifest(gameDir, manifest); err != nil {
		return nil, err
	}

	// Lock the campaign content versions and behavioural digests
	if _, err := LockContent(paths, gameID); err != nil {
		return nil, fmt.Errorf("lock content: %w", err)
	}

	return &Session{
		Paths:    paths,
		Manifest: manifest,
		System:   sysManifest,
		World:    worldManifest,
		Store:    store,
	}, nil
}

// validateInitIDs checks the three ids a campaign is created under. They become
// directory names, so they are validated before anything is written.
func validateInitIDs(opts InitOptions) error {
	if err := pathutil.ValidateID(opts.GameID); err != nil {
		return fmt.Errorf("invalid game id %q: %w", opts.GameID, err)
	}
	if err := pathutil.ValidateID(opts.SystemID); err != nil {
		return fmt.Errorf("invalid system id %q: %w", opts.SystemID, err)
	}
	if err := pathutil.ValidateID(opts.WorldID); err != nil {
		return fmt.Errorf("invalid world id %q: %w", opts.WorldID, err)
	}
	return nil
}

// loadVerifiedContent loads the system and world a campaign is created from, and
// enforces the version constraints the world places on the system.
func loadVerifiedContent(paths *core.PathResolver, systemID, worldID string) (*core.SystemManifest, *core.WorldManifest, error) {
	sysManifest, err := core.LoadSystemManifest(filepath.Join(paths.SystemDir(systemID), "system.yaml"))
	if err != nil {
		return nil, nil, fmt.Errorf("load system %q: %w", systemID, err)
	}

	worldManifest, err := core.LoadWorldManifest(filepath.Join(paths.WorldDir(worldID), "world.yaml"))
	if err != nil {
		return nil, nil, fmt.Errorf("load world %q: %w", worldID, err)
	}

	if err := checkRequires(worldManifest.Requires, systemID, worldID, sysManifest.Version); err != nil {
		return nil, nil, err
	}
	return sysManifest, worldManifest, nil
}

// createCampaignDirs makes the directories a campaign needs before anything is written
// into them.
func createCampaignDirs(dirs ...string) error {
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("create dir %q: %w", d, err)
		}
	}
	return nil
}

// newGameManifest is the campaign's own record. The player is stored as an entity ID,
// with the entered name kept beside it for display: writing the slug is what keeps the
// orchestrator, the JS bridge, and the GUI state route agreeing on one identifier for
// the protagonist.
func newGameManifest(gameID, systemID, worldID string, opts InitOptions) *core.GameManifest {
	playerID := entity.Slugify(opts.PlayerName)
	if playerID == "" {
		playerID = "player"
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = gameID
	}

	manifest := &core.GameManifest{
		ID:         gameID,
		Name:       name,
		SystemID:   systemID,
		WorldID:    worldID,
		Player:     playerID,
		PlayerName: opts.PlayerName,
		Settings:   make(map[string]any),
	}
	if prompt := strings.TrimSpace(opts.OpeningPrompt); prompt != "" {
		manifest.Settings[OpeningPromptSetting] = prompt
	}
	return manifest
}

// copyWorldTemplateEntities copies a world's initial entity templates into the campaign,
// naming each <id>.md so the GUI, which derives entity IDs from file names, can reach
// them. A world with no entities directory contributes nothing rather than failing.
func copyWorldTemplateEntities(worldEntitiesDir, gameEntitiesDir string) error {
	entries, err := os.ReadDir(worldEntitiesDir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(worldEntitiesDir, e.Name()))
		if err != nil {
			return fmt.Errorf("read entity template %q: %w", e.Name(), err)
		}
		template, err := entity.ParseMarkdownEntity(data)
		if err != nil {
			return fmt.Errorf("parse entity template %q: %w", e.Name(), err)
		}
		templateID := template.ID
		if templateID == "" {
			templateID = entity.Slugify(template.Name)
		}
		if templateID == "" {
			templateID = strings.TrimSuffix(e.Name(), ".md")
		}
		templateID = pathutil.SanitizeID(templateID)
		safePath, err := pathutil.ResolveSafeChild(gameEntitiesDir, templateID+".md")
		if err != nil {
			return fmt.Errorf("invalid entity template id %q: %w", templateID, err)
		}
		if err := os.WriteFile(safePath, data, 0644); err != nil {
			return fmt.Errorf("write entity template %q: %w", e.Name(), err)
		}
	}
	return nil
}

// openCampaignStore opens the campaign's canonical store, syncs the notes just written
// into it, and repairs the derived index when the log and the index disagree.
func openCampaignStore(paths *core.PathResolver, gameID, gameDir, gameEntitiesDir string) (*storage.Store, error) {
	store, err := storage.OpenGameStore(paths, gameID)
	if err != nil {
		return nil, fmt.Errorf("init game store: %w", err)
	}

	syncer := storage.NewSyncer(store)
	if _, err := syncer.Sync(gameEntitiesDir); err != nil {
		return nil, fmt.Errorf("initial sync: %w", err)
	}

	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(gameDir, "history.jsonl")), gameID)
	if err := timeline.EnsureIndexed(); err != nil {
		return nil, fmt.Errorf("index turns: %w", err)
	}
	return store, nil
}

// pinOpeningLocation resolves where the campaign begins and records it on the manifest,
// so every later session agrees.
func pinOpeningLocation(paths *core.PathResolver, store *storage.Store, manifest *core.GameManifest) (string, error) {
	startLocation, err := ResolveStartLocation(paths, store, manifest)
	if err != nil {
		return "", fmt.Errorf("resolve start location: %w", err)
	}
	manifest.Settings[StartLocationSetting] = startLocation
	return startLocation, nil
}

// writeCampaignManifest writes the campaign's own record. It runs after the player note
// exists, so a failure leaves no half-built campaign.
func writeCampaignManifest(gameDir string, manifest *core.GameManifest) error {
	manifestBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal game manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), manifestBytes, 0644); err != nil {
		return fmt.Errorf("write game.yaml: %w", err)
	}
	return nil
}

// ensurePlayerNote writes the campaign's player note when the file is absent,
// linking it to the opening location. An authored note is never touched. Without
// this, a campaign created through the GUI cannot serve its own game state, and the
// player is silently missing from every turn's involvement list.
func ensurePlayerNote(paths *core.PathResolver, store *storage.Store, gameID, playerName, details string, pc PlayerCharacter, locationID string) error {
	id := pathutil.SanitizeID(entity.Slugify(playerName))
	if id == "" {
		id = "player"
	}

	path, err := pathutil.ResolveSafeChild(filepath.Join(paths.GameDir(gameID), "entities"), id+".md")
	if err != nil {
		return fmt.Errorf("invalid player note path: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	body := strings.TrimSpace(details)
	if body == "" {
		body = strings.TrimSpace(pc.Background)
	}
	if body == "" {
		body = "The player character."
	}

	player := &entity.Entity{
		ID:         id,
		Name:       playerName,
		Type:       "character",
		Appearance: pc.Appearance,
		Gender:     pc.Gender,
		Age:        pc.Age,
		Body:       body,
		Voice:      pc.Voice,
		Portrait:   pc.Portrait,
		ExtraMeta:  map[string]any{},
	}
	for key, value := range pc.Extra {
		if strings.TrimSpace(value) == "" {
			continue
		}
		player.ExtraMeta[key] = value
	}
	if strings.TrimSpace(pc.Pronouns) != "" {
		player.ExtraMeta["pronouns"] = pc.Pronouns
	}
	if locationID != "" {
		player.Location = "[[" + locationID + "]]"
	}

	data, err := player.SerializeMarkdown()
	if err != nil {
		return fmt.Errorf("serialize player note: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write player note: %w", err)
	}

	if err := storage.NewSyncer(store).SyncFile(path); err != nil {
		return fmt.Errorf("index player note: %w", err)
	}
	return nil
}
