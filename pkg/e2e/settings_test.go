//go:build e2e

package e2e

import (
	"fmt"
	"testing"
)

// TestSettingsStudioRendersEveryTab opens the studio from the dock and asserts
// each tab renders its signature content, which catches a lazy-chunk or
// tab-wiring failure that a type check cannot.
func TestSettingsStudioRendersEveryTab(t *testing.T) {
	f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")

	b := f.Launch(t)
	b.Navigate("/")
	b.Click(`//button[@aria-label="Settings"]`)
	b.WaitFor("Global Settings")

	tabs := []struct{ label, needle string }{
		{"Paths", "Storage & Discovery Paths"},
		{"Providers", "Cloud & Ecosystem Providers"},
		{"AI Agents", "AI Agents & Role Routing"},
		{"Media Engines", "Text-to-Speech (TTS) Engine"},
		{"Batch Jobs", "Batch Speech Backfill"},
		{"Preferences", "App Preferences & Appearance"},
		{"Usage", "Total Spend"},
		{"Debug", "Trace & Debug"},
	}
	for _, tab := range tabs {
		b.Click(fmt.Sprintf(`//button[.//span[normalize-space()='%s']]`, tab.label))
		b.WaitFor(tab.needle)
	}
}

// TestSettingsRoundTripAPath edits a storage path, saves, reloads the page, and
// asserts the value survives, which is what "settings persist" means to a user.
func TestSettingsRoundTripAPath(t *testing.T) {
	f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")

	b := f.Launch(t)
	b.Navigate("/")
	b.Click(`//button[@aria-label="Settings"]`)
	b.WaitFor("Global Settings")

	const input = `//label[normalize-space()='Rule Systems Directory']/following-sibling::input`
	b.WaitVisible(input)
	b.SetInputValue(input, "/tmp/e2e-systems")
	b.Click(`//button[.//span[normalize-space()='Save Settings']]`)

	b.Navigate("/")
	b.Click(`//button[@aria-label="Settings"]`)
	b.WaitFor("Global Settings")
	if got := b.InputValue(input); got != "/tmp/e2e-systems" {
		t.Fatalf("path did not round-trip: got %q", got)
	}
}
