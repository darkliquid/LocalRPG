package harness

import "testing"

func TestTurnSubmissionRoundTrip(t *testing.T) {
	raw := `{
		"action_verdict": {"feasibility": "uncertain", "reason": "a rope bridge over a chasm"},
		"segments": [
			{"kind": "narration", "text": "The bridge sways."},
			{"kind": "speech", "speaker": "Kae", "text": "Hold the rope!"}
		],
		"personae": [{"name": "Kae", "type": "character", "new": true, "gender": "woman", "role_tags": ["scout"]}],
		"memories": [{"kind": "event", "entity_refs": ["kae", "player"], "text": "Crossed the rope bridge.", "importance": 3}],
		"state_changes": [{"entity": "player", "path": "hp", "op": "sub", "value": 1, "reason": "strain"}]
	}`
	sub, err := ParseSubmission(raw)
	if err != nil {
		t.Fatalf("ParseSubmission: %v", err)
	}
	if sub.Verdict.Feasibility != FeasibilityUncertain {
		t.Fatalf("feasibility = %q", sub.Verdict.Feasibility)
	}
	if len(sub.Segments) != 2 || sub.Segments[1].Speaker != "Kae" {
		t.Fatalf("segments = %+v", sub.Segments)
	}
	if len(sub.Personae) != 1 || !sub.Personae[0].New {
		t.Fatalf("personae = %+v", sub.Personae)
	}
	if len(sub.StateChanges) != 1 || sub.StateChanges[0].Op != "sub" {
		t.Fatalf("state changes = %+v", sub.StateChanges)
	}
}

func TestProposedCheckCarriesItsRef(t *testing.T) {
	check := ProposedCheck{Ref: "player-roll", Actor: "sean", Description: "pick the lock"}
	if check.Ref != "player-roll" || check.Description != "pick the lock" {
		t.Fatalf("unexpected proposed check: %+v", check)
	}
}
