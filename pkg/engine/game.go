package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
	"gopkg.in/yaml.v3"
)

type Session struct {
	Paths    *core.PathResolver
	Manifest *core.GameManifest
	System   *core.SystemManifest
	World    *core.WorldManifest
	Store    *storage.Store
}

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
	GameID        string
	SystemID      string
	WorldID       string
	PlayerName    string
	PlayerDetails string
}

func InitGame(paths *core.PathResolver, opts InitOptions) (*Session, error) {
	gameID := opts.GameID
	systemID := opts.SystemID
	worldID := opts.WorldID

	// Verify system & world exist
	sysManifest, err := core.LoadSystemManifest(filepath.Join(paths.SystemDir(systemID), "system.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load system %q: %w", systemID, err)
	}

	worldManifest, err := core.LoadWorldManifest(filepath.Join(paths.WorldDir(worldID), "world.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load world %q: %w", worldID, err)
	}

	gameDir := paths.GameDir(gameID)
	gameEntitiesDir := filepath.Join(gameDir, "entities")
	gameCacheDir := filepath.Join(gameDir, "cache")
	gameAssetsDir := filepath.Join(gameDir, "assets")

	for _, d := range []string{gameDir, gameEntitiesDir, gameCacheDir, gameAssetsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("create dir %q: %w", d, err)
		}
	}

	// Create game manifest
	// The player is stored as an entity ID, with the entered name kept beside it
	// for display. Writing the slug is what keeps the orchestrator, the JS bridge,
	// and the GUI state route agreeing on one identifier for the protagonist.
	playerID := entity.Slugify(opts.PlayerName)
	if playerID == "" {
		playerID = "player"
	}

	manifest := &core.GameManifest{
		ID:         gameID,
		Name:       gameID,
		SystemID:   systemID,
		WorldID:    worldID,
		Player:     playerID,
		PlayerName: opts.PlayerName,
		Settings:   make(map[string]interface{}),
	}

	// Copy initial template entities from world into game, named <id>.md so the
	// GUI, which derives entity IDs from file names, can reach them.
	worldEntitiesDir := filepath.Join(paths.WorldDir(worldID), "entities")
	if entries, err := os.ReadDir(worldEntitiesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(worldEntitiesDir, e.Name()))
			if err != nil {
				return nil, fmt.Errorf("read entity template %q: %w", e.Name(), err)
			}
			template, err := entity.ParseMarkdownEntity(data)
			if err != nil {
				return nil, fmt.Errorf("parse entity template %q: %w", e.Name(), err)
			}
			if err := os.WriteFile(filepath.Join(gameEntitiesDir, template.ID+".md"), data, 0644); err != nil {
				return nil, fmt.Errorf("write entity template %q: %w", e.Name(), err)
			}
		}
	}

	// Open the canonical store and run initial sync
	store, err := storage.OpenGameStore(paths, gameID)
	if err != nil {
		return nil, fmt.Errorf("init game store: %w", err)
	}

	syncer := storage.NewSyncer(store)
	if _, err := syncer.Sync(gameEntitiesDir); err != nil {
		return nil, fmt.Errorf("initial sync: %w", err)
	}

	// Repair the derived index when the log and the index disagree
	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(gameDir, "history.jsonl")), gameID)
	if err := timeline.EnsureIndexed(); err != nil {
		return nil, fmt.Errorf("index turns: %w", err)
	}

	// Pin the opening location so every later session agrees on where the
	// campaign begins.
	startLocation, err := ResolveStartLocation(paths, store, manifest)
	if err != nil {
		return nil, fmt.Errorf("resolve start location: %w", err)
	}
	manifest.Settings[StartLocationSetting] = startLocation

	// Give the campaign a player note before the manifest is written, so a failure
	// leaves no half-built campaign.
	if err := ensurePlayerNote(paths, store, gameID, opts.PlayerName, opts.PlayerDetails, startLocation); err != nil {
		return nil, fmt.Errorf("create player note: %w", err)
	}

	manifestBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal game manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "game.yaml"), manifestBytes, 0644); err != nil {
		return nil, fmt.Errorf("write game.yaml: %w", err)
	}

	return &Session{
		Paths:    paths,
		Manifest: manifest,
		System:   sysManifest,
		World:    worldManifest,
		Store:    store,
	}, nil
}

// ensurePlayerNote writes the campaign's player note when the file is absent,
// linking it to the opening location. An authored note is never touched. Without
// this, a campaign created through the GUI cannot serve its own game state, and the
// player is silently missing from every turn's involvement list.
func ensurePlayerNote(paths *core.PathResolver, store *storage.Store, gameID, playerName, details, locationID string) error {
	id := entity.Slugify(playerName)
	if id == "" {
		id = "player"
	}

	path := filepath.Join(paths.GameDir(gameID), "entities", id+".md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	body := strings.TrimSpace(details)
	if body == "" {
		body = "The player character."
	}

	player := &entity.Entity{
		ID:   id,
		Name: playerName,
		Type: "character",
		Body: body,
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
