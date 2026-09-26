package desktop

import (
	"fmt"
	"strconv"
	"time"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

const dockWidth = 72.0

// launcherView is the campaign launcher: a left dock and a full-bleed hero
// stage for the selected campaign.
func launcherView() {
	Container(Attrs(Row, Expand, Grow(1), Clip, BackgroundVec(ui.CanvasBG)), func() {
		dockView()
		Element(Attrs(FixWidth(1), Expand, BackgroundVec(ui.Hairline)))
		Container(Attrs(Grow(1), Expand, Clip), func() {
			heroStage()
		})
	})
	if appState.CampaignGalleryOpen {
		campaignGallery()
	}
}

func dockView() {
	Container(Attrs(FixWidth(dockWidth), Expand, Clip, BackgroundVec(ui.DockBG), Pad(12), Gap(10)), func() {
		Container(Attrs(Expand, Center), func() {
			squareButton("launcher.new-campaign", TypPlus, appState.WorldFlyoutOpen, func() {
				appState.WorldFlyoutOpen = !appState.WorldFlyoutOpen
			}, true)
		})
		separator32()
		Container(Attrs(Expand, Center), func() {
			squareButton("launcher.gallery", SymGrid, appState.CampaignGalleryOpen, func() {
				appState.CampaignGalleryOpen = !appState.CampaignGalleryOpen
			}, false)
		})

		Container(Attrs(Expand, Grow(1), Clip), func() {
			ScrollOnInput()
			Container(Attrs(Expand, Gap(12), Pad2(4, 0)), func() {
				for i := range appState.Games {
					game := &appState.Games[i]
					selected := game.ID == appState.Selected
					art := appState.gameArt(game.ID)
					Container(Attrs(FixWidth(46), FixHeight(46), Corners(16), Clip), func() {
						if selected {
							ModAttrs(BorderWidth(2), BorderColorVec(ui.Accent))
						} else {
							ModAttrs(BorderWidth(1), BorderColorVec(ui.Hairline), Trans(0.72))
						}
						if IsHovered() && !selected {
							ModAttrs(Trans(1))
						}
						NextAccessName("launcher.game." + game.ID)
						if PressAction() {
							appState.Selected = game.ID
						}
						AssignAccess()
						if art.Icon != "" {
							coverImage("dock:"+game.ID, art.Icon, 46, 46)
						} else {
							Container(Attrs(Expand, Expand, Center, BackgroundVec(ui.RaisedBG)), func() {
								Label(initial(game.Name), Fonts(ui.SansStack...), FontSize(19), FontWeight(WeightBold), TextColorVec(ui.TextMain))
							})
						}
					})
				}
			})
		})

		separator32()
		Container(Attrs(Expand, Center), func() { iconButton("launcher.worlds-studio", TypGlobe, openWorldsStudio) })
		Container(Attrs(Expand, Center), func() { iconButton("launcher.systems-studio", TypBook, openSystemsStudio) })
		Container(Attrs(Expand, Center), func() { iconButton("launcher.global-settings", TypCog, openGlobalSettings) })
	})
}

// squareButton is the dock's 46x46 action tile. A dashed border is unavailable,
// so the "new" affordance leans on a faint border and the accent when active.
func squareButton(name string, icon IconGlyph, active bool, action func(), dashed bool) {
	Container(Attrs(FixWidth(46), FixHeight(46), Corners(16), Center), func() {
		switch {
		case active:
			ModAttrs(BackgroundVec(ui.AccentSoft), BorderWidth(2), BorderColorVec(ui.Accent))
		case dashed:
			ModAttrs(BackgroundVec(ui.HoverFill), BorderWidth(1), BorderColorVec(ui.TextFaint))
		default:
			ModAttrs(BackgroundVec(ui.HoverFill), BorderWidth(1), BorderColorVec(ui.Hairline))
		}
		if IsHovered() && !active {
			ModAttrs(BorderColorVec(ui.Accent))
		}
		NextAccessName(name)
		if PressAction() {
			action()
		}
		AssignAccess()
		Icon(icon, FontSize(20), TextColorVec(ui.TextMain))
	})
}

func iconButton(name string, icon IconGlyph, action func()) {
	Container(Attrs(FixWidth(40), FixHeight(40), Corners(12), Center), func() {
		if IsHovered() {
			ModAttrs(BackgroundVec(ui.HoverFill))
		}
		NextAccessName(name)
		if PressAction() {
			action()
		}
		AssignAccess()
		Icon(icon, FontSize(18), TextColorVec(ui.TextMuted))
	})
}

func separator32() {
	Container(Attrs(Expand, Center), func() {
		Element(Attrs(FixWidth(32), FixHeight(1), BackgroundVec(ui.Hairline)))
	})
}

func heroStage() {
	game := appState.SelectedGame()
	Container(Attrs(Grow(1), Expand, Clip, BackgroundVec(ui.CanvasBG)), func() {
		w := GetContentWidth()
		h := GetContentHeight()
		if w < 1 || h < 1 {
			RequestNextFrame()
			return
		}

		// Full-bleed background.
		Container(Attrs(FixSize(w, h), Float(0, 0), Clip, NoAnimate), func() {
			if game != nil {
				art := appState.gameArt(game.ID)
				if art.Banner != "" {
					coverImage("hero:"+game.ID, art.Banner, w, h)
				} else {
					fillGradient()
				}
			} else {
				fillGradient()
			}
		})

		// Overlay content.
		Container(Attrs(FixSize(w, h), Float(0, 0), Pad(32)), func() {
			Container(Attrs(Row, Expand), func() {
				Filler(1)
				if game != nil {
					statsPill(game)
				}
			})
			Filler(1)
			if game == nil {
				zeroState()
				return
			}
			Container(Attrs(Row, CrossAlign(AlignEnd), Expand, Gap(16)), func() {
				titleCard(game)
				Filler(1)
				Container(Attrs(Row, Gap(12), CrossAlign(AlignEnd)), func() {
					settingsTile("launcher.settings", func() { openSettings(game.ID) })
					playButton("launcher.open", func() { openCampaign(game.ID) })
				})
			})
		})
	})
}

func fillGradient() {
	Container(Attrs(Expand, Expand, BackgroundVec(ui.RaisedBG)), func() {})
}

func statsPill(game *gui.GameSummaryDTO) {
	Container(Attrs(Row, CrossMid, Gap(24), Corners(16), BackgroundVec(ui.PillBG),
		BorderWidth(1), BorderColorVec(ui.Hairline), Pad2(10, 20), BoxShadow(20)), func() {
		stat(TypTime, "Play Time", formatPlayTime(game.PlayTimeSeconds))
		statDivider()
		stat(TypCompass, "Turns", strconv.Itoa(game.TurnCount)+" turns")
		statDivider()
		stat(SymRefresh, "Last Played", formatRelativeTime(game.LastPlayed))
	})
}

func stat(icon IconGlyph, label, value string) {
	Container(Attrs(Row, CrossMid, Gap(10)), func() {
		Icon(icon, FontSize(16), TextColorVec(ui.Accent))
		Container(Attrs(Gap(2)), func() {
			Label(label, Fonts(ui.SansStack...), FontSize(10), FontWeight(WeightBold), TextColorVec(ui.TextMuted))
			Label(value, Fonts(ui.SansStack...), FontSize(13), FontWeight(WeightBold), TextColorVec(ui.TextMain))
		})
	})
}

func statDivider() {
	Element(Attrs(FixWidth(1), FixHeight(28), BackgroundVec(ui.Hairline)))
}

func titleCard(game *gui.GameSummaryDTO) {
	art := appState.gameArt(game.ID)
	Container(Attrs(Row, CrossMid, Gap(16), Corners(16), BackgroundVec(ui.PillBG),
		BorderWidth(1), BorderColorVec(ui.Hairline), Pad(16), BoxShadow(22)), func() {
		Container(Attrs(FixWidth(52), FixHeight(52), Corners(12), Clip), func() {
			if art.Icon != "" {
				coverImage("card:"+game.ID, art.Icon, 52, 52)
			} else {
				Container(Attrs(Expand, Expand, Center, BackgroundVec(ui.RaisedBG)), func() {
					Label(initial(game.Name), Fonts(ui.SansStack...), FontSize(22), FontWeight(WeightBold), TextColorVec(ui.TextMain))
				})
			}
		})
		Container(Attrs(Gap(4)), func() {
			Label(game.Name, Fonts(ui.SansStack...), FontSize(20), FontWeight(WeightBold), TextColorVec(ui.TextMain))
			line := "World: " + appState.WorldName(game.WorldID) + "   •   System: " + appState.SystemName(game.SystemID)
			if game.PlayerName != "" {
				line += "   •   Hero: " + game.PlayerName
			}
			Label(line, FontSize(12), TextColorVec(ui.TextMuted))
		})
	})
}

func settingsTile(name string, action func()) {
	Container(Attrs(FixWidth(52), FixHeight(52), Corners(16), Center, BackgroundVec(ui.PillBG),
		BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		if IsHovered() {
			ModAttrs(BorderColorVec(ui.Accent))
		}
		NextAccessName(name)
		if PressAction() {
			action()
		}
		AssignAccess()
		Icon(TypCog, FontSize(20), TextColorVec(ui.TextMain))
	})
}

func playButton(name string, action func()) {
	Container(Attrs(FixHeight(52), Corners(16), Pad2(0, 36), Center, BackgroundVec(ui.AccentBtn),
		BorderWidth(1), BorderColorVec(ui.AccentBorder), BoxShadow(24)), func() {
		if IsHovered() {
			ModAttrs(BackgroundVec(ui.AccentBtnHover))
		}
		NextAccessName(name)
		if PressAction() {
			action()
		}
		AssignAccess()
		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			Icon(SymPlay, FontSize(16), TextColorVec(ui.TextMain))
			Label("PLAY", Fonts(ui.SansStack...), FontSize(16), FontWeight(WeightBold), TextColorVec(ui.TextMain))
		})
	})
}

