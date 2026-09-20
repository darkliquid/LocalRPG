package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifests(t *testing.T) {
	tempDir := t.TempDir()

	systemYAML := `
id: "d20-classic"
name: "D20 Classic"
version: "1.0.0"
`
	systemPath := filepath.Join(tempDir, "system.yaml")
	if err := os.WriteFile(systemPath, []byte(systemYAML), 0644); err != nil {
		t.Fatalf("failed to write system.yaml: %v", err)
	}

	sys, err := LoadSystemManifest(systemPath)
	if err != nil {
		t.Fatalf("LoadSystemManifest failed: %v", err)
	}
	if sys.ID != "d20-classic" || sys.Name != "D20 Classic" || sys.Version != "1.0.0" {
		t.Errorf("unexpected system manifest: %+v", sys)
	}
}

func TestResolvePaths(t *testing.T) {
	baseDir := "/test/rpg"
	paths := NewPathResolver(baseDir)

	if paths.SystemDir("d20-classic") != "/test/rpg/systems/d20-classic" {
		t.Errorf("unexpected system dir: %s", paths.SystemDir("d20-classic"))
	}
	if paths.WorldDir("forgotten-reach") != "/test/rpg/worlds/forgotten-reach" {
		t.Errorf("unexpected world dir: %s", paths.WorldDir("forgotten-reach"))
	}
	if paths.GameDir("campaign-01") != "/test/rpg/games/campaign-01" {
		t.Errorf("unexpected game dir: %s", paths.GameDir("campaign-01"))
	}
}

func TestPathResolver_CustomPaths(t *testing.T) {
	resolver := NewCustomPathResolver("/custom/sys", "/custom/worlds", "/custom/games", "/custom/cache")
	if resolver.SystemsDir() != "/custom/sys" {
		t.Errorf("expected /custom/sys, got %q", resolver.SystemsDir())
	}
	if resolver.WorldsDir() != "/custom/worlds" {
		t.Errorf("expected /custom/worlds, got %q", resolver.WorldsDir())
	}
	if resolver.GamesDir() != "/custom/games" {
		t.Errorf("expected /custom/games, got %q", resolver.GamesDir())
	}
	if resolver.CacheDir() != "/custom/cache" {
		t.Errorf("expected /custom/cache, got %q", resolver.CacheDir())
	}
	if resolver.SystemDir("d20") != "/custom/sys/d20" {
		t.Errorf("expected /custom/sys/d20, got %q", resolver.SystemDir("d20"))
	}

	resolver.SetPaths("/new/sys", "/new/worlds", "/new/games", "/new/cache")
	if resolver.SystemsDir() != "/new/sys" {
		t.Errorf("expected /new/sys, got %q", resolver.SystemsDir())
	}
	if resolver.CacheDir() != "/new/cache" {
		t.Errorf("expected /new/cache, got %q", resolver.CacheDir())
	}
}
