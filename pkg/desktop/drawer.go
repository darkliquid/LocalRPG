package desktop

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// toggleDrawer opens the named drawer, or closes it when already active.
// One drawer is visible at a time, matching the SPA.
func toggleDrawer(name string) {
	if appState.Drawer == name {
		appState.Drawer = ""
		return
	}
	appState.Drawer = name
}

// drawerToolbar offers the drawer entry points in the chronicle.
func drawerToolbar(p ui.Palette) {
	Container(Attrs(Row, Gap(6)), func() {
		for _, tab := range []struct{ key, label string }{
			{"codex", "Codex"}, {"context", "Context"}, {"graph", "Graph"},
			{"world", "World"}, {"character", "Character"},
		} {
			tab := tab
			NextAccessName("drawer.open." + tab.key)
			if Button(NoIcon, tab.label) {
				toggleDrawer(tab.key)
			}
			AssignAccess()
		}
	})
}

// drawerPanel renders the active drawer as a fixed-width column beside the
// chronicle. It is an ordinary layout container, not a popup.
func drawerPanel() {
	if appState.Drawer == "" {
		return
	}
	p := ui.DefaultPalette()
	Container(Attrs(FixWidth(420), Expand, Clip, BackgroundVec(p.Panel)), func() {
		Container(Attrs(Row, CrossMid, Gap(4), Pad(8)), func() {
			Label(drawerTitle(), FontSize(14), FontWeight(WeightBold), TextColorVec(p.Text))
			Filler(1)
			NextAccessName("drawer.close")
			if Button(NoIcon, "×") {
				appState.Drawer = ""
			}
			AssignAccess()
		})
		Element(Attrs(Expand, FixHeight(1), BackgroundVec(p.Border)))
		Container(Attrs(Viewport, Grow(1), Pad(12)), func() {
			ScrollOnInput()
			ScrollBars()
			switch appState.Drawer {
			case "codex":
				codexDrawer(p)
			case "context":
				contextDrawer(p)
			case "graph":
				graphDrawer(p)
			case "world":
				worldDrawer(p)
			case "character":
				characterDrawer(p)
			}
		})
	})
}

// drawerTitle returns the active drawer's display name.
func drawerTitle() string {
	switch appState.Drawer {
	case "codex":
		return "Codex"
	case "context":
		return "Context"
	case "graph":
		return "Lore Graph"
	case "world":
		return "Living World"
	case "character":
		return "Character Sheet"
	default:
		return "Panel"
	}
}

func contextDrawer(p ui.Palette)   { Label("Context", TextColorVec(p.Muted)) }
func graphDrawer(p ui.Palette)     { Label("Graph", TextColorVec(p.Muted)) }
func worldDrawer(p ui.Palette)     { Label("Living World", TextColorVec(p.Muted)) }
func characterDrawer(p ui.Palette) { Label("Character Sheet", TextColorVec(p.Muted)) }
