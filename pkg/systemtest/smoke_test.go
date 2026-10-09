package systemtest

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestSmokeScenarioNamesTheProfile(t *testing.T) {
	spec := &core.MechanicsSpec{Checks: core.CheckConventions{
		Profiles: map[string]core.ResolutionProfile{"pbta": {Ladder: []core.LadderStep{{Min: 0, Outcome: "miss"}}}}}}
	s := SmokeScenario(spec)
	if len(s.Steps) == 0 || len(s.Steps[0].Expect.OutcomeOneOf) == 0 {
		t.Fatalf("scenario = %+v", s)
	}
	if s.Steps[0].Input != "pbta" {
		t.Fatalf("expected the scenario to name the profile, got %q", s.Steps[0].Input)
	}
}

func TestSmokeScenarioEmptyForSchemaAgnostic(t *testing.T) {
	if s := SmokeScenario(nil); len(s.Steps) != 0 {
		t.Fatalf("scenario = %+v", s)
	}
}

func TestSmokeScenarioResolvesADeclarativeProfile(t *testing.T) {
	spec := &core.MechanicsSpec{Checks: core.CheckConventions{
		Notation: "2d6",
		Profiles: map[string]core.ResolutionProfile{"pbta": {Ladder: []core.LadderStep{
			{Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}, {Min: 0, Outcome: "miss"},
		}}},
	}}
	sys := System{ID: "generated", Mechanics: spec}
	if failures := Run(sys, SmokeScenario(spec)); len(failures) > 0 {
		t.Fatalf("failures = %+v", failures)
	}
}

func TestSmokeScenarioCatchesAProfileThatNeverResolves(t *testing.T) {
	spec := &core.MechanicsSpec{Checks: core.CheckConventions{
		Notation: "2d6",
		Profiles: map[string]core.ResolutionProfile{"broken": {Notation: "not-dice"}},
	}}
	sys := System{ID: "broken", Mechanics: spec}
	if failures := Run(sys, SmokeScenario(spec)); len(failures) == 0 {
		t.Fatal("a profile that never resolves should fail the smoke scenario")
	}
}

func TestSmokeScenarioDeclaresADeclaredStat(t *testing.T) {
	spec := &core.MechanicsSpec{
		Stats:  []core.StatSpec{{ID: "might", Default: 3}},
		Checks: core.CheckConventions{Outcome: []string{"success", "fail"}},
	}
	s := SmokeScenario(spec)
	if got := s.Setup.Player.Stats["might"]; got != 3 {
		t.Fatalf("setup stat = %v, want 3", got)
	}
}
