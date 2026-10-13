// Campaigns: their lifecycle, their state as the client sees it, and their assets.

package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

func (s *Service) GetGameState(ctx context.Context, gameID string) (*GameStateDTO, error) {
	gameDir := s.resolver.GameDir(gameID)
	manifestPath := filepath.Join(gameDir, "game.yaml")
	gameManifest, err := core.LoadGameManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read game manifest: %w", err)
	}

	_, contentWarnings, err := engine.ResolveContentLock(s.resolver, gameID)
	if err != nil {
		return nil, fmt.Errorf("resolve content lock: %w", err)
	}

	playerID, err := engine.ResolvePlayerID(s.storeOrNil(gameID), gameManifest)
	if err != nil {
		return nil, fmt.Errorf("resolve player: %w", err)
	}
	if playerID == "" {
		return nil, fmt.Errorf("campaign %q has no player note", gameID)
	}
	playerFile := filepath.Join(gameDir, "entities", playerID+".md")
	data, err := os.ReadFile(playerFile)
	if err != nil {
		return nil, fmt.Errorf("read player entity: %w", err)
	}

	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return nil, fmt.Errorf("parse player entity: %w", err)
	}

	var stateMap map[string]any
	if ent.State != nil {
		stateMap = ent.State.Raw()
	}

	arcs, clocks, locations := s.gameCorpus(gameID)

	var narratorVoice, startLocation string
	if gameManifest.Settings != nil {
		if nv, ok := gameManifest.Settings["narrator_voice"].(string); ok {
			narratorVoice = nv
		}
		if sl, ok := gameManifest.Settings[engine.StartLocationSetting].(string); ok {
			startLocation = sl
		}
	}

	// The campaign route serves the world's banner when the campaign has none of
	// its own, so one URL covers both and a change to either is a new URL.
	bannerURL, _ := s.gameAssetURL(gameDir, gameManifest.WorldID, "banner", gameID)

	var systemManifest *core.SystemManifest
	if sm, err := core.LoadSystemManifest(filepath.Join(s.resolver.SystemDir(gameManifest.SystemID), "system.yaml")); err == nil {
		systemManifest = sm
	}

	// The world's genre tints the app's chrome, so a genre-less campaign keeps the
	// neutral look.
	worldGenre := ""
	if world, err := core.LoadWorldManifest(filepath.Join(s.resolver.WorldDir(gameManifest.WorldID), "world.yaml")); err == nil {
		worldGenre = strings.TrimSpace(world.Genre)
	}

	return &GameStateDTO{
		GameID:   gameID,
		GameName: gameManifest.Name,
		Player: PlayerDTO{
			ID:                playerID,
			Name:              ent.Name,
			Type:              ent.Type,
			State:             stateMap,
			Appearance:        ent.Appearance,
			CurrentAppearance: ent.CurrentAppearance,
			Voice:             voiceProfileDTO(ent.Voice),
		},
		Arcs:          arcs,
		Clocks:        clocks,
		Locations:     locations,
		OpeningPrompt: engine.OpeningPrompt(gameManifest),
		NarratorVoice: narratorVoice,
		Genre:         worldGenre,
		ImageBudget:   engine.ImageBudgetFromManifest(gameManifest),
		ImageApproval: engine.ImageApprovalMode(gameManifest),
		StartLocation: startLocation,
		BannerURL:     bannerURL,

		MechanicsEngagement: engine.ResolveEngagement(gameManifest, systemManifest, s.configMgr.Get()),
		Advancement:         s.computeAdvancement(gameID, systemManifest),
		ContentWarnings:     contentWarnings,
	}, nil
}

// gameCorpus reads the campaign's living world from the index: the locations it
// can visit, the arcs running in the background, and any faction clocks. It is
// derived rather than stored, so a note edited by hand appears immediately.
func (s *Service) gameCorpus(gameID string) ([]NarrativeArcDTO, []FactionClockDTO, []string) {
	arcs := make([]NarrativeArcDTO, 0)
	clocks := make([]FactionClockDTO, 0)
	locations := make([]string, 0)

	store := s.storeOrNil(gameID)
	if store == nil {
		return arcs, clocks, locations
	}

	summaries, err := store.ListEntities()
	if err != nil {
		return arcs, clocks, locations
	}

	for _, summary := range summaries {
		switch summary.Type {
		case "location":
			locations = append(locations, summary.Name)
		case "arc":
			raw := s.entityState(store, summary.ID)
			progress, maxProgress := arcProgress(raw)
			arcs = append(arcs, NarrativeArcDTO{
				ID:          summary.ID,
				Name:        summary.Name,
				Progress:    progress,
				MaxProgress: maxProgress,
				Status:      stateString(raw, "status"),
			})
		case "clock", "faction":
			raw := s.entityState(store, summary.ID)
			ticks, maxTicks := clockTicks(raw)
			clocks = append(clocks, FactionClockDTO{
				Faction:  summary.Name,
				Name:     summary.Name,
				Ticks:    ticks,
				MaxTicks: maxTicks,
			})
		}
	}

	return arcs, clocks, locations
}

