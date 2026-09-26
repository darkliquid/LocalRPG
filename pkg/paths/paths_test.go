package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestResolveGlobalDefaults(t *testing.T) {
	bases := Bases{
		Config: filepath.Join(t.TempDir(), "config"),
		Data:   filepath.Join(t.TempDir(), "data"),
		Cache:  filepath.Join(t.TempDir(), "cache"),
	}
	dirs := Resolve(bases, config.PathsConfig{}, "")

	if dirs.Systems != filepath.Join(bases.Data, "localrpg", "systems") {
		t.Errorf("Systems = %q", dirs.Systems)
	}
	if dirs.Games != filepath.Join(bases.Data, "localrpg", "games") {
		t.Errorf("Games = %q", dirs.Games)
	}
	if dirs.Cache != filepath.Join(bases.Cache, "localrpg") {
		t.Errorf("Cache = %q", dirs.Cache)
	}
	if filepath.Dir(dirs.Cache) == filepath.Dir(dirs.Systems) {
		t.Error("cache and data must use different bases")
	}
}

func TestResolveRelativeAndAbsolute(t *testing.T) {
	bases := Bases{Data: filepath.Join(t.TempDir(), "data"), Cache: filepath.Join(t.TempDir(), "cache")}

	global := Resolve(bases, config.PathsConfig{Systems: "shared/systems", Cache: "clips"}, "")
	if global.Systems != filepath.Join(bases.Data, "localrpg", "shared/systems") {
		t.Errorf("relative global Systems = %q", global.Systems)
	}
	if global.Cache != filepath.Join(bases.Cache, "localrpg", "clips") {
		t.Errorf("relative global Cache = %q", global.Cache)
	}

	abs := "/opt/localrpg/systems"
	if got := Resolve(bases, config.PathsConfig{Systems: abs}, "").Systems; got != abs {
		t.Errorf("absolute Systems = %q, want %q", got, abs)
	}
}

func TestResolveProjectMode(t *testing.T) {
	root := t.TempDir()
	bases := Bases{Data: filepath.Join(t.TempDir(), "data"), Cache: filepath.Join(t.TempDir(), "cache")}

	dirs := Resolve(bases, config.PathsConfig{Systems: "systems", Cache: "cache"}, root)
	if dirs.Systems != filepath.Join(root, "systems") {
		t.Errorf("project Systems = %q", dirs.Systems)
	}
	if dirs.Cache != filepath.Join(root, "cache") {
		t.Errorf("project Cache = %q", dirs.Cache)
	}

	empty := Resolve(bases, config.PathsConfig{}, root)
	if empty.Systems != filepath.Join(root, "systems") || empty.Cache != filepath.Join(root, "cache") {
		t.Errorf("project empty defaults = %+v", empty)
	}

	abs := filepath.Join(root, "elsewhere", "systems")
	if got := Resolve(bases, config.PathsConfig{Systems: abs}, root).Systems; got != abs {
		t.Errorf("absolute wins in project mode: %q", got)
	}
}

func TestLegacyWarningPointsAtTheWorkingDirectory(t *testing.T) {
	base := Bases{Data: filepath.Join(t.TempDir(), "data"), Cache: filepath.Join(t.TempDir(), "cache")}
	if got := LegacyWarning(base, config.PathsConfig{}); got != "" {
		t.Fatalf("no legacy directory exists, got %q", got)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "systems"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if got := LegacyWarning(base, config.PathsConfig{}); got == "" {
		t.Fatal("expected a warning for a legacy ./systems")
	}
}
