package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestForcedTotalResolvesToThatTotal(t *testing.T) {
	o := &TurnOrchestrator{}
	total := 11
	o.SetForcedTotal(&total)
	res, err := o.resolveCheck(context.Background(), harness.CheckRequest{Notation: "2d6"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Roll == nil || res.Roll.Total != 11 || res.Source != "manual" {
		t.Fatalf("result = %+v", res)
	}
}

func TestForcedTotalIsConsumedOnce(t *testing.T) {
	o := &TurnOrchestrator{}
	total := 11
	o.SetForcedTotal(&total)
	if _, err := o.resolveCheck(context.Background(), harness.CheckRequest{Notation: "2d6"}, nil); err != nil {
		t.Fatal(err)
	}
	res, err := o.resolveCheck(context.Background(), harness.CheckRequest{Notation: "2d6"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == "manual" {
		t.Fatal("the forced total should be consumed after the first check")
	}
}

func TestManualDiceResolveToTheirSumPlusBonuses(t *testing.T) {
	o := &TurnOrchestrator{}
	o.SetManualDice([]int{4, 3})
	res, err := o.resolveCheck(context.Background(), harness.CheckRequest{Notation: "2d6"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Roll == nil || res.Roll.Total != 7 || res.Source != "manual" {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Roll.Dice) != 2 || res.Roll.Dice[0].Value != 4 {
		t.Fatalf("dice = %+v, want the entered faces", res.Roll.Dice)
	}
}

func TestManualDiceAreConsumedOnce(t *testing.T) {
	o := &TurnOrchestrator{}
	o.SetManualDice([]int{4, 3})
	if _, err := o.resolveCheck(context.Background(), harness.CheckRequest{Notation: "2d6"}, nil); err != nil {
		t.Fatal(err)
	}
	res, err := o.resolveCheck(context.Background(), harness.CheckRequest{Notation: "2d6"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Source == "manual" {
		t.Fatal("the entered dice should be consumed after the first check")
	}
}
