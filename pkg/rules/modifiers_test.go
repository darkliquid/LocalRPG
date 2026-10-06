package rules

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestSumBonuses(t *testing.T) {
	values := map[string]int{"might": 2, "stealth": 3}
	lookup := func(n string) (int, bool) { v, ok := values[n]; return v, ok }
	req := harness.CheckRequest{
		Stat:  "might",
		Skill: "stealth",
		Modifiers: []harness.CheckModifier{
			{Source: "high ground", Value: 1},
			{Source: "wounded", Value: -2},
		},
	}
	total, applied := SumBonuses(lookup, req)
	if total != 4 {
		t.Fatalf("total = %d, want 4", total)
	}
	if len(applied) != 4 {
		t.Fatalf("applied = %+v, want 4 entries", applied)
	}
}

func TestSumBonusesMissingValuesAddZero(t *testing.T) {
	req := harness.CheckRequest{Stat: "luck", Skill: "nope"}
	total, applied := SumBonuses(func(string) (int, bool) { return 0, false }, req)
	if total != 0 || len(applied) != 0 {
		t.Fatalf("total = %d applied = %+v, want 0 and empty", total, applied)
	}
}

// TestSumBonusesTotalEqualsAppliedSum is the property the resolvers rely on: the
// total is exactly the sum of the named contributions, whatever the request.
func TestSumBonusesTotalEqualsAppliedSum(t *testing.T) {
	values := map[string]int{"might": 2, "stealth": 3, "luck": -1}
	lookup := func(n string) (int, bool) { v, ok := values[n]; return v, ok }
	cases := []harness.CheckRequest{
		{},
		{Stat: "might"},
		{Stat: "might", Skill: "stealth"},
		{Stat: "luck", Modifiers: []harness.CheckModifier{{Source: "wounded", Value: -2}, {Source: "high ground", Value: 1}}},
		{Skill: "ghost", Modifiers: []harness.CheckModifier{{Source: "zero", Value: 0}}},
	}
	for _, req := range cases {
		total, applied := SumBonuses(lookup, req)
		sum := 0
		for _, a := range applied {
			sum += a.Value
		}
		if total != sum {
			t.Errorf("SumBonuses(%+v) total %d != applied sum %d", req, total, sum)
		}
	}
}
