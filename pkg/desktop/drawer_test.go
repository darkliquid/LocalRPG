package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestDrawerTabsToggle(t *testing.T) {
	appState = &State{Loaded: true, Drawer: ""}
	toggleDrawer("codex")
	if appState.Drawer != "codex" {
		t.Fatalf("Drawer = %q, want codex", appState.Drawer)
	}
	toggleDrawer("codex")
	if appState.Drawer != "" {
		t.Fatalf("re-toggling the same drawer must close it, got %q", appState.Drawer)
	}
	toggleDrawer("graph")
	if appState.Drawer != "graph" {
		t.Fatalf("Drawer = %q, want graph", appState.Drawer)
	}
}

func TestDrawerOpenSnapshot(t *testing.T) {
	appState = &State{
		Loaded: true,
		Screen: ScreenChronicle,
		Drawer: "codex",
		Turns: []gui.TurnDTO{
			{TurnNumber: 1, InputText: "look", Mode: "do", Segments: []gui.SegmentDTO{{Kind: "narration", Text: "A hall."}}},
		},
	}
	ui.Snapshot(t, "drawer", 1000, 700, RootView)
}