func zeroState() {
	Container(Attrs(Expand, Grow(1), Center), func() {
		Container(Attrs(ui.Card(FixWidth(460), Gap(10), Pad(28))), func() {
			Label("Welcome to LocalRPG", Fonts(ui.SansStack...), FontSize(20), FontWeight(WeightBold), TextColorVec(ui.TextMain))
			if len(appState.Worlds) > 0 {
				Label("Click the + button in the left dock to choose a world and start your first adventure.",
					FontSize(13), TextColorVec(ui.TextMuted))
			} else {
				Label("Create your first world or explore available game systems to begin crafting your tabletop campaign.",
					FontSize(13), TextColorVec(ui.TextMuted))
				Container(Attrs(Row, Gap(10)), func() {
					NextAccessName("zero.create-world")
					if Button(NoIcon, "Create First World") {
						openWorldsStudio()
					}
					AssignAccess()
					NextAccessName("zero.browse-systems")
					if Button(NoIcon, "Browse Systems") {
						openSystemsStudio()
					}
					AssignAccess()
				})
			}
		})
	})
}

func campaignGallery() {
	w := GetContentWidth()
	h := GetContentHeight()
	Container(Attrs(FixSize(w, h), Float(0, 0), BackgroundVec(ui.CanvasBG), Pad(28), Gap(16)), func() {
		Container(Attrs(Row, CrossMid, Gap(12)), func() {
			Label("Campaigns", Fonts(ui.SansStack...), FontSize(26), FontWeight(WeightBold), TextColorVec(ui.TextMain))
			Label(strconv.Itoa(len(appState.Games)), FontSize(14), TextColorVec(ui.TextMuted))
			Filler(1)
			NextAccessName("gallery.new")
			if Button(NoIcon, "New Campaign") {
				appState.CampaignGalleryOpen = false
				appState.Screen = ScreenNewCampaign
			}
			AssignAccess()
			NextAccessName("gallery.close")
			if Button(NoIcon, "Close") {
				appState.CampaignGalleryOpen = false
			}
			AssignAccess()
		})
		if len(appState.Games) == 0 {
			Label("No campaigns yet.", FontSize(14), TextColorVec(ui.TextMuted))
			return
		}
		Container(Attrs(Wrap, Gap(18)), func() {
			for i := range appState.Games {
				game := &appState.Games[i]
				galleryCard(game)
			}
		})
	})
}

