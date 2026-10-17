//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
)

const undoTestHistory = `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"You look around the harbour."}` + "\n" +
	`{"number":2,"timestamp":"2026-09-21T10:05:00Z","mode":"Do","input":"wait","narration":"Time passes slowly."}` + "\n" +
	`{"number":3,"timestamp":"2026-09-21T10:10:00Z","mode":"Do","input":"fight","narration":"You rest after the fight.","current_appearances":[{"id":"sean","before":"","after":"Cut above one eye."}]}` + "\n"

// TestUndoRewindsChronicleAndEntityState is TC-9: submitting /undo rewinds the
// campaign by one turn, removing the discarded turn from the chronicle and
// reverting modified entity state back to its baseline in the browser.
func TestUndoRewindsChronicleAndEntityState(t *testing.T) {
	f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")
	f.WriteSystem(t, "freeform", "Freeform")
	f.WriteWorld(t, "harbour-realm", "Harbour Realm", nil)
	f.InitGame(t, engine.InitOptions{
		GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Sean",
	})
	f.WriteEntities(t, "campaign-01", map[string]string{
		"sean.md": "---\nid: sean\nname: Sean\ntype: character\nappearance: Silver hair and a grey cloak.\ncurrent_appearance: Cut above one eye.\n---\nA traveller.\n",
	})
	f.WriteHistory(t, "campaign-01", undoTestHistory)

	b := f.Launch(t)
	b.Navigate("/")
	b.Click(`//button[@aria-label="Play Campaign"]`)

	// Assert the initial chronicle contains all three turns.
	b.WaitFor("You look around the harbour.")
	b.WaitFor("Time passes slowly.")
	b.WaitFor("You rest after the fight.")

	// Open the Character Sheet drawer and assert the modified appearance is present.
	b.Activate(`//button[contains(normalize-space(.), 'Character')]`)
	b.Poll(`document.querySelector("[role='dialog'][aria-label='Character Sheet']") !== null`,
		"the Character Sheet drawer opens")
	b.WaitFor("Cut above one eye.")
	b.PressKey(`//div[@role='dialog'][@aria-label='Character Sheet']`, "Escape")
	b.Poll(`document.querySelector("[role='dialog'][aria-label='Character Sheet']") === null`,
		"the Character Sheet drawer closes")

	// Submit /undo in the action console.
	b.SetInputValue(`//input[@id="action-console-input"]`, "/undo")
	b.Activate(`//form//button[@type='submit']`)

	// The chronicle loses turn 3, while turns 1 and 2 remain.
	b.WaitForGone("You rest after the fight.")
	b.WaitFor("Time passes slowly.")
	b.WaitFor("You look around the harbour.")

	// Open the Character Sheet drawer again and assert current appearance reverted.
	b.Activate(`//button[contains(normalize-space(.), 'Character')]`)
	b.Poll(`document.querySelector("[role='dialog'][aria-label='Character Sheet']") !== null`,
		"the Character Sheet drawer opens again")
	b.WaitForGone("Cut above one eye.")
	b.WaitFor("Silver hair and a grey cloak.")
	b.PressKey(`//div[@role='dialog'][@aria-label='Character Sheet']`, "Escape")
	b.Poll(`document.querySelector("[role='dialog'][aria-label='Character Sheet']") === null`,
		"the Character Sheet drawer closes again")

	if body := b.BodyText(); strings.Contains(body, "You rest after the fight.") {
		t.Fatalf("discarded turn is still visible in the document: %s", body)
	}
}
