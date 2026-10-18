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
	baseDir := filepath.Join(string(filepath.Separator), "test", "rpg")
	paths := NewPathResolver(baseDir)

	if paths.SystemDir("d20-classic") != filepath.Join(baseDir, "systems", "d20-classic") {
		t.Errorf("unexpected system dir: %s", paths.SystemDir("d20-classic"))
	}
	if paths.WorldDir("forgotten-reach") != filepath.Join(baseDir, "worlds", "forgotten-reach") {
		t.Errorf("unexpected world dir: %s", paths.WorldDir("forgotten-reach"))
	}
	if paths.GameDir("campaign-01") != filepath.Join(baseDir, "games", "campaign-01") {
		t.Errorf("unexpected game dir: %s", paths.GameDir("campaign-01"))
	}
}

func TestPathResolver_CustomPaths(t *testing.T) {
	sysDir := filepath.Join(string(filepath.Separator), "custom", "sys")
	worldsDir := filepath.Join(string(filepath.Separator), "custom", "worlds")
	gamesDir := filepath.Join(string(filepath.Separator), "custom", "games")
	cacheDir := filepath.Join(string(filepath.Separator), "custom", "cache")
	resolver := NewCustomPathResolver(sysDir, worldsDir, gamesDir, cacheDir)
	if resolver.SystemsDir() != sysDir {
		t.Errorf("expected %q, got %q", sysDir, resolver.SystemsDir())
	}
	if resolver.WorldsDir() != worldsDir {
		t.Errorf("expected %q, got %q", worldsDir, resolver.WorldsDir())
	}
	if resolver.GamesDir() != gamesDir {
		t.Errorf("expected %q, got %q", gamesDir, resolver.GamesDir())
	}
	if resolver.CacheDir() != cacheDir {
		t.Errorf("expected %q, got %q", cacheDir, resolver.CacheDir())
	}
	if resolver.SystemDir("d20") != filepath.Join(sysDir, "d20") {
		t.Errorf("expected %q, got %q", filepath.Join(sysDir, "d20"), resolver.SystemDir("d20"))
	}

	newSys := filepath.Join(string(filepath.Separator), "new", "sys")
	newWorlds := filepath.Join(string(filepath.Separator), "new", "worlds")
	newGames := filepath.Join(string(filepath.Separator), "new", "games")
	newCache := filepath.Join(string(filepath.Separator), "new", "cache")
	resolver.SetPaths(newSys, newWorlds, newGames, newCache)
	if resolver.SystemsDir() != newSys {
		t.Errorf("expected %q, got %q", newSys, resolver.SystemsDir())
	}
	if resolver.CacheDir() != newCache {
		t.Errorf("expected %q, got %q", newCache, resolver.CacheDir())
	}
}

func TestPathResolverSanitizesIDs(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "app", "base")
	resolver := NewPathResolver(base)

	malicious := "../../etc/passwd"
	gameDir := resolver.GameDir(malicious)
	expected := filepath.Clean(filepath.Join(base, "games", "etc-passwd"))
	if filepath.Clean(gameDir) != expected {
		t.Errorf("GameDir(%q) = %q, expected %q", malicious, gameDir, expected)
	}

	worldDir := resolver.WorldDir(malicious)
	expectedWorld := filepath.Clean(filepath.Join(base, "worlds", "etc-passwd"))
	if filepath.Clean(worldDir) != expectedWorld {
		t.Errorf("WorldDir(%q) = %q, expected %q", malicious, worldDir, expectedWorld)
	}

	sysDir := resolver.SystemDir(malicious)
	expectedSys := filepath.Clean(filepath.Join(base, "systems", "etc-passwd"))
	if filepath.Clean(sysDir) != expectedSys {
		t.Errorf("SystemDir(%q) = %q, expected %q", malicious, sysDir, expectedSys)
	}
}

func TestManifestVersionMustBeSemver(t *testing.T) {
	if err := (SystemManifest{ID: "s", Version: "latest"}).Validate(); err == nil {
		t.Fatal("an invalid system version should be rejected")
	}
	if err := (SystemManifest{ID: "s", Version: "1.2.0"}).Validate(); err != nil {
		t.Fatalf("a valid system version was rejected: %v", err)
	}
	if err := (SystemManifest{ID: "s", Version: "v1.2.0"}).Validate(); err != nil {
		t.Fatalf("a valid system version with v prefix was rejected: %v", err)
	}
	if err := (SystemManifest{ID: "s", Version: ""}).Validate(); err != nil {
		t.Fatalf("an unversioned system manifest should be allowed: %v", err)
	}

	if err := (WorldManifest{ID: "w", Version: "latest"}).Validate(); err == nil {
		t.Fatal("an invalid world version should be rejected")
	}
	if err := (WorldManifest{ID: "w", Version: "1.2.0"}).Validate(); err != nil {
		t.Fatalf("a valid world version was rejected: %v", err)
	}
	if err := (WorldManifest{ID: "w", Version: ""}).Validate(); err != nil {
		t.Fatalf("an unversioned world manifest should be allowed: %v", err)
	}
}

func TestRequirementSatisfies(t *testing.T) {
	r := ContentRequirement{Type: "system", ID: "narrative_2d6", Version: ">=1.0.0 <2.0.0"}
	if !r.Satisfies("1.4.0") {
		t.Fatal("1.4.0 should satisfy the range")
	}
	if r.Satisfies("2.0.0") {
		t.Fatal("2.0.0 should not satisfy the range")
	}
	if !r.Satisfies("1.0.0") {
		t.Fatal("1.0.0 should satisfy >=1.0.0")
	}
	if r.Satisfies("0.9.0") {
		t.Fatal("0.9.0 should not satisfy >=1.0.0")
	}

	any := ContentRequirement{Type: "system", ID: "x"}
	if !any.Satisfies("0.0.0") {
		t.Fatal("an empty constraint should accept any version")
	}
	if !any.Satisfies("1.2.3") {
		t.Fatal("an empty constraint should accept any version")
	}
}
