//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
)

const threeTurns = `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"You look around."}` + "\n" +
	`{"number":2,"timestamp":"2026-09-21T10:05:00Z","mode":"Do","input":"wait","narration":"Time passes."}` + "\n" +
	`{"number":3,"timestamp":"2026-09-21T10:10:00Z","mode":"Do","input":"sleep","narration":"You rest."}` + "\n"

// TestDeletingTheLastCampaignResetsLauncherStats is the feedback loop for the
// report "when deleting a campaign, the stats in the top right of the launcher
// should reset": it drives the real SPA in a headless browser, deletes the only
// campaign, and asserts the stats badge is gone.
func TestDeletingTheLastCampaignResetsLauncherStats(t *testing.T) {
	f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")
	f.WriteSystem(t, "freeform", "Freeform")
	f.WriteWorld(t, "harbour-realm", "Harbour Realm", nil)
	f.InitGame(t, engine.InitOptions{GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Sean"})
	f.WriteHistory(t, "campaign-01", threeTurns)

	b := f.Launch(t)
	b.Navigate("/")
	b.WaitVisible(`//button[@aria-label="Campaign Settings"]`)

	if text := b.BodyText(); !strings.Contains(text, "3 turns") {
		t.Fatalf("stats badge does not show the campaign's turn count before deletion:\n%s", text)
	}

	b.Click(`//button[@aria-label="Campaign Settings"]`)
	b.Click(`//button[normalize-space()='Delete']`)
	b.Click(`//button[normalize-space()='Confirm Delete']`)
	b.WaitForGone("3 turns")
}
