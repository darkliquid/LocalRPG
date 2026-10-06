package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/state"
)

func TestDefaultCheckResolver(t *testing.T) {
	r := defaultCheckResolver{}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Actor: "player", CheckKind: "skill", Notation: "1d6+10",
		Outcomes: map[string]string{"pass": "ok", "fail": "no"},
	}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != "pass" {
		t.Fatalf("outcome = %q, want pass", res.Outcome)
	}
	if res.CheckID == "" {
		t.Fatal("CheckID is empty")
	}
	if res.Roll == nil || res.Roll.Total < 11 {
		t.Fatalf("roll = %+v, want a total of at least 11", res.Roll)
	}
}

func TestDefaultResolverAppliesModifiers(t *testing.T) {
	res, err := defaultCheckResolver{}.Resolve(context.Background(), harness.CheckRequest{
		Notation: "2d6", Modifiers: []harness.CheckModifier{{Source: "wounded", Value: -3}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Roll == nil || res.Roll.Total > 9 {
		t.Fatalf("total %+v did not include the -3 modifier", res.Roll)
	}
	if len(res.Applied) != 1 || res.Applied[0].Value != -3 {
		t.Fatalf("applied = %+v, want a single -3", res.Applied)
	}
}

func TestDefaultResolverAppliesSkillFromState(t *testing.T) {
	actor := &entity.Entity{ID: "hero", State: state.NewState(map[string]interface{}{"stealth": 3})}
	res, err := defaultCheckResolver{}.Resolve(context.Background(), harness.CheckRequest{
		Notation: "2d6", Skill: "stealth",
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if res.Roll == nil || res.Roll.Total < 5 {
		t.Fatalf("total %+v did not include the +3 skill", res.Roll)
	}
}
