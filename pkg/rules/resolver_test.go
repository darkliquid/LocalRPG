package rules

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
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
