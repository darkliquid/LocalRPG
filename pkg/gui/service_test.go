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

	dbPath := filepath.Join(gamesDir, "game.db")
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
