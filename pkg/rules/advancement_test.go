package rules

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestEarnFromTurnRecognisesEvents(t *testing.T) {
	spec := &core.AdvancementSpec{Earn: []core.EarnRule{
		{On: "miss", Amount: 1},
		{On: "check_outcome", Outcome: "strong", Amount: 2},
	}}
	outcomes := []string{"strong", "weak", "miss"}

	checks := []harness.CheckResult{{Outcome: "miss"}}
	if got := EarnFromTurn(spec, outcomes, checks); got != 1 {
		t.Fatalf("miss award = %d, want 1", got)
	}
	checks = []harness.CheckResult{{Outcome: "strong"}}
	if got := EarnFromTurn(spec, outcomes, checks); got != 2 {
		t.Fatalf("strong award = %d, want 2", got)
	}
	if got := EarnFromTurn(spec, outcomes, nil); got != 0 {
		t.Fatalf("quiet turn award = %d, want 0", got)
	}
}

func TestApplyEarnIncrementsAndDeducts(t *testing.T) {
	bridge := NewHostBridge(newRulesTestStore(t), nil, "player")
	spec := &core.AdvancementSpec{Currency: core.CurrencySpec{Stat: "xp"}}
	if err := ApplyEarn(bridge, spec, "player", 2); err != nil {
		t.Fatalf("ApplyEarn: %v", err)
	}
	if got, _ := bridge.GetStat("player", "xp"); mustInt(t, got) != 2 {
		t.Fatalf("xp = %v, want 2", got)
	}
	if err := ApplyEarn(bridge, spec, "player", -2); err != nil {
		t.Fatalf("ApplyEarn negative: %v", err)
	}
	if got, _ := bridge.GetStat("player", "xp"); mustInt(t, got) != 0 {
		t.Fatalf("xp = %v, want 0", got)
	}
}

func mustInt(t *testing.T, value interface{}) int {
	t.Helper()
	n, ok := toInt(value)
	if !ok {
		t.Fatalf("value %v is not a number", value)
	}
	return n
}