func (s *Service) entityState(store *storage.Store, entityID string) map[string]any {
	entity, err := store.GetEntity(entityID)
	if err != nil || entity == nil || entity.State == nil {
		return nil
	}
	return entity.State.Raw()
}

// GameFolders lists a campaign's folder tree.
func (s *Service) GameFolders(gameID string) ([]FolderDTO, error) {
	return s.ListFolders(filepath.Join(s.resolver.GameDir(gameID), "entities"))
}

func (s *Service) GetChronicle(ctx context.Context, gameID string) ([]TurnDTO, error) {
	gameDir := s.resolver.GameDir(gameID)
	historyFile := filepath.Join(gameDir, "history.jsonl")
	logger := engine.NewHistoryLogger(historyFile)
	turns, err := logger.LoadHistory()
	if err != nil {
		return []TurnDTO{}, nil
	}

	cfg := s.configMgr.Get()
	store, err := s.store(gameID)
	if err != nil {
		store = nil // location names and art URLs are decoration, not prerequisites
	}

	dtos := make([]TurnDTO, len(turns))
	engagement := s.engagementFor(gameID)
	for i, turn := range turns {
		dtos[i] = s.turnDTO(turn, store, cfg, gameID, engagement)
	}
	return dtos, nil
}

// gameLock returns the campaign's turn lock, creating it on first use.
func (s *Service) gameLock(gameID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.locks == nil {
		s.locks = make(map[string]*sync.Mutex)
	}
	if lock, ok := s.locks[gameID]; ok {
		return lock
	}

	lock := &sync.Mutex{}
	s.locks[gameID] = lock
	return lock
}

// GetRecap returns the campaign's story so far.
func (s *Service) GetRecap(ctx context.Context, gameID string) (*RecapDTO, error) {
	chronicler := s.chronicler(gameID)
	if chronicler == nil {
		return &RecapDTO{}, nil
	}

	chronicle, err := chronicler.Recap(gameID)
	if err != nil {
		return nil, fmt.Errorf("read chronicle: %w", err)
	}

	latest := 0
	historyPath := filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")
	if turns, err := engine.NewHistoryLogger(historyPath).LoadHistory(); err == nil && len(turns) > 0 {
		latest = turns[len(turns)-1].Number
	}

	threads := make([]ThreadDTO, 0)
	if open, err := engine.OpenThreads(s.storeOrNil(gameID), latest); err == nil {
		for _, thread := range open {
			threads = append(threads, ThreadDTO{
				ID:           thread.ID,
				Name:         thread.Name,
				Status:       thread.Status,
				LastAdvanced: thread.LastAdvanced,
				Idle:         thread.Idle,
			})
		}
	}

	return &RecapDTO{
		Summary:     chronicle.Summary,
		ThroughTurn: chronicle.ThroughTurn,
		Enabled:     s.configMgr.Get().SummaryEvery() > 0,
		Threads:     threads,
	}, nil
}

