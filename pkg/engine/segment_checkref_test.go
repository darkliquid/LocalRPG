package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

type stubCheckResolver struct{}

func (stubCheckResolver) Resolve(context.Context, harness.CheckRequest, *entity.Entity) (*harness.CheckResult, error) {
	return &harness.CheckResult{CheckID: "chk_x", Outcome: "pass"}, nil
}

func TestResolveCheckCopiesKindAndStakes(t *testing.T) {
	o := &TurnOrchestrator{checkResolver: stubCheckResolver{}}
	got, err := o.resolveCheck(context.Background(), harness.CheckRequest{
		CheckKind: "stealth", Stakes: "The guard wakes",
	}, nil)
	if err != nil {
		t.Fatalf("resolveCheck failed: %v", err)
	}
	if got.CheckKind != "stealth" || got.Stakes != "The guard wakes" {
		t.Fatalf("kind=%q stakes=%q", got.CheckKind, got.Stakes)
	}
}

// The chronicle draws each die from its face, so a resolved check has to carry
// the faces that landed: a total of 4 from 2d6 is not two sixes.
func TestDefaultCheckResolverReportsTheDiceThatLanded(t *testing.T) {
	o := &TurnOrchestrator{}

	got, err := o.resolveCheck(context.Background(), harness.CheckRequest{CheckKind: "skill", Notation: "2d6"}, nil)
	if err != nil {
		t.Fatalf("resolveCheck failed: %v", err)
	}
	if got.Roll == nil || len(got.Roll.Dice) != 2 {
		t.Fatalf("roll = %+v, want the two faces", got.Roll)
	}
	if got.Roll.RollCount != len(got.Roll.Dice) {
		t.Errorf("roll count = %d, faces = %d", got.Roll.RollCount, len(got.Roll.Dice))
	}

	sum := 0
	for _, die := range got.Roll.Dice {
		if die.Value < 1 || die.Value > 6 {
			t.Errorf("face %d is not a d6", die.Value)
		}
		if die.Symbol == "" {
			t.Errorf("face %d has nothing to show", die.Value)
		}
		sum += die.Value
	}
	if sum != got.Roll.Total {
		t.Errorf("faces total %d, want the roll's own total %d", sum, got.Roll.Total)
	}
}
