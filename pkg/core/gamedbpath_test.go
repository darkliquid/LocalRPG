package core

import (
	"path/filepath"
	"testing"
)

func TestGameDBPath(t *testing.T) {
	paths := NewPathResolver("/srv/localrpg")
	want := filepath.Join("/srv/localrpg", "games", "campaign-01", "cache", "index.db")
	if got := paths.GameDBPath("campaign-01"); got != want {
		t.Errorf("GameDBPath() = %q, want %q", got, want)
	}

	custom := NewCustomPathResolver("s", "w", filepath.Join("/data", "games"), "c")
	wantCustom := filepath.Join("/data", "games", "x", "cache", "index.db")
	if got := custom.GameDBPath("x"); got != wantCustom {
		t.Errorf("GameDBPath() = %q, want %q", got, wantCustom)
	}
}
