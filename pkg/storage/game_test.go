package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestOpenGameStoreUsesCanonicalPathAndRetiresLegacyDB(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	t.Cleanup(func() { _ = CloseGameStores() })

	gameDir := paths.GameDir("campaign-01")
	if err := os.MkdirAll(gameDir, 0755); err != nil {
		t.Fatal(err)
	}

	legacy := filepath.Join(gameDir, "game.db")
	if err := os.WriteFile(legacy, []byte("legacy"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy+"-wal", []byte("legacy-wal"), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := OpenGameStore(paths, "campaign-01")
	if err != nil {
		t.Fatalf("OpenGameStore failed: %v", err)
	}

	if _, err := os.Stat(paths.GameDBPath("campaign-01")); err != nil {
		t.Errorf("expected canonical database at %s: %v", paths.GameDBPath("campaign-01"), err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("expected legacy game.db to be retired, stat err = %v", err)
	}
	if _, err := os.Stat(legacy + ".legacy"); err != nil {
		t.Errorf("expected legacy database to be renamed: %v", err)
	}
	if _, err := os.Stat(legacy + "-wal.legacy"); err != nil {
		t.Errorf("expected legacy WAL sidecar to be renamed: %v", err)
	}

	again, err := OpenGameStore(paths, "campaign-01")
	if err != nil {
		t.Fatalf("second OpenGameStore failed: %v", err)
	}
	if again != store {
		t.Errorf("expected the shared store for the canonical path")
	}
}

func TestOpenGameStoreRejectsInvalidIDs(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	t.Cleanup(func() { _ = CloseGameStores() })

	if _, err := OpenGameStore(paths, "../escaped"); err == nil {
		t.Error("OpenGameStore expected error for traversal gameID, got nil")
	}
	if err := CloseGameStore(paths, "../escaped"); err == nil {
		t.Error("CloseGameStore expected error for traversal gameID, got nil")
	}
}