// GetLocationArt returns a location's scene image and its content type, drawing it
// on first request and reusing it until the appearance changes.
func (s *Service) GetLocationArt(ctx context.Context, gameID, locationID string, force bool) (string, string, error) {
	// Both ids name a file, so both are validated as ids. Appending ".md" to an
	// unvalidated value leaves any separator and parent reference intact, and the
	// join below would then resolve outside the game directory.
	if err := pathutil.ValidateID(gameID); err != nil {
		return "", "", fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(locationID); err != nil {
		return "", "", fmt.Errorf("invalid location id: %w", err)
	}
	// Read the note from disk rather than the index: appearance, tags, and state are
	// authored content, and the index is derived from them.
	notePath := filepath.Join(s.resolver.GameDir(gameID), "entities", locationID+".md")
	data, err := os.ReadFile(notePath)
	if err != nil {
		return "", "", fmt.Errorf("location %q not found: %w", locationID, err)
	}

	location, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return "", "", fmt.Errorf("parse location %q: %w", locationID, err)
	}

	store := s.sceneArtResolver(gameID)
	if store == nil {
		return "", "", fmt.Errorf("build image client")
	}

	cfg := s.configMgr.Get()
	start := time.Now()
	s.logger = trace.OrNil(s.logger)
	s.logger.Event("media.image.request", map[string]any{
		"location": locationID,
		"provider": cfg.Media.Image.Type + ":" + cfg.Media.Image.Model,
		"force":    force,
	})

	path, err := store.SceneArt(ctx, location, force)
	if err != nil {
		s.logger.Event("provider.error", map[string]any{"role": "image", "error": err.Error()})
		return "", "", err
	}

	if data, err := os.ReadFile(path); err == nil {
		s.logger.Event("media.image.result", map[string]any{
			"bytes":       len(data),
			"duration_ms": time.Since(start).Milliseconds(),
		})
	}

	return path, contentTypeForArt(path), nil
}

// worldArtStyle reads the art style and genre of the campaign's world, which keeps a
// setting's imagery visually consistent.
func (s *Service) worldArtStyle(gameID string) string {
	manifest, err := core.LoadGameManifest(filepath.Join(s.resolver.GameDir(gameID), "game.yaml"))
	if err != nil {
		return ""
	}

	world, err := core.LoadWorldManifest(filepath.Join(s.resolver.WorldDir(manifest.WorldID), "world.yaml"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.Join([]string{world.ArtStyle, world.Genre}, ", "))
}

// gameAssetSource resolves a campaign's banner or icon, preferring the
// campaign's own copy and falling back to the world's when it has none. The
// fallback is resolved at read time, so it copies nothing and a campaign that
// never had its own art still has none.
func (s *Service) gameAssetSource(gameDir, worldID, assetKind string) (string, string, bool) {
	if p, ext := findAssetFile(gameDir, assetKind); p != "" {
		return p, contentTypeForArt(ext), true
	}
	if strings.TrimSpace(worldID) == "" {
		return "", "", false
	}
	if p, ext := findAssetFile(s.resolver.WorldDir(worldID), assetKind); p != "" {
		return p, contentTypeForArt(ext), true
	}
	return "", "", false
}

// gameAssetURL returns the campaign route for a banner or icon, preferring the
// campaign's own asset and falling back to the world's, and reports which one it
// is ("campaign", "world", or "" when neither exists).
//
// The URL carries a token derived from the file it will serve. Without it the
// route is identical before and after an asset is generated or cleared, so a
// browser keeps showing the image it already cached and a new generation looks
// like it did nothing.
func (s *Service) gameAssetURL(gameDir, worldID, assetKind, gameID string) (string, string) {
	base := fmt.Sprintf("/api/game/%s/%s", gameID, assetKind)
	if p, _ := findAssetFile(gameDir, assetKind); p != "" {
		return versionedAssetURL(base, p), "campaign"
	}
	if strings.TrimSpace(worldID) != "" {
		if p, _ := findAssetFile(s.resolver.WorldDir(worldID), assetKind); p != "" {
			return versionedAssetURL(base, p), "world"
		}
	}
	return "", ""
}

func (s *Service) ListGames(ctx context.Context) ([]GameSummaryDTO, error) {
	gamesDir := s.resolver.GamesDir()
	entries, err := os.ReadDir(gamesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []GameSummaryDTO{}, nil
		}
		return nil, fmt.Errorf("read games dir: %w", err)
	}

	summaries := make([]GameSummaryDTO, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		gameID := e.Name()
		gameDir := filepath.Join(gamesDir, gameID)
		manifestPath := filepath.Join(gameDir, "game.yaml")
		m, err := core.LoadGameManifest(manifestPath)
		if err != nil {
			continue
		}

		turnCount := 0
		historyPath := filepath.Join(gameDir, "history.jsonl")
		logger := engine.NewHistoryLogger(historyPath)
		if turns, err := logger.LoadHistory(); err == nil {
			turnCount = len(turns)
		}

		var latestTime time.Time
		if fi, err := os.Stat(manifestPath); err == nil {
			latestTime = fi.ModTime()
		}
		if fi, err := os.Stat(historyPath); err == nil {
			if fi.ModTime().After(latestTime) {
				latestTime = fi.ModTime()
			}
		}

		lastPlayed := ""
		if !latestTime.IsZero() {
			lastPlayed = latestTime.Format(time.RFC3339)
		}

		name := m.Name
		if name == "" {
			name = gameID
		}

		playerName := m.PlayerName
		if playerName == "" {
			playerName = m.Player
		}

		bannerURL, bannerSource := s.gameAssetURL(gameDir, m.WorldID, "banner", gameID)
		iconURL, iconSource := s.gameAssetURL(gameDir, m.WorldID, "icon", gameID)

		summaries = append(summaries, GameSummaryDTO{
			ID:              gameID,
			Name:            name,
			SystemID:        m.SystemID,
			WorldID:         m.WorldID,
			PlayerName:      playerName,
			TurnCount:       turnCount,
			LastPlayed:      lastPlayed,
			BannerURL:       bannerURL,
			IconURL:         iconURL,
			BannerSource:    bannerSource,
			IconSource:      iconSource,
			PlayTimeSeconds: int64(turnCount * 120),
		})
	}

	// Most recently played first, so "Resume" is the campaign the player left.
	sort.SliceStable(summaries, func(i, j int) bool {
		return summaries[i].LastPlayed > summaries[j].LastPlayed
	})
	return summaries, nil
}

