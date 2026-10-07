package engine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/content"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
)

func setupTestEnvironment(t *testing.T) (*core.PathResolver, string, string) {
	t.Helper()
	dir := t.TempDir()
	paths := core.NewPathResolver(dir)

	sysID := "test_sys"
	worldID := "test_world"

	sysDir := paths.SystemDir(sysID)
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		t.Fatal(err)
	}
	sysYAML := "id: " + sysID + "\nname: Test System\nversion: 1.0.0\n"
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte(sysYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "mechanics.js"), []byte("// mechanics"), 0644); err != nil {
		t.Fatal(err)
	}

	worldDir := paths.WorldDir(worldID)
	if err := os.MkdirAll(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	worldYAML := "id: " + worldID + "\nname: Test World\nversion: 1.0.0\n"
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}

	return paths, sysID, worldID
}

func TestInitGameWritesLock(t *testing.T) {
	paths, sysID, worldID := setupTestEnvironment(t)
	gameID := "game_lock_test"

	session, err := engine.InitGame(paths, engine.InitOptions{
		GameID:     gameID,
		Name:       "Lock Test",
		SystemID:   sysID,
		WorldID:    worldID,
		PlayerName: "Hero",
	})
	if err != nil {
		t.Fatalf("InitGame: %v", err)
	}
	defer session.Close()

	lockPath := filepath.Join(paths.GameDir(gameID), "content.lock.yaml")
	lock, err := content.LoadLock(lockPath)
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}

	sysEntry, ok := lock.FindEntry("system", sysID)
	if !ok || sysEntry.SHA256 == "" || sysEntry.Version != "1.0.0" {
		t.Fatalf("expected system entry in lock, got: %+v (found=%v)", sysEntry, ok)
	}

	worldEntry, ok := lock.FindEntry("world", worldID)
	if !ok || worldEntry.SHA256 == "" || worldEntry.Version != "1.0.0" {
		t.Fatalf("expected world entry in lock, got: %+v (found=%v)", worldEntry, ok)
	}
}

func TestOpenWarnsOnChangedDigest(t *testing.T) {
	paths, sysID, worldID := setupTestEnvironment(t)
	gameID := "game_warn_test"

	session, err := engine.InitGame(paths, engine.InitOptions{
		GameID:     gameID,
		Name:       "Warn Test",
		SystemID:   sysID,
		WorldID:    worldID,
		PlayerName: "Hero",
	})
	if err != nil {
		t.Fatalf("InitGame: %v", err)
	}
	session.Close()

	// Modify mechanics.js
	mechPath := filepath.Join(paths.SystemDir(sysID), "mechanics.js")
	if err := os.WriteFile(mechPath, []byte("// modified mechanics"), 0644); err != nil {
		t.Fatal(err)
	}

	_, warnings, err := engine.ResolveContentLock(paths, gameID)
	if err != nil {
		t.Fatalf("ResolveContentLock returned unexpected error: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warnings for changed digest, got none")
	}

	foundDigestWarn := false
	for _, w := range warnings {
		if strings.Contains(w, "digest changed") {
			foundDigestWarn = true
			break
		}
	}
	if !foundDigestWarn {
		t.Fatalf("expected warning mentioning digest changed, got: %v", warnings)
	}
}

func TestOpenRefusesUnsatisfiedRequirement(t *testing.T) {
	paths, sysID, worldID := setupTestEnvironment(t)
	gameID := "game_refuse_test"

	// World requires system <2.0.0
	worldYAML := "id: " + worldID + "\nname: Test World\nversion: 1.0.0\nrequires:\n  - type: system\n    id: " + sysID + "\n    version: '<2.0.0'\n"
	if err := os.WriteFile(filepath.Join(paths.WorldDir(worldID), "world.yaml"), []byte(worldYAML), 0644); err != nil {
		t.Fatal(err)
	}

	session, err := engine.InitGame(paths, engine.InitOptions{
		GameID:     gameID,
		Name:       "Refuse Test",
		SystemID:   sysID,
		WorldID:    worldID,
		PlayerName: "Hero",
	})
	if err != nil {
		t.Fatalf("InitGame: %v", err)
	}
	session.Close()

	// Update system to 2.0.0 (incompatible with <2.0.0)
	sysYAML := "id: " + sysID + "\nname: Test System\nversion: 2.0.0\n"
	if err := os.WriteFile(filepath.Join(paths.SystemDir(sysID), "system.yaml"), []byte(sysYAML), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err = engine.ResolveContentLock(paths, gameID)
	if err == nil {
		t.Fatal("expected error for unsatisfied requirement, got nil")
	}
	if !strings.Contains(err.Error(), "incompatible system") && !strings.Contains(err.Error(), "requires") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestLegacyCampaignWithoutLockLocksOnOpen(t *testing.T) {
	paths, sysID, worldID := setupTestEnvironment(t)
	gameID := "game_legacy_test"

	session, err := engine.InitGame(paths, engine.InitOptions{
		GameID:     gameID,
		Name:       "Legacy Test",
		SystemID:   sysID,
		WorldID:    worldID,
		PlayerName: "Hero",
	})
	if err != nil {
		t.Fatalf("InitGame: %v", err)
	}
	session.Close()

	// Simulate a pre-lock campaign by deleting content.lock.yaml
	lockPath := filepath.Join(paths.GameDir(gameID), "content.lock.yaml")
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("remove lock: %v", err)
	}

	// ResolveContentLock should automatically re-create the lock without warnings
	lock, warnings, err := engine.ResolveContentLock(paths, gameID)
	if err != nil {
		t.Fatalf("ResolveContentLock on unlocked campaign: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected 0 warnings on fresh lock, got: %v", warnings)
	}
	if len(lock.Entries) != 2 {
		t.Fatalf("expected 2 lock entries, got: %d", len(lock.Entries))
	}

	// Verify file was written to disk
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected content.lock.yaml to exist on disk: %v", err)
	}
}
