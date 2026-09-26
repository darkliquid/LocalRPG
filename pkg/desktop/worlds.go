package desktop

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// worldsView is the full-screen world browser. Selecting a world opens the new
// campaign form for it.
func worldsView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg), Pad(24), Gap(12)), func() {
		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			Label("Worlds", FontSize(24), FontWeight(WeightBold), TextColorVec(p.Text))
			Filler(1)
			NextAccessName("worlds.new")
			if Button(NoIcon, "New World") {
				// Studio work lands in a later plan; this is a stub entry point.
			}
			AssignAccess()
			NextAccessName("worlds.close")
			if Button(NoIcon, "Back") {
				appState.Screen = ScreenLauncher
			}
			AssignAccess()
		})

		if len(appState.Worlds) == 0 {
			Label("No worlds yet.", FontSize(14), TextColorVec(p.Muted))
			return
		}
		Container(Attrs(Wrap, Gap(16)), func() {
			for i := range appState.Worlds {
				w := &appState.Worlds[i]
				art := appState.worldArt(w.ID)
				Container(Attrs(FixWidth(280), Corners(10), Clip, BackgroundVec(p.Panel)), func() {
					NextAccessName("worlds.world." + w.ID)
					if PressAction() {
						appState.PendingWorld = w.ID
						appState.Screen = ScreenNewCampaign
						if newForm.Name == "" {
							newForm.Name = "Chronicles of " + w.Name
						}
					}
					AssignAccess()
					artTile(p, art.Banner, initial(w.Name), 140)
					Container(Attrs(Pad(12), Gap(6)), func() {
						Label(w.Name, FontSize(16), FontWeight(WeightBold), TextColorVec(p.Text))
						Label(w.Genre, FontSize(12), TextColorVec(p.Muted))
						Label(w.Description, FontSize(12), TextColorVec(p.Muted))
					})
				})
			}
		})
	})
}

// worldFlyout is the compact vertical strip used beside the dock.
func worldFlyout() {
	p := ui.DefaultPalette()
	Container(Attrs(FixWidth(56), Expand, Clip, BackgroundVec(p.Panel), Pad(8), Gap(8)), func() {
		for i := range appState.Worlds {
			w := &appState.Worlds[i]
			art := appState.worldArt(w.ID)
			Container(Attrs(FixWidth(40), FixHeight(40), Corners(8), Clip), func() {
				if IsHovered() {
					ModAttrs(BackgroundVec(p.Border))
				}
				NextAccessName("worldflyout." + w.ID)
				if PressAction() {
					appState.PendingWorld = w.ID
					appState.Screen = ScreenNewCampaign
				}
				AssignAccess()
				if art.Icon != "" {
					Image(art.Icon, Vec2{40, 40})
				} else {
					Label(initial(w.Name), FontSize(16), TextColorVec(p.Text))
				}
			})
		}
		NextAccessName("worldflyout.expand")
		if Button(NoIcon, "›") {
			appState.Screen = ScreenWorldGallery
		}
		AssignAccess()
	})
}
