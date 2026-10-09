package rules

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/state"
)

func manualResolver() SchemaResolver {
	return SchemaResolver{conventions: core.CheckConventions{
		Notation:   "2d6",
		Outcome:    []string{"pass", "fail"},
		Difficulty: []core.DifficultySpec{{ID: "easy", Target: 1}},
	}}
}

func TestManualDiceAreSummed(t *testing.T) {
	res, err := manualResolver().Resolve(context.Background(), harness.CheckRequest{
		ForcedDice: []int{4, 3}, Difficulty: "easy",
	}, &entity.Entity{ID: "hero"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Roll == nil || res.Roll.Total != 7 || res.Roll.RollCount != 2 {
		t.Fatalf("roll = %+v", res.Roll)
	}
	if len(res.Roll.Dice) != 2 || res.Roll.Dice[0].Value != 4 || res.Roll.Dice[1].Value != 3 {
		t.Fatalf("dice = %+v, want the faces the player entered", res.Roll.Dice)
	}
	if res.Source != "manual" {
		t.Fatalf("source = %q, want manual", res.Source)
	}
}

func TestManualTotalPlusBonuses(t *testing.T) {
	actor := &entity.Entity{ID: "hero", State: state.NewState(map[string]any{"stealth": 2})}
	total := 7
	res, err := manualResolver().Resolve(context.Background(), harness.CheckRequest{
		Skill: "stealth", ForcedTotal: &total, Difficulty: "easy",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Roll.Total != 9 {
		t.Fatalf("total = %d, want 7 entered plus 2 stealth", res.Roll.Total)
	}
	if res.Source != "manual" {
		t.Fatalf("source = %q, want manual", res.Source)
	}
}

func TestManualDiceKeepTheBonuses(t *testing.T) {
	actor := &entity.Entity{ID: "hero", State: state.NewState(map[string]any{"stealth": 2})}
	res, err := manualResolver().Resolve(context.Background(), harness.CheckRequest{
		Skill: "stealth", ForcedDice: []int{4, 3}, Difficulty: "easy",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Roll.Total != 9 {
		t.Fatalf("total = %d, want the dice sum plus the bonus", res.Roll.Total)
	}
}

// TestManualPoolCountsSuccesses guards that a manual entry into a pool notation
// resolves the way a rolled pool would, rather than reading a random roll's
// successes.
func TestManualPoolCountsSuccesses(t *testing.T) {
	r := SchemaResolver{conventions: core.CheckConventions{
		Notation: "5d10",
		Profiles: map[string]core.ResolutionProfile{
			"pool": {SuccessOn: ">=8", Outcomes: []core.SuccessOutcome{
				{Min: 2, Max: -1, Outcome: "strong"}, {Min: 0, Max: 1, Outcome: "weak"},
			}},
		},
	}}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Profile: "pool", ForcedDice: []int{9, 8, 2, 10, 1},
	}, &entity.Entity{ID: "hero"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Successes != 3 {
		t.Fatalf("successes = %d, want 3 from 9, 8, and 10", res.Successes)
	}
	if res.Outcome != "strong" {
		t.Fatalf("outcome = %q, want the pool's strong outcome", res.Outcome)
	}
}

// TestManualEntryNeverRejected guards the app's trust: an implausible entry is
// accepted, because a local-first table may have a house rule.
func TestManualEntryNeverRejected(t *testing.T) {
	res, err := manualResolver().Resolve(context.Background(), harness.CheckRequest{
		ForcedDice: []int{20}, Difficulty: "easy",
	}, &entity.Entity{ID: "hero"})
	if err != nil {
		t.Fatalf("an implausible entry must resolve, got %v", err)
	}
	if res.Roll.Total != 20 {
		t.Fatalf("total = %d, want the entered 20", res.Roll.Total)
	}
}

// TestManualTotalIsDicePlusBonuses is the property the honesty rests on: whatever
// dice a player enters, the recorded total is their sum plus the system's bonuses.
func TestManualTotalIsDicePlusBonuses(t *testing.T) {
	actor := &entity.Entity{ID: "hero", State: state.NewState(map[string]any{"stealth": 3, "might": 1})}
	req := harness.CheckRequest{
		Skill: "stealth", Stat: "might", Modifiers: []harness.CheckModifier{{Source: "high ground", Value: 2}},
		Difficulty: "easy",
	}
	for _, dice := range [][]int{{1}, {4, 3}, {6, 6}, {1, 1, 1, 1}} {
		sum := 0
		for _, face := range dice {
			sum += face
		}
		entry := append([]int(nil), dice...)
		req.ForcedDice = entry
		res, err := manualResolver().Resolve(context.Background(), req, actor)
		if err != nil {
			t.Fatal(err)
		}
		if want := sum + 3 + 1 + 2; res.Roll.Total != want {
			t.Fatalf("dice %v recorded %d, want %d", dice, res.Roll.Total, want)
		}
	}
}

func TestManualRollCountsNoSuccessesWithoutAThreshold(t *testing.T) {
	roll := ManualRoll("2d6", []int{6, 6})
	if roll.Successes != 0 || roll.Total != 12 {
		t.Fatalf("roll = %+v", roll)
	}
}
