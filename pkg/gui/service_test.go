// pkg/gui/service_test.go
package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/storage"
)

func setupTestGame(t *testing.T) (string, *Service) {
	tempDir := t.TempDir()
	gamesDir := filepath.Join(tempDir, "games", "test-campaign")
	entitiesDir := filepath.Join(gamesDir, "entities")
	_ = os.MkdirAll(entitiesDir, 0755)

	// Create test game manifest
	manifestContent := `id: test-campaign
name: Test Campaign
system: core-d20
world: shadow-realm
player: player-elena
`
	_ = os.WriteFile(filepath.Join(gamesDir, "game.yaml"), []byte(manifestContent), 0644)

	// Create player entity
	playerMD := `---
id: player-elena
name: Elena Nightshade
type: character
location: "[[aldon-harbour]]"
state:
  hp: 24
  max_hp: 30
  level: 3
---
A cunning rogue in dark leather.`
	_ = os.WriteFile(filepath.Join(entitiesDir, "player-elena.md"), []byte(playerMD), 0644)

	// Create a location, so art and turn-location tests have something real.
	locationMD := `---
id: aldon-harbour
name: Aldon Harbour
type: location
tags: [harbour, docks]
---
Salt air and gull cries.`
	_ = os.WriteFile(filepath.Join(entitiesDir, "aldon-harbour.md"), []byte(locationMD), 0644)

	// Create NPC entity
	npcMD := `---
id: captain-kaelen
name: Captain Kaelen
type: npc
voice:
  provider: kokoro
  voice_id: bm_george
state:
  attitude: neutral
---
The town watch captain. Speaks with [[player-elena]].`
	_ = os.WriteFile(filepath.Join(entitiesDir, "captain-kaelen.md"), []byte(npcMD), 0644)

	dbPath := filepath.Join(gamesDir, "cache", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	syncer := storage.NewSyncer(store)
	_, _ = syncer.Sync(entitiesDir)
	_ = store.Close()

	// Media is enabled so playback and art routes have something to serve. The
	// built-in TTS client returns synthetic bytes and needs no external process.
	configYAML := "media:\n  tts:\n    type: builtin\n    default_voice: narrator\n"
	if err := os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	service := NewService(tempDir)
	return "test-campaign", service
}

func TestGUIService_GetGameState(t *testing.T) {
	gameID, svc := setupTestGame(t)

	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGameState failed: %v", err)
	}

	if state.Player.Name != "Elena Nightshade" {
		t.Errorf("expected player Elena Nightshade, got %s", state.Player.Name)
	}
	if state.Player.State["hp"] != 24 {
		t.Errorf("expected hp 24, got %v", state.Player.State["hp"])
	}
}

func TestGUIService_GetGraph(t *testing.T) {
	gameID, svc := setupTestGame(t)

	graph, err := svc.GetGraph(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGraph failed: %v", err)
	}

	if len(graph.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(graph.Nodes))
	}

	foundLink := false
	for _, link := range graph.Links {
		if link.Source == "captain-kaelen" && link.Target == "player-elena" {
			foundLink = true
			break
		}
	}
	if !foundLink {
		t.Errorf("expected wikilink edge between captain-kaelen and player-elena")
	}
}

