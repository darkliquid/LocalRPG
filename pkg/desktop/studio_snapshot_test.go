package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestSystemsStudioSnapshot(t *testing.T) {
	appState = &State{
		Loaded: true,
		Screen: ScreenSystemsStudio,
		Systems: []gui.SystemSummaryDTO{
			{ID: "blades-in-the-dark", Name: "Blades in the Dark", Description: "Scoundrels in a haunted city.", Version: "1.0"},
			{ID: "masks", Name: "Masks", Description: "Teen heroes in a city of masks.", Version: "2.1"},
		},
		Studio:      Selection{Kind: "saved", ID: "blades-in-the-dark"},
		SystemTab:   "manifest",
		FormSysName: "Blades in the Dark",
		FormSysDesc: "Scoundrels in a haunted city.",
		FormSysSlug: "blades-in-the-dark",
	}
	ui.Snapshot(t, "systems_studio", 1000, 700, RootView)
}

func TestWorldsStudioSnapshot(t *testing.T) {
	appState = &State{
		Loaded: true,
		Screen: ScreenWorldsStudio,
		Worlds: []gui.WorldSummaryDTO{
			{ID: "sundered-realm", Name: "The Sundered Realm", Description: "A broken continent.", Genre: "dark fantasy"},
			{ID: "neon-city", Name: "Neon City", Description: "Rain and chrome.", Genre: "cyberpunk"},
		},
		Studio:         Selection{Kind: "saved", ID: "sundered-realm"},
		WorldTab:       "lore",
		FormWorldName:  "The Sundered Realm",
		FormWorldDesc:  "A broken continent.",
		FormWorldSlug:  "sundered-realm",
		FormWorldGenre: "dark fantasy",
	}
	ui.Snapshot(t, "worlds_studio", 1000, 700, RootView)
}