func (s *Service) CreateGame(ctx context.Context, req CreateGameRequestDTO) (*GameSummaryDTO, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("campaign name is required")
	}
	if req.SystemID == "" {
		return nil, fmt.Errorf("system_id is required")
	}
	if req.WorldID == "" {
		return nil, fmt.Errorf("world_id is required")
	}
	if req.PlayerName == "" {
		req.PlayerName = "Adventurer"
	}

	gameID := req.ID
	if gameID == "" {
		gameID = slugify(req.Name)
	}

	// The system decides which prompts a character must answer, falling back to
	// the engine's defaults when it defines none.
	sysManifest, err := core.LoadSystemManifest(filepath.Join(s.resolver.SystemDir(req.SystemID), "system.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load system %q: %w", req.SystemID, err)
	}
	fields := engine.CharacterFields(sysManifest)
	answers := map[string]string{
		"appearance": req.Player.Appearance,
		"age":        req.Player.Age,
		"gender":     req.Player.Gender,
		"pronouns":   req.Player.Pronouns,
		"background": req.Player.Background,
	}
	for key, value := range req.Player.Extra {
		answers[key] = value
	}
	// Required prompts are enforced only when the caller is doing character
	// creation. A programmatic or legacy create with no player object keeps
	// working and simply leaves the protagonist lightly described.
	playerProvided := strings.TrimSpace(req.Player.Appearance) != "" ||
		strings.TrimSpace(req.Player.Background) != "" ||
		strings.TrimSpace(req.Player.Age) != "" ||
		strings.TrimSpace(req.Player.Gender) != "" ||
		strings.TrimSpace(req.Player.Pronouns) != "" ||
		len(req.Player.Extra) > 0 ||
		req.Player.Voice != nil
	if playerProvided {
		for _, id := range engine.RequiredCharacterFields(fields) {
			if strings.TrimSpace(answers[id]) == "" {
				return nil, fmt.Errorf("character field %q is required", id)
			}
		}
	}

	var voice *entity.VoiceConfig
	if req.Player.Voice != nil && strings.TrimSpace(req.Player.Voice.VoiceID) != "" {
		voice = &entity.VoiceConfig{
			Provider:   req.Player.Voice.Provider,
			VoiceID:    req.Player.Voice.VoiceID,
			Pitch:      req.Player.Voice.Pitch,
			SpeechRate: req.Player.Voice.SpeechRate,
		}
	}

	session, err := engine.InitGame(s.resolver, engine.InitOptions{
		GameID:     gameID,
		Name:       req.Name,
		SystemID:   req.SystemID,
		WorldID:    req.WorldID,
		PlayerName: req.PlayerName,
		PlayerCharacter: engine.PlayerCharacter{
			Appearance: req.Player.Appearance,
			Age:        req.Player.Age,
			Gender:     req.Player.Gender,
			Pronouns:   req.Player.Pronouns,
			Background: req.Player.Background,
			Voice:      voice,
			Extra:      req.Player.Extra,
		},
		OpeningPrompt: req.OpeningPrompt,
	})
	if err != nil {
		return nil, fmt.Errorf("init game: %w", err)
	}
	_ = session.Close()

	// Spend the client deferred while building this campaign now belongs to it.
	// Nothing is recorded under this token unless the client asked for deferral.
	if err := s.CommitDeferredUsage(gameID, gameID); err != nil {
		trace.OrNil(s.logger).Event("usage.commit_error", map[string]any{"game": gameID, "error": err.Error()})
	}

	if strings.TrimSpace(req.NarratorVoice) != "" {
		_ = s.UpdateGameSettings(ctx, gameID, map[string]any{
			"narrator_voice": strings.TrimSpace(req.NarratorVoice),
		})
	}
	if strings.TrimSpace(req.StartLocation) != "" {
		_ = s.UpdateGameSettings(ctx, gameID, map[string]any{
			engine.StartLocationSetting: strings.TrimSpace(req.StartLocation),
		})
	}

	// A voice the player did not choose is chosen from their description, so the
	// protagonist can speak in their own voice from the first turn.
	if voice == nil {
		_ = s.assignPlayerVoice(gameID, req.PlayerName)
	}

	return &GameSummaryDTO{
		ID:         gameID,
		Name:       req.Name,
		SystemID:   req.SystemID,
		WorldID:    req.WorldID,
		PlayerName: req.PlayerName,
		TurnCount:  0,
		LastPlayed: time.Now().Format(time.RFC3339),
	}, nil
}

