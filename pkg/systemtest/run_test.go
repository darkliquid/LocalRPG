package systemtest

import (
	"reflect"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestRunPassesAndFails(t *testing.T) {
	sys := System{ID: "t", Script: `onAction("do", () => ({ outcome: "weak", message: "past" }))`,
		Mechanics: &core.MechanicsSpec{}}
	ok := Scenario{Name: "ok", Steps: []Step{{Action: "do", Expect: Expectations{Outcome: "weak", MessageContains: "past"}}}}
	if f := Run(sys, ok); len(f) != 0 {
		t.Fatalf("expected pass, got %+v", f)
	}
	bad := Scenario{Name: "bad", Steps: []Step{{Action: "do", Expect: Expectations{Outcome: "strong"}}}}
	if f := Run(sys, bad); len(f) == 0 {
		t.Fatal("expected a failure")
	}
}

func TestRunAssertsState(t *testing.T) {
	sys := System{ID: "t", Script: `onAction("do", () => { setStat("player", "grit", 1); return { outcome: "weak" }; })`,
		Mechanics: &core.MechanicsSpec{}}
	sc := Scenario{
		Name:  "state",
		Setup: SetupSpec{Player: SetupEntity{Stats: map[string]interface{}{"grit": 2}}},
		Steps: []Step{{Action: "do", Expect: Expectations{State: map[string]interface{}{"grit": 1}}}},
	}
	if f := Run(sys, sc); len(f) != 0 {
		t.Fatalf("expected pass, got %+v", f)
	}
}

func TestRunReportsLoadError(t *testing.T) {
	sys := System{ID: "t", Script: `this is not javascript ((`, Mechanics: &core.MechanicsSpec{}}
	sc := Scenario{Name: "broken", Steps: []Step{{Action: "do"}}}
	f := Run(sys, sc)
	if len(f) == 0 || f[0].Detail == "" {
		t.Fatalf("expected a load failure, got %+v", f)
	}
}

func TestRunIsDeterministic(t *testing.T) {
	sys := System{ID: "t", Script: `onAction("do", () => { var r = roll("2d6"); return { outcome: "x", roll: r }; })`,
		Mechanics: &core.MechanicsSpec{}}
	sc := Scenario{Name: "det", Seed: 99, Steps: []Step{{Action: "do", Expect: Expectations{Total: &Range{Min: 0, Max: 0}}}}}
	first := Run(sys, sc)
	second := Run(sys, sc)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("the same scenario produced %+v and %+v", first, second)
	}
}
