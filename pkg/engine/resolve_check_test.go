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
