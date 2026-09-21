// pkg/gui/service_test.go
package gui

import (
	"context"
	"os"
	"path/filepath"
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
state:
  hp: 24
  max_hp: 30
  level: 3
---
A cunning rogue in dark leather.`
	_ = os.WriteFile(filepath.Join(entitiesDir, "player-elena.md"), []byte(playerMD), 0644)

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
