package desktop

import (
	"context"
	"strings"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// parseConsole applies the /say, /do, /story, /roll prefixes, falling back to
// the supplied mode.
func parseConsole(input, fallback string) (string, string) {
	input = strings.TrimSpace(input)
	for _, p := range []struct{ prefix, mode string }{
		{"/say", "say"}, {"/do", "do"}, {"/story", "story"}, {"/roll", "roll"},
	} {
		if strings.HasPrefix(input, p.prefix) {
			return p.mode, strings.TrimSpace(strings.TrimPrefix(input, p.prefix))
		}
	}
	return fallback, input
}

// consoleMode returns the active console mode.
func consoleMode() string {
	if appState.ConsoleMode == "" {
		return "do"
	}
	return appState.ConsoleMode
}

func actionConsole() {
	Container(Attrs(Expand, Pad(16), Gap(12), BackgroundVec(ui.InputBG), BorderWidth(1), BorderColorVec(ui.Hairline)), func() {
		Element(Attrs(Expand, FixHeight(1), BackgroundVec(ui.Hairline)))

		Container(Attrs(Row, Wrap, Gap(8)), func() {
			for _, tab := range []struct{ mode, label string }{
				{"do", "DO"}, {"say", "SAY"}, {"story", "STORY"}, {"roll", "ROLL"},
			} {
				tab := tab
				selected := consoleMode() == tab.mode
				Container(Attrs(Pad2(5, 14), Corners(10)), func() {
					if selected {
						ModAttrs(BackgroundVec(ui.AccentBtn))
					} else if IsHovered() {
						ModAttrs(BackgroundVec(ui.HoverFill))
					}
					NextAccessName("console.mode." + tab.mode)
					if PressAction() {
						appState.ConsoleMode = tab.mode
					}
					AssignAccess()
					Label(tab.label, Fonts(ui.SansStack...), FontSize(11), FontWeight(WeightBold),
						TextColorVec(pick(selected, ui.TextMain, ui.TextMuted)))
				})
			}
		})

		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			Container(Attrs(Grow(1), Corners(12), BackgroundVec(ui.InputBG), BorderWidth(1), BorderColorVec(ui.Hairline), Clip), func() {
				TextInput(&appState.ConsoleText)
			})
			if appState.TurnError != "" {
				Label(appState.TurnError, FontSize(11), TextColorVec(ui.Danger))
			}
			if appState.TurnInFlight {
				NextAccessName("console.stop")
				actionTile("STOP", ui.RaisedBG, false, func() {})
				AssignAccess()
			} else {
				NextAccessName("console.submit")
				actionTile("Submit", ui.AccentBtn, appState.ConsoleText == "", func() {
					mode, text := parseConsole(appState.ConsoleText, consoleMode())
					if text == "" {
						return
					}
					appState.ConsoleText = ""
					svc := liveService
					gameID := appState.OpenGame
					go submitTurn(context.Background(), svc, gameID, mode, text)
				})
				AssignAccess()
			}
		})
	})
}

func pick(cond bool, a, b Vec4) Vec4 {
	if cond {
		return a
	}
	return b
}

// actionTile is a flat coloured action button.
func actionTile(label string, bg Vec4, disabled bool, action func()) {
	Container(Attrs(Pad2(11, 22), Corners(12), Center, BackgroundVec(bg)), func() {
		if disabled {
			ModAttrs(Trans(0.4))
			Label(label, Fonts(ui.SansStack...), FontSize(14), FontWeight(WeightBold), TextColorVec(ui.TextMain))
			return
		}
		if IsHovered() {
			ModAttrs(BorderWidth(1), BorderColorVec(ui.AccentBorder))
		}
		if PressAction() {
			action()
		}
		Label(label, Fonts(ui.SansStack...), FontSize(14), FontWeight(WeightBold), TextColorVec(ui.TextMain))
	})
}

// prologuePanel is what a campaign shows before it has any history.
func prologuePanel() {
	player := ""
	if game := appState.SelectedGame(); game != nil {
		player = game.PlayerName
	}
	Container(Attrs(Expand, Grow(1), Center, Pad(24)), func() {
		Container(Attrs(ui.Card(FixWidth(660), Gap(16), Pad(28))), func() {
			Container(Attrs(Expand, Center, Gap(6)), func() {
				Container(Attrs(Row, CrossMid, Gap(8)), func() {
					Icon(SymStar, FontSize(14), TextColorVec(ui.Accent))
					Label("PROLOGUE", Fonts(ui.SansStack...), FontSize(11), FontWeight(WeightBold), TextColorVec(ui.Accent))
				})
				Label(appState.GameName(), Fonts(ui.SansStack...), FontSize(26), FontWeight(WeightBold), TextColorVec(ui.TextMain))
				text := "Let the Game Master set the opening scene, then take it from there."
				if player != "" {
					text = player + " has not stepped into the story yet. " + text
				}
				Label(text, FontSize(14), TextColorVec(ui.TextMuted))
			})

			Label("Opening Prompt (optional)", Fonts(ui.SansStack...), FontSize(11), FontWeight(WeightBold), TextColorVec(ui.TextMuted))
			Container(Attrs(Expand, FixHeight(120), Corners(12), BackgroundVec(ui.InputBG), BorderWidth(1), BorderColorVec(ui.Hairline), Clip), func() {
				TextArea(&appState.ProloguePrompt)
			})
			Label("Leave it blank and the GM will invent the scene from your world and rules.",
				FontSize(11), TextColorVec(ui.TextFaint))

			Container(Attrs(Row, Center, Gap(12)), func() {
				NextAccessName("prologue.begin")
				actionTile("Begin the story", ui.AccentBtn, appState.TurnInFlight, func() {
					svc := liveService
					gameID := appState.OpenGame
					prompt := appState.ProloguePrompt
					go submitTurn(context.Background(), svc, gameID, "story", prompt)
				})
				AssignAccess()
				NextAccessName("prologue.first-step")
				actionTile("I'll take the first step", ui.RaisedBG, false, func() {
					appState.PrologueDismissed = true
				})
				AssignAccess()
			})
		})
	})
}
