package harness

import (
	"encoding/json"
	"testing"
)

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

func TestTurnSubmissionSchema(t *testing.T) {
	schema := TurnSubmissionSchema()
	if schema == nil {
		t.Fatal("TurnSubmissionSchema() returned nil")
	}

	if schema["type"] != "object" {
		t.Fatalf("schema type = %v, want 'object'", schema["type"])
	}

	reqs, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("schema required is not []string: %T", schema["required"])
	}
	reqMap := make(map[string]bool)
	for _, r := range reqs {
		reqMap[r] = true
	}
	if !reqMap["action_verdict"] || !reqMap["segments"] {
		t.Fatalf("required fields missing: %+v", reqs)
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema properties is not map[string]interface{}: %T", schema["properties"])
	}

	expectedProps := []string{
		"action_verdict",
		"segments",
		"personae",
		"memories",
		"state_changes",
		"player_location",
		"dismissed_checks",
	}
	for _, ep := range expectedProps {
		if _, exists := props[ep]; !exists {
			t.Errorf("schema missing property %q", ep)
		}
	}

	// Verify that a valid submission serialized matches properties defined in schema.
	sub := TurnSubmission{
		Verdict: ActionVerdict{
			Feasibility: FeasibilityAutomatic,
			Reason:      "open door",
		},
		Segments: []SegmentSpec{
			{Kind: "narration", Text: "You walk through."},
		},
		Personae: []PersonaDecl{
			{Name: "Guard", Type: "character"},
		},
		Memories: []MemoryDecl{
			{Kind: "event", EntityRefs: []string{"guard"}, Text: "met guard", Importance: 1},
		},
		StateChanges: []StateChangeDecl{
			{Entity: "player", Path: "hp", Op: "set", Value: 10},
		},
		PlayerLocation: "[[courtyard]]",
		DismissedChecks: []DismissedCheck{
			{CheckRef: "check-1", Reason: "trivial"},
		},
	}
	raw, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("marshal TurnSubmission: %v", err)
	}
	var rawMap map[string]interface{}
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		t.Fatalf("unmarshal TurnSubmission into map: %v", err)
	}
	for k := range rawMap {
		if _, exists := props[k]; !exists {
			t.Errorf("TurnSubmission field %q not defined in schema properties", k)
		}
	}
}
