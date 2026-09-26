package desktop

import (
	"strconv"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

const (
	dockWidth  = 72.0
	heroHeight = 300.0
)

// launcherView is the campaign launcher: a dock of campaigns, a hero panel for
// the selected one, and a campaign list.
func launcherView() {
	p := ui.DefaultPalette()
	Container(Attrs(Viewport, BackgroundVec(p.Bg)), func() {
		Container(Attrs(Row, Expand, Grow(1), Clip), func() {
			dockView(p)
			if appState.WorldFlyoutOpen {
				worldFlyout()
			}
			Container(Attrs(Grow(1), Expand, Clip, Pad(24), Gap(12)), func() {
				heroView(p)
				campaignListView(p)
			})
		})
	})
}

func dockView(p ui.Palette) {
	Container(Attrs(FixWidth(dockWidth), Expand, Clip, BackgroundVec(p.Panel), Pad(8), Gap(8)), func() {
		NextAccessName("launcher.new-campaign")
		if Button(NoIcon, "+") {
			appState.Screen = ScreenNewCampaign
		}
		AssignAccess()

		NextAccessName("launcher.worlds")
		if Button(NoIcon, "W") {
			appState.WorldFlyoutOpen = !appState.WorldFlyoutOpen
		}
		AssignAccess()

		for i := range appState.Games {
			game := &appState.Games[i]
			selected := game.ID == appState.Selected
			Container(Attrs(FixWidth(48), FixHeight(48), Corners(8), Clip), func() {
				if selected {
					ModAttrs(BackgroundVec(p.Accent))
				} else if IsHovered() {
					ModAttrs(BackgroundVec(p.Border))
				}
				NextAccessName("launcher.game." + game.ID)
				if PressAction() {
					appState.Selected = game.ID
				}
				AssignAccess()
				art := appState.gameArt(game.ID)
				if art.Icon != "" {
					Image(art.Icon, Vec2{48, 48})
				} else {
					Label(initial(game.Name), FontSize(18), FontWeight(WeightBold), TextColorVec(p.Text))
				}
			})
		}
	})
}

func heroView(p ui.Palette) {
	game := appState.SelectedGame()
	Container(Attrs(Expand, FixHeight(heroHeight), Corners(12), Clip, BackgroundVec(p.Panel), Pad(20), Gap(6)), func() {
		if game == nil {
			Label("No campaigns yet", FontSize(20), FontWeight(WeightBold), TextColorVec(p.Text))
			Label("Create one with the + button.", FontSize(13), TextColorVec(p.Muted))
			return
		}
		art := appState.gameArt(game.ID)
		artTile(p, art.Banner, initial(game.Name), heroHeight/2)
		Label(game.Name, FontSize(28), FontWeight(WeightBold), TextColorVec(p.Text))
		Label(appState.WorldName(game.WorldID)+" · "+appState.SystemName(game.SystemID), FontSize(14), TextColorVec(p.Muted))
		Spacer(8)
		Label(game.PlayerName+" · "+turnLabel(game.TurnCount), FontSize(13), TextColorVec(p.Muted))
		Spacer(8)
		NextAccessName("launcher.settings")
		if Button(NoIcon, "Settings") {
			openSettings(game.ID)
		}
		AssignAccess()
	})
}

func campaignListView(p ui.Palette) {
	Label("Campaigns", FontSize(15), FontWeight(WeightBold), TextColorVec(p.Text))
	Spacer(8)
	if len(appState.Games) == 0 {
		Label("No campaigns yet.", FontSize(13), TextColorVec(p.Muted))
		return
	}
	for i := range appState.Games {
		game := &appState.Games[i]
		selected := game.ID == appState.Selected
		Container(Attrs(Expand, FixHeight(56), Corners(8), Pad(12), BackgroundVec(p.Panel)), func() {
			if selected {
				ModAttrs(BackgroundVec(p.Border))
			} else if IsHovered() {
				ModAttrs(BackgroundVec(p.Border))
			}
			NextAccessName("launcher.campaign." + game.ID)
			if PressAction() {
				appState.Selected = game.ID
			}
			AssignAccess()
			Container(Attrs(Row, CrossMid, Gap(10)), func() {
				Label(game.Name, FontSize(15), FontWeight(WeightBold), TextColorVec(p.Text))
				Filler(1)
				Label(appState.WorldName(game.WorldID), FontSize(12), TextColorVec(p.Muted))
			})
		})
		Spacer(6)
	}
}

// artTile draws an image at a fixed height, preserving aspect ratio, or falls
// back to a labelled tile when no file exists.
func artTile(p ui.Palette, path, fallback string, height float32) {
	Container(Attrs(Expand, FixHeight(height), Corners(8), Clip, BackgroundVec(p.Border)), func() {
		if path != "" {
			if IsClicked() {
				appState.LightboxPath = path
			}
			Image(path, Vec2{GetContentWidth(), height})
			return
		}
		Container(Attrs(Expand, FixHeight(height), Center), func() {
			Label(fallback, FontSize(height/3), FontWeight(WeightBold), TextColorVec(p.Muted))
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
