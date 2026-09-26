package desktop

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestLoadAllIgnoresServiceErrors(t *testing.T) {
	// A fresh service with no games/systems/worlds directories must not error.
	svc := gui.NewService(t.TempDir())
	st := loadAll(context.Background(), svc)
	if !st.Loaded {
		t.Fatal("loadAll must mark the state loaded even when directories are empty")
	}
	if st.Games == nil || st.Worlds == nil || st.Systems == nil {
		t.Fatal("loadAll must return non-nil slices so views can range safely")
	}
	if st.Err != nil {
		t.Fatalf("loadAll reported %v", st.Err)
	}
	if st.GameArt == nil || st.WorldArt == nil {
		t.Fatal("loadAll must initialise the artwork maps")
	}
	// A campaign with no assets resolves to an empty Art, not an error.
	if got := st.gameArt("missing"); got.Banner != "" || got.Icon != "" {
		t.Fatalf("missing campaign art = %+v, want empty", got)
	}
}
