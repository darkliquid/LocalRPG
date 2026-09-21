package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestGameInitAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)

	// 1. Setup system
	sysDir := paths.SystemDir("d20-test")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: d20-test\nname: D20 Test\nversion: 1.0\n"), 0644)

	// 2. Setup world
	worldDir := paths.WorldDir("fantasy-realm")
	worldEntities := filepath.Join(worldDir, "entities")
	if err := os.MkdirAll(worldEntities, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte("id: fantasy-realm\nname: Fantasy Realm\n"), 0644)
	tavernDoc := "---\nid: tavern\nname: Oakhaven Tavern\ntype: location\n---\nStarting tavern."
	os.WriteFile(filepath.Join(worldEntities, "Tavern.md"), []byte(tavernDoc), 0644)

	// 3. Initialize game
	session, err := InitGame(paths, "campaign-01", "d20-test", "fantasy-realm", "Sean")
	if err != nil {
		t.Fatalf("InitGame failed: %v", err)
	}
	defer session.Close()

	if session.Manifest.Player != "Sean" {
		t.Errorf("expected player Sean, got %q", session.Manifest.Player)
	}

	// Verify base entity was copied and indexed into game
	loaded, err := session.Store.GetEntity("tavern")
	if err != nil {
		t.Fatalf("failed to query tavern from game index: %v", err)
	}
	if loaded.Name != "Oakhaven Tavern" {
		t.Errorf("expected Oakhaven Tavern, got %q", loaded.Name)
	}

	// The resolved opening location is recorded on the manifest and persisted
	if got := session.Manifest.Settings[StartLocationSetting]; got != "tavern" {
		t.Errorf("expected start location tavern, got %v", got)
	}
	persisted, err := core.LoadGameManifest(filepath.Join(paths.GameDir("campaign-01"), "game.yaml"))
	if err != nil {
		t.Fatalf("reload game manifest failed: %v", err)
	}
	if got := persisted.Settings[StartLocationSetting]; got != "tavern" {
		t.Errorf("expected persisted start location tavern, got %v", got)
	}

	if _, err := os.Stat(paths.GameDBPath("campaign-01")); err != nil {
		t.Errorf("expected the canonical game database: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.GameDir("campaign-01"), "game.db")); !os.IsNotExist(err) {
		t.Errorf("expected no legacy game.db beside the game, stat err = %v", err)
	}

	if _, err := os.Stat(filepath.Join(paths.GameDir("campaign-01"), "entities", "tavern.md")); err != nil {
		t.Errorf("expected the world template to be copied as <id>.md: %v", err)
	}
}
