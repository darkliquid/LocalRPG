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

func TestApplyUnlockDeductsAndApplies(t *testing.T) {
	bridge := NewHostBridge(newRulesTestStore(t), nil, "player")
	if err := bridge.SetStat("player", "xp", 5); err != nil {
		t.Fatal(err)
	}
	if err := bridge.SetStat("player", "might", 1); err != nil {
		t.Fatal(err)
	}
	spec := &core.AdvancementSpec{Currency: core.CurrencySpec{Stat: "xp"}}
	unlock := core.UnlockSpec{ID: "stat-increase", Cost: 5, Effects: []core.EffectSpec{
		{Type: "stat_increase", Stat: "might", Amount: 1, Max: 3},
	}}
	if err := ApplyUnlock(bridge, spec, unlock, "player", nil, true); err != nil {
		t.Fatalf("ApplyUnlock: %v", err)
	}
	if xp, _ := bridge.GetStat("player", "xp"); mustInt(t, xp) != 0 {
		t.Fatalf("xp = %v, want 0", xp)
	}
	if might, _ := bridge.GetStat("player", "might"); mustInt(t, might) != 2 {
		t.Fatalf("might = %v, want 2", might)
	}
}

func TestApplyUnlockHonoursCapAndGate(t *testing.T) {
	bridge := NewHostBridge(newRulesTestStore(t), nil, "player")
	if err := bridge.SetStat("player", "xp", 5); err != nil {
		t.Fatal(err)
	}
	if err := bridge.SetStat("player", "might", 3); err != nil {
		t.Fatal(err)
	}
	spec := &core.AdvancementSpec{Currency: core.CurrencySpec{Stat: "xp"}}
	unlock := core.UnlockSpec{ID: "cap", Cost: 5, Effects: []core.EffectSpec{
		{Type: "stat_increase", Stat: "might", Amount: 1, Max: 3},
	}}
	if err := ApplyUnlock(bridge, spec, unlock, "player", nil, true); err != nil {
		t.Fatalf("ApplyUnlock: %v", err)
	}
	if might, _ := bridge.GetStat("player", "might"); mustInt(t, might) != 3 {
		t.Fatalf("might = %v, want the cap 3", might)
	}

	gated := &core.AdvancementSpec{Currency: core.CurrencySpec{Stat: "xp"}, Gate: "downtime"}
	if err := ApplyUnlock(bridge, gated, core.UnlockSpec{ID: "gated", Cost: 1}, "player", nil, false); err == nil {
		t.Fatal("a closed gate should refuse the spend")
	}
}

func TestApplyUnlockGrantsTagAndSetsStat(t *testing.T) {
	bridge := NewHostBridge(newRulesTestStore(t), nil, "player")
	if err := bridge.SetStat("player", "xp", 4); err != nil {
		t.Fatal(err)
	}
	spec := &core.AdvancementSpec{Currency: core.CurrencySpec{Stat: "xp"}}
	unlock := core.UnlockSpec{ID: "veteran", Cost: 4, Effects: []core.EffectSpec{
		{Type: "grant_tag", Tag: "veteran"},
		{Type: "set_stat", Stat: "rank", Amount: 2},
	}}
	if err := ApplyUnlock(bridge, spec, unlock, "player", nil, true); err != nil {
		t.Fatalf("ApplyUnlock: %v", err)
	}
	ent, err := bridge.GetEntity("player")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tag := range ent.Tags {
		if tag == "veteran" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tags = %v, want veteran", ent.Tags)
	}
	if rank, _ := bridge.GetStat("player", "rank"); mustInt(t, rank) != 2 {
		t.Fatalf("rank = %v, want 2", rank)
	}
}

func TestAffordableRequirementsMetAndThreshold(t *testing.T) {
	unlock := core.UnlockSpec{ID: "u", Cost: 5, Requires: []string{"basic"}}
	if !Affordable(5, unlock) {
		t.Error("exact cost should be affordable")
	}
	if Affordable(4, unlock) {
		t.Error("one short should not be affordable")
	}
	if !RequirementsMet(unlock, []string{"basic"}, nil) {
		t.Error("owned requirement should be met")
	}
	if !RequirementsMet(unlock, nil, []string{"basic"}) {
		t.Error("tag requirement should be met")
	}
	if RequirementsMet(unlock, nil, nil) {
		t.Error("missing requirement should not be met")
	}

	spec := &core.AdvancementSpec{Levels: []core.LevelSpec{{At: 10, Label: "veteran"}, {At: 30}}}
	if level, ok := NextThreshold(spec, 12); !ok || level.At != 10 || level.Label != "veteran" {
		t.Fatalf("NextThreshold(12) = %+v, %v", level, ok)
	}
	if _, ok := NextThreshold(&core.AdvancementSpec{}, 5); ok {
		t.Error("a system with no levels has no threshold")
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
