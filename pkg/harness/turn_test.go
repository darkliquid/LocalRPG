package harness

import "testing"

func TestProposedCheckCarriesItsRef(t *testing.T) {
	check := ProposedCheck{Ref: "player-roll", Actor: "sean", Description: "pick the lock"}
	if check.Ref != "player-roll" || check.Description != "pick the lock" {
		t.Fatalf("unexpected proposed check: %+v", check)
	}
}
