package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestWorldGallerySnapshot(t *testing.T) {
	appState = &State{
		Loaded: true,
		Screen: ScreenWorldGallery,
		Worlds: []gui.WorldSummaryDTO{
			{ID: "realm", Name: "The Sundered Realm", Genre: "fantasy", Description: "A broken continent."},
			{ID: "void", Name: "The Void Between", Genre: "sci-fi", Description: "Silence and stars."},
		},
	}
	ui.Snapshot(t, "world_gallery", 1280, 800, RootView)
}
