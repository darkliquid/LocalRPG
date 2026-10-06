//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestLauncherHubRenders(t *testing.T) {
	f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")

	b := f.Launch(t)
	b.Navigate("/")
	b.WaitVisible(`//button[@aria-label="New Campaign"]`)
	if text := b.BodyText(); !strings.Contains(text, "Welcome to LocalRPG") {
		t.Fatalf("expected the empty-state launcher, got:\n%s", text)
	}
}
