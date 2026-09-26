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

func actionConsole() {
	p := ui.DefaultPalette()
	mode := appState.ConsoleMode
	if mode == "" {
		mode = "do"
	}
	Container(Attrs(Expand, Pad(12), Gap(8), BackgroundVec(p.Panel)), func() {
		Container(Attrs(Row, Gap(6)), func() {
			for _, m := range []string{"do", "say", "story", "roll"} {
				modeKey := m
				selected := mode == modeKey
				Container(Attrs(Pad2(4, 10), Corners(6), BackgroundVec(p.Bg)), func() {
					if selected {
						ModAttrs(BackgroundVec(p.Accent))
					}
					NextAccessName("console.mode." + modeKey)
					if PressAction() {
						appState.ConsoleMode = modeKey
					}
					AssignAccess()
					Label(modeKey, FontSize(12), TextColorVec(p.Text))
				})
			}
		})

		TextInput(&appState.ConsoleText)

		Container(Attrs(Row, CrossMid, Gap(10)), func() {
			if appState.TurnError != "" {
				Label(appState.TurnError, FontSize(12), TextColorVec(p.Danger))
			}
			Filler(1)
			NextAccessName("console.submit")
			if !appState.TurnInFlight && appState.ConsoleText != "" && Button(NoIcon, "Act") {
				nextMode, text := parseConsole(appState.ConsoleText, mode)
				appState.ConsoleText = ""
				svc := liveService
				gameID := appState.OpenGame
				go submitTurn(context.Background(), svc, gameID, nextMode, text)
			}
			AssignAccess()
		})
	})
}

func prologuePanel() {
	p := ui.DefaultPalette()
	Container(Attrs(Expand, Grow(1), Center, Gap(10), Pad(24)), func() {
		Label(appState.GameName(), FontSize(22), FontWeight(WeightBold), TextColorVec(p.Text))
		Label("Begin the story", FontSize(14), TextColorVec(p.Muted))
		NextAccessName("prologue.begin")
		if !appState.TurnInFlight && Button(NoIcon, "Begin the story") {
			svc := liveService
			gameID := appState.OpenGame
			go submitTurn(context.Background(), svc, gameID, "story", "")
		}
		AssignAccess()
		if appState.TurnInFlight {
			Label("The narrator is drafting…", FontSize(12), TextColorVec(p.Muted))
		}
	})
}
