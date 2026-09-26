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
	NextAccessName("context.refresh")
	if Button(NoIcon, "Refresh") {
		refreshContext()
	}
	AssignAccess()

	tc := appState.TurnContext
	if tc == nil {
		Label("No turn context yet.", FontSize(12), TextColorVec(p.Muted))
		return
	}

	Label(fmt.Sprintf("Turn %d (%s)", tc.TurnNumber, tc.Mode), FontSize(13), FontWeight(WeightBold), TextColorVec(p.Text))
	Label("strategy: "+tc.Strategy+" · "+contextTokenSummary(), FontSize(11), TextColorVec(p.Muted))
	if tc.CachedTokens > 0 {
		Label(fmt.Sprintf("cached: %d", tc.CachedTokens), FontSize(11), TextColorVec(p.Muted))
	}
	Label("prompt "+shortHash(tc.PromptHash)+" · prefix "+shortHash(tc.PrefixHash), FontSize(11), TextColorVec(p.Muted))
	if tc.Session != nil {
		Label(fmt.Sprintf("session %s · %s · through turn %d", tc.Session.Provider, tc.Session.ID, tc.Session.ThroughTurn),
			FontSize(11), TextColorVec(p.Muted))
	}

	if len(appState.WorkingSet) > 0 {
		Spacer(6)
		Label("Working Set", FontSize(12), FontWeight(WeightBold), TextColorVec(p.Text))
		for _, entry := range appState.WorkingSet {
			label := entry.Name
			if label == "" {
				label = entry.ID
			}
			Label(fmt.Sprintf("%s · %s · t%d · %.1f", label, entry.Kind, entry.LastTurn, entry.Weight),
				FontSize(11), TextColorVec(p.Muted))
		}
	}

	if len(tc.Sections) > 0 {
		Spacer(6)
		Label("Prompt Sections", FontSize(12), FontWeight(WeightBold), TextColorVec(p.Text))
		for _, section := range tc.Sections {
			mark := "·"
			if section.Included {
				mark = "✓"
			}
			Label(fmt.Sprintf("%s %s (%d tokens) %s", mark, section.Name, section.Tokens, section.Source),
				FontSize(11), TextColorVec(p.Muted))
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
