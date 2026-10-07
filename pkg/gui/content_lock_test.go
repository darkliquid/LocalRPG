package gui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
)

func TestOpenSurfacesContentWarnings(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	sysID := "warn_sys"
	worldID := "warn_world"
	gameID := "warn_game"

	sysDir := svc.resolver.SystemDir(sysID)
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	sysYAML := "id: " + sysID + "\nname: Warn System\nversion: 1.0.0\n"
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte(sysYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "mechanics.js"), []byte("// initial mechanics"), 0644); err != nil {
		t.Fatal(err)
	}

	worldDir := svc.resolver.WorldDir(worldID)
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	worldYAML := "id: " + worldID + "\nname: Warn World\nversion: 1.0.0\n"
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}

	session, err := engine.InitGame(svc.resolver, engine.InitOptions{
		GameID:     gameID,
		Name:       "Warn Game",
		SystemID:   sysID,
		WorldID:    worldID,
		PlayerName: "Hero",
	})
	if err != nil {
		t.Fatalf("InitGame: %v", err)
	}
	session.Close()

	// Modify mechanics.js after locking
	if err := os.WriteFile(filepath.Join(sysDir, "mechanics.js"), []byte("// modified mechanics"), 0644); err != nil {
		t.Fatal(err)
	}

	state, err := svc.GetGameState(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetGameState: %v", err)
	}

	if len(state.ContentWarnings) == 0 {
		t.Fatal("expected content warnings on state, got none")
	}

	foundDigestWarn := false
	for _, w := range state.ContentWarnings {
		if strings.Contains(w, "digest changed") {
			foundDigestWarn = true
			break
		}
	}
	if !foundDigestWarn {
		t.Fatalf("expected warning mentioning digest changed, got: %v", state.ContentWarnings)
	}
}
