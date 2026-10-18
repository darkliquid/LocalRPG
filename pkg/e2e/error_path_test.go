//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
)

// failingRoleConfig points the gm role at a command that refuses for one reason and
// exits non-zero. A provider's refusal is classified by the message it carries, so
// the command is what produces the funds and rate-limit cases without a server.
func failingRoleConfig(message string) string {
	return "agents:\n  roles:\n    gm:\n      type: cli\n      command: /bin/sh\n" +
		"      args: [\"-c\", \"echo '" + message + "' >&2; exit 1\"]\n"
}

// TestAFailedTurnIsVisibleToThePlayer is TC-10: a turn that fails must be visible
// in a sentence the player can act on, not only in the console. The gm role is
// pointed at a command that refuses, so the turn fails the way a provider with no
// credit fails, and the two places the app reports it are asserted: the failure
// region above the chrome, and the alert banner the player can dismiss.
func TestAFailedTurnIsVisibleToThePlayer(t *testing.T) {
	f := NewFixture(t, failingRoleConfig("insufficient_quota"))
	f.WriteSystem(t, "freeform", "Freeform")
	f.WriteWorld(t, "harbour-realm", "Harbour Realm", nil)
	f.InitGame(t, engine.InitOptions{
		GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Sean",
	})
	f.WriteHistory(t, "campaign-01", threeTurns)

	b := f.Launch(t)
	b.Navigate("/")
	b.Click(`//button[@aria-label="Play Campaign"]`)

	b.SetInputValue(`//input[@id="action-console-input"]`, "I look around.")
	b.Activate(`//form//button[@type='submit']`)

	// The failure region carries the sentence, and the banner carries the reason
	// with a way to dismiss it.
	b.WaitFor("The turn failed.")
	b.Poll(`document.querySelector("[role='alert']") !== null`, "the turn failure banner appears")

	bannerText := b.Eval(`(() => {
	  const banner = document.querySelector("[role='alert']");
	  return banner ? String(banner.innerText).slice(0, 200) : 'none';
	})()`)
	if !strings.Contains(bannerText, "insufficient funds/credits") {
		t.Fatalf("the failure banner does not report insufficient funds: %s", bannerText)
	}

	dismiss := b.Eval(`(() => {
	  const banner = document.querySelector("[role='alert']");
	  const btn = banner ? banner.querySelector("button") : null;
	  return btn ? (btn.getAttribute("aria-label") || btn.innerText) : 'none';
	})()`)
	if !strings.Contains(dismiss, "Dismiss") {
		t.Fatalf("the failure banner does not offer a way to dismiss it: %s", dismiss)
	}
}
