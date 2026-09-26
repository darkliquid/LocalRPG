package desktop

import (
	"fmt"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/darkliquid/localrpg/pkg/ui"
)

// refreshContext loads the turn context report and working set.
func refreshContext() {
	svc := liveService
	if svc == nil {
		return
	}
	gameID := appState.OpenGame
	go func() {
		tc, working := loadContext(svc, gameID)
		WithFrameLock(func() {
			appState.TurnContext = tc
			appState.WorkingSet = working
		})
		RequestNextFrame()
	}()
}

// contextTokenSummary describes the budget usage, or empty when unknown.
func contextTokenSummary() string {
	tc := appState.TurnContext
	if tc == nil {
		return ""
	}
	return fmt.Sprintf("%d/%d tokens", tc.EstimatedTokens, tc.Budget)
}

func contextDrawer(p ui.Palette) {
	settingsSmallButton("context.refresh", "Refresh", refreshContext)

	tc := appState.TurnContext
	if tc == nil {
		settingsHint("No turn context yet.")
		return
	}

	settingsSubTitle(TypThList, fmt.Sprintf("Turn %d (%s)", tc.TurnNumber, tc.Mode))
	Label("strategy: "+tc.Strategy+" · "+contextTokenSummary(), Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextMuted))
	if tc.CachedTokens > 0 {
		Label(fmt.Sprintf("cached: %d", tc.CachedTokens), Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextMuted))
	}
	Label("prompt "+shortHash(tc.PromptHash)+" · prefix "+shortHash(tc.PrefixHash), Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextMuted))
	if tc.Session != nil {
		Label(fmt.Sprintf("session %s · %s · through turn %d", tc.Session.Provider, tc.Session.ID, tc.Session.ThroughTurn),
			Fonts(Monospace...), FontSize(11), TextColorVec(ui.TextMuted))
	}

	if len(appState.WorkingSet) > 0 {
		settingsSubTitle(TypGroup, "Working Set")
		for _, entry := range appState.WorkingSet {
			label := entry.Name
			if label == "" {
				label = entry.ID
			}
			Label(fmt.Sprintf("%s · %s · t%d · %.1f", label, entry.Kind, entry.LastTurn, entry.Weight),
				FontSize(11), TextColorVec(ui.TextMuted))
		}
	}

	if len(tc.Sections) > 0 {
		settingsSubTitle(TypThList, "Prompt Sections")
		for _, section := range tc.Sections {
			mark := "·"
			if section.Included {
				mark = "✓"
			}
			Label(fmt.Sprintf("%s %s (%d tokens) %s", mark, section.Name, section.Tokens, section.Source),
				FontSize(11), TextColorVec(ui.TextMuted))
		}
	}
}

// shortHash trims a hash for display.
func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	if h == "" {
		return "-"
	}
	return h
}