// assignPlayerVoice gives the player note a voice profile matched from its
// description when the player did not choose one. A failure is not fatal: an
// unvoiced player simply reads in the narrator voice.
func (s *Service) assignPlayerVoice(gameID, playerName string) error {
	store, err := s.store(gameID)
	if err != nil {
		return err
	}

	id := entity.Slugify(playerName)
	if id == "" {
		id = "player"
	}

	ent, err := store.GetEntity(id)
	if err != nil || ent == nil || ent.Voice != nil {
		return err
	}

	// Only voices the active provider can synthesise are assignable, so a profile
	// authored for another engine is never chosen while it is unreachable.
	mediaCfg := s.configMgr.Get().Media.TTS
	harness.AssignVoiceProfile(ent, media.FilterVoiceProfiles(mediaCfg.VoiceProfiles, media.ProviderKey(mediaCfg)))
	if ent.Voice == nil {
		return nil
	}

	historyPath := filepath.Join(s.resolver.GameDir(gameID), "history.jsonl")
	timeline := engine.NewTimeline(s.resolver, store, engine.NewHistoryLogger(historyPath), gameID)
	return timeline.SaveEntity(ent)
}

// UpdateGameSettings merges a patch into a campaign's settings and writes the
// manifest. The caller supplies whole values; nothing is inferred or coerced.
func (s *Service) UpdateGameSettings(ctx context.Context, gameID string, patch map[string]any) error {
	path := filepath.Join(s.resolver.GameDir(gameID), "game.yaml")
	manifest, err := core.LoadGameManifest(path)
	if err != nil {
		return fmt.Errorf("load game manifest: %w", err)
	}
	if manifest.Settings == nil {
		manifest.Settings = map[string]any{}
	}
	for key, value := range patch {
		manifest.Settings[key] = value
	}
	if err := core.SaveGameManifest(path, manifest); err != nil {
		return fmt.Errorf("save game settings: %w", err)
	}
	return nil
}

// DeleteGame removes a campaign and everything it generated. The turn lock is
// taken first so a turn in flight finishes or is refused rather than writing into
// a directory that is being deleted.
func (s *Service) DeleteGame(ctx context.Context, gameID string) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}

	lock := s.gameLock(gameID)
	if !lock.TryLock() {
		return ErrTurnInFlight
	}
	defer lock.Unlock()

	gameDir := s.resolver.GameDir(gameID)
	if _, err := os.Stat(filepath.Join(gameDir, "game.yaml")); err != nil {
		return fmt.Errorf("campaign %q: %w", gameID, err)
	}

	if err := storage.CloseGameStore(s.resolver, gameID); err != nil {
		return err
	}
	if err := os.RemoveAll(gameDir); err != nil {
		return fmt.Errorf("remove campaign: %w", err)
	}
	s.forgetGame(gameID)
	return nil
}

