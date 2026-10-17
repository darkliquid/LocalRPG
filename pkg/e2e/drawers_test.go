//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
)

// TestEveryDrawerOpensAndNamesItself is TC-11: the campaign's five drawers are
// opened one after another in a real browser, each is asserted to be a dialog named
// for what it holds, and the campaign is asserted to still be there afterwards. A
// React error in any of them unmounts the tree and leaves a blank page, which is
// the failure a browser test catches and a unit test without one does not.
func TestEveryDrawerOpensAndNamesItself(t *testing.T) {
	f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")
	f.WriteSystem(t, "freeform", "Freeform")
	f.WriteWorld(t, "harbour-realm", "Harbour Realm", nil)
	f.InitGame(t, engine.InitOptions{
		GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Sean",
	})
	f.WriteHistory(t, "campaign-01", threeTurns)

	b := f.Launch(t)
	b.Navigate("/")
	b.Click(`//button[@aria-label="Play Campaign"]`)

	// Each pill opens the drawer it names, and the drawer declares what it holds.
	for _, drawer := range []struct{ pill, name string }{
		{"Character", "Character Sheet"},
		{"Graph", "Lore Graph"},
		{"Codex", "Codex Markdown Editor"},
		{"World Arcs", "Living World Arcs & Clocks"},
		{"Context", "Turn Context & Continuity"},
	} {
		b.Activate(`//button[contains(normalize-space(.), '` + drawer.pill + `')]`)
		b.Poll(`document.querySelector("[role='dialog'][aria-label='`+drawer.name+`']") !== null`,
			"the "+drawer.name+" drawer opens")

		// Escape closes it, which is the drawer's own keyboard behaviour and needs no
		// coordinates: headless Chrome never advances the slide-in animation, so the
		// panel stays at its start position and its close button is off-screen.
		b.PressKey(`//div[@role='dialog'][@aria-label='`+drawer.name+`']`, "Escape")
		b.Poll(`document.querySelector("[role='dialog'][aria-label='`+drawer.name+`']") === null`,
			"the "+drawer.name+" drawer closes")
	}

	if body := b.BodyText(); !strings.Contains(body, "The quay") && !strings.Contains(body, "You look around") {
		t.Fatalf("the chronicle is not on the page after closing every drawer:\n%s", body)
	}
}
