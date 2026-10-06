package harness

import "testing"

func TestProposedCheckCarriesItsRef(t *testing.T) {
	check := ProposedCheck{Ref: "player-roll", Actor: "sean", Description: "pick the lock"}
	if check.Ref != "player-roll" || check.Description != "pick the lock" {
		t.Fatalf("unexpected proposed check: %+v", check)
	}
}

func TestCheckRequestCarriesProfile(t *testing.T) {
	req, err := ParseCheckRequest(`{"actor":"x","check_kind":"do","profile":"pbta","position":"risky"}`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Profile != "pbta" || req.Position != "risky" {
		t.Fatalf("decoded %+v", req)
	}
}