func TestGUIService_EntityCRUD(t *testing.T) {
	gameID, svc := setupTestGame(t)

	ent, err := svc.GetEntity(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if ent.Name != "Captain Kaelen" {
		t.Errorf("expected Captain Kaelen, got %s", ent.Name)
	}

	// Update entity
	updatedMD := `---
name: Captain Kaelen
type: npc
state:
  attitude: friendly
---
The town watch captain, now an ally.`
	err = svc.SaveEntity(context.Background(), gameID, "captain-kaelen", updatedMD)
	if err != nil {
		t.Fatalf("SaveEntity failed: %v", err)
	}

	updated, err := svc.GetEntity(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if updated.State["attitude"] != "friendly" {
		t.Errorf("expected attitude friendly, got %v", updated.State["attitude"])
	}
}

func TestGetEntityReadsCanonicalDatabase(t *testing.T) {
	gameID, svc := setupTestGame(t)

	ent, err := svc.GetEntity(context.Background(), gameID, "player-elena")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(ent.Backlinks) == 0 {
		t.Errorf("expected backlinks from the canonical index, got none")
	}

	gameDir := svc.GetResolver().GameDir(gameID)
	if _, err := os.Stat(filepath.Join(gameDir, "game.db")); !os.IsNotExist(err) {
		t.Errorf("expected no legacy game.db, stat err = %v", err)
	}
	if _, err := os.Stat(svc.GetResolver().GameDBPath(gameID)); err != nil {
		t.Errorf("expected the canonical database: %v", err)
	}
}

func TestGetChronicleAndEntityTurnsReportInvolvement(t *testing.T) {
	gameID, svc := setupTestGame(t)
	gameDir := svc.GetResolver().GameDir(gameID)

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"I ask the captain","narration":"The captain nods.","segments":[{"kind":"narration","text":"The captain nods."}],"entities":[{"id":"player-elena","mention":"player"},{"id":"captain-kaelen","mention":"wikilink"}]}` + "\n"
	if err := os.WriteFile(filepath.Join(gameDir, "history.jsonl"), []byte(record), 0644); err != nil {
		t.Fatal(err)
	}

	turns, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetChronicle failed: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if len(turns[0].EntitiesHit) != 2 {
		t.Errorf("expected 2 entities hit, got %+v", turns[0].EntitiesHit)
	}
	if len(turns[0].Segments) != 1 || turns[0].Segments[0].Kind != "narration" {
		t.Errorf("expected segments to reach the DTO, got %+v", turns[0].Segments)
	}

	involved, err := svc.GetEntityTurns(context.Background(), gameID, "captain-kaelen")
	if err != nil {
		t.Fatalf("GetEntityTurns failed: %v", err)
	}
	if len(involved) != 1 || involved[0].TurnNumber != 1 {
		t.Fatalf("expected turn 1 for captain-kaelen, got %+v", involved)
	}

	uninvolved, err := svc.GetEntityTurns(context.Background(), gameID, "nobody-at-all")
	if err != nil {
		t.Fatalf("GetEntityTurns failed: %v", err)
	}
	if len(uninvolved) != 0 {
		t.Errorf("expected no turns for an uninvolved entity, got %+v", uninvolved)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGetLocationArtUsesTheBuiltinGeneratorAndCaches(t *testing.T) {
	gameID, svc := setupTestGame(t)

	path, contentType, err := svc.GetLocationArt(context.Background(), gameID, "aldon-harbour", false)
	if err != nil {
		t.Fatalf("GetLocationArt failed: %v", err)
	}
	if contentType != "image/svg+xml" {
		t.Errorf("contentType = %q, want image/svg+xml from the built-in generator", contentType)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected the art on disk: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(data)), "<svg") {
		t.Errorf("expected SVG art")
	}

	// A second request for the same appearance is a cache hit at the same path.
	again, _, err := svc.GetLocationArt(context.Background(), gameID, "aldon-harbour", false)
	if err != nil {
		t.Fatalf("second GetLocationArt failed: %v", err)
	}
	if again != path {
		t.Errorf("expected a cache hit at %q, got %q", path, again)
	}

	// Appearance changes move the key, so the image changes with it.
	entityPath := filepath.Join(svc.GetResolver().GameDir(gameID), "entities", "aldon-harbour.md")
	updated := strings.Replace(string(mustRead(t, entityPath)), "type: location", "type: location\nappearance: burned and abandoned", 1)
	if err := os.WriteFile(entityPath, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}

	moved, _, err := svc.GetLocationArt(context.Background(), gameID, "aldon-harbour", false)
	if err != nil {
		t.Fatalf("GetLocationArt after a change failed: %v", err)
	}
	if moved == path {
		t.Errorf("expected a new image after the appearance changed")
	}
}

func writeSegmentTurn(t *testing.T, svc *Service, gameID string) {
	t.Helper()

	record := `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Say","input":"Where is the ledger?","narration":"He does not look up.","segments":[{"kind":"narration","text":"He does not look up."},{"kind":"speech","speaker":"Captain Kaelen","speaker_id":"captain-kaelen","text":"Keep walking."}]}` + "\n"
	path := filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl")
	if err := os.WriteFile(path, []byte(record), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGetSegmentAudioSynthesizesAndCaches(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	path, err := svc.GetSegmentAudio(context.Background(), gameID, 1, 1)
	if err != nil {
		t.Fatalf("GetSegmentAudio failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected a clip on disk: %v", err)
	}

	again, err := svc.GetSegmentAudio(context.Background(), gameID, 1, 1)
	if err != nil {
		t.Fatalf("second GetSegmentAudio failed: %v", err)
	}
	if again != path {
		t.Errorf("expected the cached clip %q, got %q", path, again)
	}

	if _, err := svc.GetSegmentAudio(context.Background(), gameID, 1, 9); err == nil {
		t.Errorf("expected an error for an out-of-range segment")
	}
	if _, err := svc.GetSegmentAudio(context.Background(), gameID, 42, 0); err == nil {
		t.Errorf("expected an error for an unknown turn")
	}
}

func TestGetSegmentAudioWithoutTTSIsUnavailable(t *testing.T) {
	// A service whose config never enabled TTS.
	quiet := NewService(t.TempDir())
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	if err := os.MkdirAll(quiet.GetResolver().GameDir(gameID), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quiet.GetResolver().GameDir(gameID), "history.jsonl"), mustRead(t, filepath.Join(svc.GetResolver().GameDir(gameID), "history.jsonl")), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := quiet.GetSegmentAudio(context.Background(), gameID, 1, 1); !errors.Is(err, ErrAudioUnavailable) {
		t.Errorf("expected ErrAudioUnavailable, got %v", err)
	}
}

func TestChronicleOffersAudioURLsWhenTTSIsConfigured(t *testing.T) {
	gameID, svc := setupTestGame(t)
	writeSegmentTurn(t, svc, gameID)

	turns, err := svc.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || len(turns[0].Segments) != 2 {
		t.Fatalf("unexpected chronicle: %+v", turns)
	}
	if turns[0].Segments[0].AudioURL == "" || turns[0].Segments[1].AudioURL == "" {
		t.Errorf("expected audio URLs on both segments, got %+v", turns[0].Segments)
	}
}