// RestartGame returns a campaign to its opening state without disturbing its
// configuration: the timeline and the world's mutable cast are reset in place,
// while the manifest, the artwork, the narrator voice, the protagonist and the
// spend ledger are carried across untouched.
func (s *Service) RestartGame(ctx context.Context, gameID string) (*GameSummaryDTO, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return nil, fmt.Errorf("invalid game id: %w", err)
	}

	lock := s.gameLock(gameID)
	if !lock.TryLock() {
		return nil, ErrTurnInFlight
	}
	defer lock.Unlock()

	gameDir := s.resolver.GameDir(gameID)
	manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load game manifest: %w", err)
	}

	store, err := s.store(gameID)
	if err != nil {
		return nil, fmt.Errorf("open campaign store: %w", err)
	}

	// Stop any batch synthesis still speaking narration the reset is about to
	// discard, so the provider is not left working on it.
	s.cancelBatchJobs(ctx, gameID)

	if err := engine.ResetCampaign(s.resolver, store, manifest); err != nil {
		return nil, fmt.Errorf("reset campaign: %w", err)
	}

	name := manifest.Name
	if name == "" {
		name = gameID
	}
	playerName := manifest.PlayerName
	if playerName == "" {
		playerName = manifest.Player
	}

	return &GameSummaryDTO{
		ID:         gameID,
		Name:       name,
		SystemID:   manifest.SystemID,
		WorldID:    manifest.WorldID,
		PlayerName: playerName,
		TurnCount:  0,
		LastPlayed: time.Now().Format(time.RFC3339),
	}, nil
}

// forgetGame clears the one-time index repair marker so a recreated campaign is
// indexed again rather than trusting the deleted database.
func (s *Service) forgetGame(gameID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.indexed, gameID)
}

func (s *Service) GetGameAsset(gameID, assetKind string) (string, string, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return "", "", fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(assetKind); err != nil {
		return "", "", fmt.Errorf("invalid asset kind: %w", err)
	}
	gameDir := s.resolver.GameDir(gameID)
	if _, err := os.Stat(gameDir); err != nil {
		return "", "", os.ErrNotExist
	}
	worldID := ""
	if manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml")); err == nil {
		worldID = manifest.WorldID
	}
	if p, contentType, ok := s.gameAssetSource(gameDir, worldID, assetKind); ok {
		return p, contentType, nil
	}
	return "", "", os.ErrNotExist
}

func (s *Service) SaveGameAsset(gameID, assetKind string, data []byte, ext string) (string, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return "", fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(assetKind); err != nil {
		return "", fmt.Errorf("invalid asset kind: %w", err)
	}
	gameDir := s.resolver.GameDir(gameID)
	if _, err := os.Stat(gameDir); err != nil {
		return "", fmt.Errorf("game not found: %w", err)
	}
	assetsDir := filepath.Join(gameDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}
	if err := removeAssetFiles(assetsDir, assetKind); err != nil {
		return "", err
	}
	if ext == "" {
		ext = ".png"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	targetPath, err := pathutil.ResolveSafeChild(assetsDir, assetKind+ext)
	if err != nil {
		return "", fmt.Errorf("invalid asset path: %w", err)
	}
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", fmt.Errorf("write asset: %w", err)
	}
	return fmt.Sprintf("/api/game/%s/%s", gameID, assetKind), nil
}

// DeleteGameAsset removes a campaign's own banner or icon so it falls back to
// the world's artwork again. It is a no-op when the campaign has none of its own.
func (s *Service) DeleteGameAsset(gameID, assetKind string) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(assetKind); err != nil {
		return fmt.Errorf("invalid asset kind: %w", err)
	}
	gameDir := s.resolver.GameDir(gameID)
	if _, err := os.Stat(gameDir); err != nil {
		return fmt.Errorf("game not found: %w", err)
	}
	return removeAssetFiles(filepath.Join(gameDir, "assets"), assetKind)
}

