package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
)

func TestServiceGlobalModeUsesXDG(t *testing.T) {
	data := t.TempDir()
	cache := t.TempDir()
	cfgHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("LOCALRPG_CONFIG_DIR", "")
	xdg.Reload()
	t.Cleanup(xdg.Reload)

	svc := NewService(".")
	resolver := svc.GetResolver()
	if !strings.HasPrefix(resolver.SystemsDir(), data) {
		t.Errorf("SystemsDir = %q, want under %q", resolver.SystemsDir(), data)
	}
	if !strings.HasPrefix(resolver.CacheDir(), cache) {
		t.Errorf("CacheDir = %q, want under %q", resolver.CacheDir(), cache)
	}
	if _, err := os.Stat(filepath.Join(".", "systems")); err == nil {
		t.Error("global mode must not create ./systems")
	}
}

func TestServiceProjectModeStaysRelative(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)
	if got := svc.GetResolver().SystemsDir(); got != filepath.Join(root, "systems") {
		t.Fatalf("SystemsDir = %q, want under the project root", got)
	}
}
