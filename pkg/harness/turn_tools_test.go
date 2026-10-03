package harness

import "testing"

func TestTurnToolSpecs(t *testing.T) {
	names := map[string]bool{}
	for _, spec := range TurnToolSpecs() {
		names[spec.Name] = true
		if spec.Description == "" || spec.Parameters == nil {
			t.Fatalf("tool %q missing description or parameters", spec.Name)
		}
	}
	if !names["request_check"] {
		t.Fatal("the auto policy must offer request_check")
	}
	askNames := map[string]bool{}
	for _, spec := range TurnToolSpecsFor("ask") {
		askNames[spec.Name] = true
	}
	if !askNames["propose_check"] {
		t.Fatal("the ask policy must offer propose_check")
	}
	if names["submit_turn"] {
		t.Fatal("submit_turn is retired; the turn stream carries the prose")
	}
	if !IsTurnTool("request_check") || IsTurnTool("search_entities") {
		t.Fatal("IsTurnTool misclassified a tool")
	}
}

func TestParseCheckRequest(t *testing.T) {
	req, err := ParseCheckRequest(`{"actor":"player","check_kind":"skill","stat":"stealth","difficulty":"hard","stakes":"avoid the guard","outcomes":{"pass":"sneak past","fail":"spotted"}}`)
	if err != nil {
		t.Fatalf("ParseCheckRequest: %v", err)
	}
	if req.Stat != "stealth" || req.Outcomes["fail"] != "spotted" {
		t.Fatalf("req = %+v", req)
	}
}
