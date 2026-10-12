package rules

import "testing"

func TestTurnBeginAndEndHooks(t *testing.T) {
	bridge := NewHostBridge(newRulesTestStore(t), nil, "player")
	engine := NewJSEngine(bridge)
	script := `
	  onTurnBegin(function(ctx){ log("begin " + ctx.turn); });
	  onTurnEnd(function(ctx){ log("end " + ctx.turn + " " + ctx.verdict); });
	`
	if err := engine.LoadScript(script); err != nil {
		t.Fatal(err)
	}
	if err := engine.ExecuteTurnBegin(map[string]any{"turn": 4, "location": "hall"}); err != nil {
		t.Fatalf("ExecuteTurnBegin: %v", err)
	}
	if err := engine.ExecuteTurnEnd(map[string]any{"turn": 4, "verdict": "uncertain"}); err != nil {
		t.Fatalf("ExecuteTurnEnd: %v", err)
	}
	if len(bridge.GetLogs()) != 2 {
		t.Fatalf("logs = %v", bridge.GetLogs())
	}
}
