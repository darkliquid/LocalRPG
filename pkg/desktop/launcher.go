package desktop

import (
	"strconv"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

const (
	dockWidth  = 76.0
	heroHeight = 300.0
)

// launcherView is the campaign launcher: a dock of campaigns, a hero panel for
// the selected one, and a campaign list.
func launcherView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg)), func() {
		Container(Attrs(Row, Expand, Grow(1), Clip), func() {
			dockView()
			if appState.WorldFlyoutOpen {
				worldFlyout()
			}
			Container(Attrs(Grow(1), Expand, Clip, Pad(28), Gap(16)), func() {
				heroView()
				campaignListView()
			})
		})
	})
}

func dockView() {
	Container(Attrs(FixWidth(dockWidth), Expand, Clip, ui.GlassSurface(), Pad(10), Gap(10)), func() {
		dockButton("launcher.new-campaign", "+", func() { appState.Screen = ScreenNewCampaign })
		dockButton("launcher.worlds", "W", func() { appState.WorldFlyoutOpen = !appState.WorldFlyoutOpen })
		dockButton("launcher.systems-studio", "S", openSystemsStudio)
		dockButton("launcher.global-settings", "⚙", openGlobalSettings)

		for i := range appState.Games {
			game := &appState.Games[i]
			selected := game.ID == appState.Selected
			art := appState.gameArt(game.ID)
			Container(Attrs(FixWidth(52), FixHeight(52), Corners(12), Clip), func() {
				if selected {
					ModAttrs(BorderWidth(2), BorderColorVec(ui.Accent))
				} else {
					ModAttrs(BorderWidth(1), BorderColorVec(ui.Hairline))
				}
				NextAccessName("launcher.game." + game.ID)
				if PressAction() {
					appState.Selected = game.ID
				}
				AssignAccess()
				if art.Icon != "" {
					Image(art.Icon, Vec2{52, 52})
				} else {
					Container(Attrs(Expand, Expand, Center, BackgroundVec(ui.RaisedBG)), func() {
						Label(initial(game.Name), Fonts(ui.SansStack...), FontSize(20), FontWeight(WeightBold), TextColorVec(ui.TextMain))
					})
				}
			})
		}
	})
}

func dockButton(name, label string, action func()) {
	Container(Attrs(FixWidth(52), FixHeight(40), Corners(10), ui.HairlineBorder(), Center,
		BackgroundVec(ui.RaisedBG)), func() {
		if IsHovered() {
			ModAttrs(BorderColorVec(ui.Accent))
		}
		NextAccessName(name)
		if PressAction() {
			action()
		}
		AssignAccess()
		Label(label, Fonts(ui.SansStack...), FontSize(16), FontWeight(WeightBold), TextColorVec(ui.TextMain))
	})
}

func heroView() {
	game := appState.SelectedGame()
	Container(Attrs(ui.Card(Expand, FixHeight(heroHeight), Clip, Gap(8))), func() {
		if game == nil {
			Label("No campaigns yet", Fonts(ui.SansStack...), FontSize(22), FontWeight(WeightBold), TextColorVec(ui.TextMain))
			Label("Create one with the + button.", FontSize(14), TextColorVec(ui.TextMuted))
			return
		}
		art := appState.gameArt(game.ID)
		artTile(ui.DefaultPalette(), art.Banner, initial(game.Name), 150)
		Label(game.Name, Fonts(ui.SansStack...), FontSize(30), FontWeight(WeightBold), TextColorVec(ui.TextMain))
		Label(appState.WorldName(game.WorldID)+"  ·  "+appState.SystemName(game.SystemID), FontSize(15), TextColorVec(ui.TextMuted))
		Label(game.PlayerName+"  ·  "+turnLabel(game.TurnCount), FontSize(13), TextColorVec(ui.TextFaint))
		Spacer(4)
		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			NextAccessName("launcher.open")
			if Button(NoIcon, "Open") {
				openCampaign(game.ID)
			}
			AssignAccess()
			NextAccessName("launcher.settings")
			if Button(NoIcon, "Settings") {
				openSettings(game.ID)
			}
			AssignAccess()
		})
	})
}

func campaignListView() {
	Label("Campaigns", Fonts(ui.SansStack...), FontSize(16), FontWeight(WeightBold), TextColorVec(ui.TextMain))
	Spacer(6)
	if len(appState.Games) == 0 {
		Label("No campaigns yet.", FontSize(14), TextColorVec(ui.TextMuted))
		return
	}
	for i := range appState.Games {
		game := &appState.Games[i]
		selected := game.ID == appState.Selected
		Container(Attrs(Row, CrossMid, Gap(12), Expand, FixHeight(64), Corners(12), Pad2(14, 18),
			BackgroundVec(ui.CardBG), BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
			if selected {
				ModAttrs(BorderColorVec(ui.Accent))
			} else if IsHovered() {
				ModAttrs(BorderColorVec(ui.TextFaint))
			}
			NextAccessName("launcher.campaign." + game.ID)
			if PressAction() {
				appState.Selected = game.ID
			}
			AssignAccess()
			Label(game.Name, Fonts(ui.SansStack...), FontSize(16), FontWeight(WeightBold), TextColorVec(ui.TextMain))
			Filler(1)
			Label(appState.WorldName(game.WorldID), FontSize(13), TextColorVec(ui.TextMuted))
		})
		Spacer(8)
	}
}

// artTile draws an image at a fixed height, preserving aspect ratio, or falls
// back to a labelled tile when no file exists.
func artTile(p ui.Palette, path, fallback string, height float32) {
	Container(Attrs(Expand, FixHeight(height), Corners(12), Clip, ui.HairlineBorder(), BackgroundVec(ui.RaisedBG)), func() {
		if path != "" {
			if IsClicked() {
				appState.LightboxPath = path
			}
			Image(path, Vec2{GetContentWidth(), height})
			return
		}
		Container(Attrs(Expand, FixHeight(height), Center), func() {
			Label(fallback, Fonts(ui.SansStack...), FontSize(height/3), FontWeight(WeightBold), TextColorVec(ui.TextFaint))
		})
	})
}

// initial returns the first rune of a name for the dock tile.
func initial(name string) string {
	for _, r := range name {
		return string(r)
	}
	return "?"
}

// turnLabel renders a turn count.
func turnLabel(n int) string {
	if n == 1 {
		return "1 turn"
	}
	return strconv.Itoa(n) + " turns"
}