func galleryCard(game *gui.GameSummaryDTO) {
	art := appState.gameArt(game.ID)
	Container(Attrs(FixWidth(300), Corners(16), Clip, BackgroundVec(ui.CardBG),
		BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		if IsHovered() {
			ModAttrs(BorderColorVec(ui.Accent))
		}
		NextAccessName("gallery.card." + game.ID)
		if PressAction() {
			appState.Selected = game.ID
			appState.CampaignGalleryOpen = false
		}
		AssignAccess()
		Container(Attrs(Expand, FixHeight(150), Clip), func() {
			if art.Banner != "" {
				coverImage("gallery:"+game.ID, art.Banner, 300, 150)
			} else {
				Container(Attrs(Expand, Expand, Center, BackgroundVec(ui.RaisedBG)), func() {
					Label(initial(game.Name), Fonts(ui.SansStack...), FontSize(40), FontWeight(WeightBold), TextColorVec(ui.TextFaint))
				})
			}
		})
		Container(Attrs(Pad(14), Gap(6)), func() {
			Label(game.Name, Fonts(ui.SansStack...), FontSize(16), FontWeight(WeightBold), TextColorVec(ui.TextMain))
			Label(appState.WorldName(game.WorldID)+"  ·  "+appState.SystemName(game.SystemID), FontSize(12), TextColorVec(ui.TextMuted))
			Label(game.PlayerName+"  ·  "+turnLabel(game.TurnCount), FontSize(12), TextColorVec(ui.TextFaint))
		})
	})
}

