package desktop

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// drawerTab names one side panel and its trigger pill.
type drawerTab struct {
	key   string
	label string
	icon  IconGlyph
}

var drawerTabs = []drawerTab{
	{"character", "Character", TypUser},
	{"graph", "Graph", TypFlowMerge},
	{"codex", "Codex", TypBook},
	{"world", "World Arcs", TypTime},
	{"context", "Context", TypThList},
}

// toggleDrawer opens the named drawer, or closes it when already active.
// One drawer is visible at a time, matching the SPA.
func toggleDrawer(name string) {
	if appState.Drawer == name {
		appState.Drawer = ""
		return
	}
	appState.Drawer = name
	switch name {
	case "context":
		refreshContext()
	case "graph":
		refreshGraph()
	case "world":
		refreshWorld()
	}
}

// drawerToolbar offers the drawer entry points in the chronicle header. Labels
// collapse to icons on narrow windows, matching the original's responsive pills.
func drawerToolbar(p ui.Palette, showLabels bool) {
	Container(Attrs(Row, CrossMid, Gap(2)), func() {
		for _, tab := range drawerTabs {
			tab := tab
			active := appState.Drawer == tab.key
			Container(Attrs(Row, CrossMid, Gap(6), Corners(12), Pad2(8, 10)), func() {
				switch {
				case active:
					ModAttrs(BackgroundVec(ui.AccentBtn), BoxShadow(14))
				case IsHovered():
					ModAttrs(BackgroundVec(ui.HoverFill))
				}
				NextAccessName("drawer.open." + tab.key)
				if PressAction() {
					toggleDrawer(tab.key)
				}
				AssignAccess()
				Icon(tab.icon, FontSize(14), TextColorVec(ui.TextMain))
				if showLabels {
					Label(tab.label, Fonts(ui.SansStack...), FontSize(12), TextColorVec(ui.TextMain))
				}
			})
		}
	})
}

// drawerWidth is the panel width per drawer, mirroring the SPA's md/xl sizes.
func drawerWidth() float32 {
	switch appState.Drawer {
	case "codex", "graph", "context":
		return 560
	default:
		return 420
	}
}

// drawerPanel renders the active drawer as a right-side overlay: a dimming
// scrim plus a translucent glass panel.
func drawerPanel() {
	if appState.Drawer == "" {
		return
	}
	w := GetContentWidth()
	h := GetContentHeight()
	if w < 1 || h < 1 {
		RequestNextFrame()
		return
	}
	pw := drawerWidth()
	if pw > w {
		pw = w
	}

	Container(Attrs(FixSize(w, h), Float(0, 0), Clip), func() {
		Container(Attrs(Expand, Expand, BackgroundVec(Vec4{0, 0, 0, 0.6})), func() {
			NextAccessName("drawer.scrim")
			if PressAction() {
				appState.Drawer = ""
			}
			AssignAccess()
		})
	})

	Container(Attrs(FixSize(pw, h), Float(w-pw, 0), Clip, BackgroundVec(ui.DrawerBG),
		BorderWidth(1), BorderColorVec(ui.Hairline), BoxShadow(30), Pad(24), Gap(16)), func() {
		Container(Attrs(Row, CrossMid, Expand), func() {
			Label(drawerTitle(), Fonts(ui.SansStack...), FontSize(16), FontWeight(WeightBold), TextColorVec(ui.Accent))
			Filler(1)
			Container(Attrs(FixWidth(32), FixHeight(32), Corners(10), Center), func() {
				if IsHovered() {
					ModAttrs(BackgroundVec(ui.HoverFill))
				}
				NextAccessName("drawer.close")
				if PressAction() {
					appState.Drawer = ""
				}
				AssignAccess()
				Icon(TypTimes, FontSize(18), TextColorVec(ui.TextMuted))
			})
		})
		Element(Attrs(Expand, FixHeight(1), BackgroundVec(ui.Hairline)))
		Container(Attrs(Viewport, Grow(1), Clip), func() {
			ScrollOnInput()
			ScrollBars()
			switch appState.Drawer {
			case "codex":
				codexDrawer(ui.DefaultPalette())
			case "context":
				contextDrawer(ui.DefaultPalette())
			case "graph":
				graphDrawer(ui.DefaultPalette())
			case "world":
				worldDrawer(ui.DefaultPalette())
			case "character":
				characterDrawer(ui.DefaultPalette())
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

// (drawer bodies live in their own files: codex.go, context.go, graph.go, world.go)
