//go:build e2e

package e2e

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

// TestPrimaryJourneyLaunchesCreatesCampaignAndPlaysTurn is TC-8: end-to-end test
// the primary user journey through a real browser: launch the app, create a
// campaign from the launcher, submit a turn, watch it stream, and assert the
// chronicle gains the turn and the turn serialization contract holds.
func TestPrimaryJourneyLaunchesCreatesCampaignAndPlaysTurn(t *testing.T) {
	f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")
	f.WriteSystem(t, "freeform", "Freeform")
	f.WriteWorld(t, "harbour-realm", "Harbour Realm", []string{"freeform"})

	b := f.Launch(t)
	b.Navigate("/")

	createCampaignFromLauncher(b, "Harbour Realm", "Sean")

	games, err := f.Service.ListGames(context.Background())
	if err != nil || len(games) == 0 {
		t.Fatalf("expected created campaign in ListGames: %v", err)
	}
	gameID := games[0].ID
	assertTurnSerialization(t, f.Service, gameID)

	// Enter the campaign screen from the launcher hero stage.
	b.Click(`//button[@aria-label="Play Campaign"]`)

	// Play a turn through the action console.
	b.WaitVisible(`//input[@id="action-console-input"]`)
	b.SetInputValue(`//input[@id="action-console-input"]`, "I examine the harbour waters.")
	b.Activate(`//form//button[@type='submit']`)

	// The turn streams and resolves with the provider's narration and ready state.
	b.WaitFor("Turn 1 ready")
	b.WaitFor("I examine the harbour waters.")

	body := b.BodyText()
	if !strings.Contains(body, "Echo") {
		t.Fatalf("chronicle does not render the narrator's prose:\n%s", body)
	}

	// Verify the chronicle on disk recorded the turn.
	chronicle, err := f.Service.GetChronicle(context.Background(), gameID)
	if err != nil {
		t.Fatalf("GetChronicle: %v", err)
	}
	if len(chronicle) != 1 || chronicle[0].TurnNumber != 1 {
		t.Fatalf("expected 1 recorded turn on disk, got %d (%+v)", len(chronicle), chronicle)
	}
}

func createCampaignFromLauncher(b *Browser, worldName, playerName string) {
	b.Click(`//button[@aria-label="New Campaign"]`)
	b.Click(`//button[@aria-label="` + worldName + `"]`)
	b.WaitVisible(`//button[normalize-space()='Create Campaign']`)
	b.Type(`//input[@placeholder='e.g. Valen Duskwarden']`, playerName)
	b.Click(`//button[normalize-space()='Create Campaign']`)
	b.WaitFor("Chronicles of " + worldName)
}

func assertTurnSerialization(t *testing.T, svc *gui.Service, gameID string) {
	t.Helper()
	session, err := svc.BeginTurn(gameID)
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	defer session.Close()

	if _, err := svc.BeginTurn(gameID); !errors.Is(err, gui.ErrTurnInFlight) {
		t.Fatalf("expected ErrTurnInFlight for concurrent BeginTurn, got %v", err)
	}
}
