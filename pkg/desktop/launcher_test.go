package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestLauncherSnapshot(t *testing.T) {
	appState = &State{
		Loaded:   true,
		Selected: "campaign-01",
		Games: []gui.GameSummaryDTO{
			{ID: "campaign-01", Name: "The Hollow Crown", WorldID: "realm", SystemID: "dnd5e", PlayerName: "Vance", TurnCount: 12},
			{ID: "campaign-02", Name: "Salt and Ash", WorldID: "realm", SystemID: "dnd5e", PlayerName: "Mira", TurnCount: 3},
		},
		Worlds:  []gui.WorldSummaryDTO{{ID: "realm", Name: "The Sundered Realm"}},
		Systems: []gui.SystemSummaryDTO{{ID: "dnd5e", Name: "Dungeons & Dragons 5e"}},
	}
	ui.Snapshot(t, "launcher", 1280, 800, RootView)
}
