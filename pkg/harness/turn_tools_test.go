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
	for _, want := range []string{"submit_turn", "request_check"} {
		if !names[want] {
			t.Fatalf("missing turn tool %q", want)
		}
	}
	if !IsTurnTool("submit_turn") || IsTurnTool("search_entities") {
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