// campaignArtPrompt describes a campaign for its own banner or icon, so the
// generated art is about this campaign rather than a copy of its world's. It
// draws on the campaign's own material -- where it opens, its opening directive,
// and the protagonist -- while the world still supplies the art style and a
// grounding name.
func (s *Service) campaignArtPrompt(gameID, kind string) string {
	gameDir := s.resolver.GameDir(gameID)
	manifest, err := core.LoadGameManifest(filepath.Join(gameDir, "game.yaml"))
	if err != nil || manifest == nil {
		return buildAssetPrompt(kind, gameID, "", "", "")
	}

	artStyle, worldName := "", ""
	if wm, err := core.LoadWorldManifest(filepath.Join(s.resolver.WorldDir(manifest.WorldID), "world.yaml")); err == nil && wm != nil {
		artStyle = wm.ArtStyle
		worldName = wm.Name
	}

	parts := make([]string, 0, 4)
	if location := gameSettingString(manifest, engine.StartLocationSetting); location != "" {
		parts = append(parts, location)
	}
	if opening := engine.OpeningPrompt(manifest); opening != "" {
		parts = append(parts, opening)
	}
	if protagonist := s.protagonistArtSummary(gameDir, manifest); protagonist != "" {
		parts = append(parts, protagonist)
	}
	if worldName != "" {
		parts = append(parts, "set in "+worldName)
	}

	name := manifest.Name
	if name == "" {
		name = gameID
	}
	if len(parts) == 0 {
		// Nothing campaign-specific to say, so ground the prompt on the campaign
		// name alone rather than repeating the world.
		parts = append(parts, name)
	}

	description := harness.TruncateRunes(strings.Join(parts, ", "), campaignArtDescriptionLimit)
	return buildAssetPrompt(kind, name, description, artStyle, "")
}

// protagonistArtSummary names the protagonist and describes how they look, so a
// campaign's artwork can feature the character the player actually made.
func (s *Service) protagonistArtSummary(gameDir string, manifest *core.GameManifest) string {
	store, err := s.store(manifest.ID)
	if err != nil {
		return ""
	}
	playerID, err := engine.ResolvePlayerID(store, manifest)
	if err != nil || playerID == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(gameDir, "entities", playerID+".md"))
	if err != nil {
		return ""
	}
	ent, err := entity.ParseMarkdownEntity(data)
	if err != nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if ent.Name != "" {
		parts = append(parts, ent.Name)
	}
	if ent.Appearance != "" {
		parts = append(parts, ent.Appearance)
	}
	return strings.Join(parts, ", ")
}

func (s *Service) GenerateGameAsset(ctx context.Context, gameID string, req GenerateAssetRequestDTO) (string, error) {
	if err := pathutil.ValidateID(gameID); err != nil {
		return "", fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(req.Kind); err != nil {
		return "", fmt.Errorf("invalid asset kind: %w", err)
	}
	prompt := req.Prompt
	if prompt == "" {
		prompt = s.campaignArtPrompt(gameID, req.Kind)
	}
	imgBytes, failure := s.generateImage(ctx, req.Kind, prompt, usageScope{gameID: gameID})
	if failure != nil {
		return "", failure
	}
	return s.SaveGameAsset(gameID, req.Kind, imgBytes, media.ArtExtension(imgBytes))
}

// GetTurnContext returns the most recent turn's context snapshot and prompt for a campaign.
func (s *Service) GetTurnContext(gameID string) (*TurnContextDTO, error) {
	s.ensureIndexed(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	turns, err := store.ListTurns(1, 0)
	if err != nil || len(turns) == 0 {
		return nil, fmt.Errorf("no turns recorded for game %q", gameID)
	}
	lastTurn := turns[0].Number
	raw, prompt, err := store.GetTurnContext(lastTurn)
	if err != nil {
		return nil, fmt.Errorf("get turn context: %w", err)
	}
	var turnCtx harness.TurnContext
	if err := json.Unmarshal(raw, &turnCtx); err != nil {
		return nil, fmt.Errorf("decode turn context: %w", err)
	}
	return toTurnContextDTO(&turnCtx, prompt), nil
}

// GetWorkingSet returns the active continuity working set for a campaign.
func (s *Service) GetWorkingSet(gameID string) ([]WorkingEntryDTO, error) {
	s.ensureIndexed(gameID)
	store, err := s.store(gameID)
	if err != nil {
		return nil, err
	}
	records, err := store.LoadWorkingSet()
	if err != nil {
		return nil, fmt.Errorf("load working set: %w", err)
	}
	entries := make([]WorkingEntryDTO, 0, len(records))
	for _, rec := range records {
		name := rec.EntityID
		if ent, err := store.GetEntity(rec.EntityID); err == nil && ent != nil && ent.Name != "" {
			name = ent.Name
		}
		entries = append(entries, WorkingEntryDTO{
			Kind:     rec.Kind,
			ID:       rec.EntityID,
			Name:     name,
			Weight:   rec.Weight,
			LastTurn: rec.LastTurn,
			Role:     rec.Role,
		})
	}
	return entries, nil
}
