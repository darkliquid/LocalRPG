package rules

import "testing"

func TestHealthZeroEffectRunsHook(t *testing.T) {
	engine := NewJSEngine(NewHostBridge(newRulesTestStore(t), nil, "player"))
	if err := engine.LoadScript(`onHealthZero(function(effect){ return "incapacitated"; });`); err != nil {
		t.Fatal(err)
	}
	effect, err := engine.EvaluateHealthZero("incapacitated")
	if err != nil {
		t.Fatalf("EvaluateHealthZero: %v", err)
	}
	if effect != "incapacitated" {
		t.Fatalf("effect = %q", effect)
	}
}

func TestHealthZeroFallsBackToDeclaredText(t *testing.T) {
	engine := NewJSEngine(NewHostBridge(newRulesTestStore(t), nil, "player"))
	effect, err := engine.EvaluateHealthZero("dead")
	if err != nil {
		t.Fatalf("EvaluateHealthZero: %v", err)
	}
	if effect != "dead" {
		t.Fatalf("effect = %q, want the declared text", effect)
	}
}