func turnLabel(n int) string {
	if n == 1 {
		return "1 turn"
	}
	return strconv.Itoa(n) + " turns"
}

func initial(name string) string {
	for _, r := range name {
		return string(r)
	}
	return "?"
}

func formatPlayTime(seconds int64) string {
	if seconds <= 0 {
		return "0 mins"
	}
	mins := seconds / 60
	if mins < 60 {
		return strconv.FormatInt(mins, 10) + " mins"
	}
	hours := mins / 60
	rem := mins % 60
	if rem > 0 {
		return fmt.Sprintf("%dh %dm", hours, rem)
	}
	return fmt.Sprintf("%dh", hours)
}

func formatRelativeTime(value string) string {
	if value == "" {
		return "Never"
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	diff := time.Since(t)
	switch {
	case diff < time.Minute:
		return "Just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

// artTile draws an image at a fixed height using a cover crop, or falls back to
// a labelled tile when no file exists.
func artTile(p ui.Palette, path, fallback string, height float32) {
	Container(Attrs(Expand, FixHeight(height), Corners(12), Clip, ui.HairlineBorder(), BackgroundVec(ui.RaisedBG)), func() {
		if path != "" {
			if IsClicked() {
				appState.LightboxPath = path
			}
			coverImage("art:"+path, path, GetContentWidth(), height)
			return
		}
		Container(Attrs(Expand, FixHeight(height), Center), func() {
			Label(fallback, Fonts(ui.SansStack...), FontSize(height/3), FontWeight(WeightBold), TextColorVec(ui.TextFaint))
		})
	})
}
