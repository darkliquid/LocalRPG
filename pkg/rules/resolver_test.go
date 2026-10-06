package rules

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/state"
)

func TestSchemaResolverUsesDifficulty(t *testing.T) {
	conventions := core.CheckConventions{
		Notation:   "1d6",
		Outcome:    []string{"pass", "fail"},
		Difficulty: []core.DifficultySpec{{ID: "trivial", Target: 1}, {ID: "hard", Target: 10}},
	}
	r := SchemaResolver{conventions: conventions}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		CheckKind: "skill", Difficulty: "trivial",
		Outcomes: map[string]string{"pass": "ok", "fail": "no"},
	}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != "pass" || res.Roll == nil {
		t.Fatalf("res = %+v", res)
	}
}

func TestJSCheckResolverOverridesSchema(t *testing.T) {
	engine := NewJSEngine(NewHostBridge(nil, nil, "player"))
	if err := engine.LoadScript(`onCheck("luck", function(req){ return {outcome:"pass"}; });`); err != nil {
		t.Fatalf("LoadScript: %v", err)
	}
	res, err := engine.Resolve(context.Background(), harness.CheckRequest{
		CheckKind: "luck",
		Outcomes:  map[string]string{"pass": "ok"},
	}, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Outcome != "pass" {
		t.Fatalf("outcome = %q", res.Outcome)
	}
}

func TestSchemaResolverAppliesSkillAndModifiers(t *testing.T) {
	actor := &entity.Entity{ID: "hero", State: state.NewState(map[string]interface{}{"might": 2, "stealth": 3})}
	r := SchemaResolver{conventions: core.CheckConventions{Notation: "2d6"}}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Stat:      "might",
		Skill:     "stealth",
		Modifiers: []harness.CheckModifier{{Source: "high ground", Value: 1}},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	// 2d6 is 2..12, so the total always includes the +6 bonus.
	if res.Roll == nil || res.Roll.Total < 8 {
		t.Fatalf("total %+v did not include the +6 bonus", res.Roll)
	}
	if len(res.Applied) != 3 {
		t.Fatalf("applied = %+v, want 3", res.Applied)
	}
}

func TestSchemaResolverUsesProfile(t *testing.T) {
	conventions := core.CheckConventions{Notation: "2d6", Profiles: map[string]core.ResolutionProfile{
		"pbta": {Ladder: []core.LadderStep{{Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}, {Min: 0, Outcome: "miss"}}},
	}}
	r := SchemaResolver{conventions: conventions}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{Profile: "pbta"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Profile != "pbta" || res.Outcome == "" {
		t.Fatalf("result = %+v", res)
	}
}

func TestSchemaResolverClampsStakes(t *testing.T) {
	conventions := core.CheckConventions{Notation: "2d6", Profiles: map[string]core.ResolutionProfile{
		"blades": {
			Ladder:   []core.LadderStep{{Min: 0, Outcome: "weak"}},
			Position: []string{"controlled", "risky", "desperate"},
			Effect:   []string{"limited", "standard", "great"},
		},
	}}
	r := SchemaResolver{conventions: conventions}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{Profile: "blades", Position: "risky", Effect: "bogus"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Position != "risky" || res.Effect != "limited" {
		t.Fatalf("position/effect = %q/%q, want risky/limited", res.Position, res.Effect)
	}
}

func TestSchemaResolverFallsBackWithoutProfile(t *testing.T) {
	// A request that names no profile resolves exactly as the conventions path did
	// before profiles existed.
	conventions := core.CheckConventions{Notation: "1d6", Outcome: []string{"pass", "fail"},
		Difficulty: []core.DifficultySpec{{ID: "trivial", Target: 1}}}
	r := SchemaResolver{conventions: conventions}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{Difficulty: "trivial"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != "pass" || res.Profile != "" {
		t.Fatalf("res = %+v, want a pass with no profile", res)
	}
}

func TestSchemaResolverStatOnlyIsUnchanged(t *testing.T) {
	actor := &entity.Entity{ID: "hero", State: state.NewState(map[string]interface{}{"might": 2})}
	r := SchemaResolver{conventions: core.CheckConventions{Notation: "2d6"}}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{Stat: "might"}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 1 || res.Applied[0].Source != "might" || res.Applied[0].Value != 2 {
		t.Fatalf("applied = %+v, want a single might +2", res.Applied)
	}
	// The total is exactly the dice plus the stat, so the change cannot alter a
	// stat-only check.
	dice := 0
	for _, die := range res.Roll.Dice {
		dice += die.Value
	}
	if res.Roll.Total != dice+2 {
		t.Fatalf("total %d != dice %d + 2", res.Roll.Total, dice)
	}
}
