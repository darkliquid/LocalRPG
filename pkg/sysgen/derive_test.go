package sysgen

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/refsystems"
	"github.com/darkliquid/localrpg/pkg/systemtest"
)

func (j *jsonGen) sawPromptContaining(substr string) bool {
	for _, prompt := range j.prompts {
		if strings.Contains(prompt, substr) {
			return true
		}
	}
	return false
}

func TestDeriveRequiresAnInstruction(t *testing.T) {
	if _, err := Derive(context.Background(), &jsonGen{}, refsystems.ReferenceSystem{ID: "b"}, ""); err == nil {
		t.Fatal("an empty instruction should error")
	}
}

func TestDeriveSeedsTheBase(t *testing.T) {
	g := &jsonGen{responses: []string{`{"mechanics":{"stats":[{"id":"sanity"}]},"rules":"x"}`}}
	base := refsystems.ReferenceSystem{ID: "b", Name: "Base", Script: "onAction(...)"}
	s, err := Derive(context.Background(), g, base, "add sanity")
	if err != nil {
		t.Fatal(err)
	}
	if s.Mechanics == nil {
		t.Fatalf("system = %+v", s)
	}
	if !g.sawPromptContaining("Base") {
		t.Fatal("the base should seed the prompt")
	}
}

func TestDeriveFallsBackToTheBase(t *testing.T) {
	g := &jsonGen{responses: []string{`{}`}}
	base := refsystems.ReferenceSystem{
		ID:        "b",
		Name:      "Base",
		Mechanics: &core.MechanicsSpec{Stats: []core.StatSpec{{ID: "might"}}},
		Script:    `onAction("do", () => ({ outcome: "miss" }))`,
	}
	s, err := Derive(context.Background(), g, base, "keep it")
	if err != nil {
		t.Fatal(err)
	}
	if s.Mechanics == nil || len(s.Mechanics.Stats) != 1 {
		t.Fatalf("system = %+v", s)
	}
	if s.Name != "Base" {
		t.Fatalf("name = %q", s.Name)
	}
	if !s.Verify.OK {
		t.Fatalf("the base should verify, got %v", s.Verify.Failures)
	}
}

func TestValidateDerivationRunsBaseScenarios(t *testing.T) {
	sys := System{Script: `onAction("do", () => ({ outcome: "strong" }))`, Mechanics: &core.MechanicsSpec{}}
	base := []systemtest.Scenario{{Name: "base", Steps: []systemtest.Step{
		{Action: "do", Expect: systemtest.Expectations{Outcome: "weak"}},
	}}}
	if got := ValidateDerivation(sys, base); got.OK {
		t.Fatal("a variant that breaks a base scenario should be flagged")
	}
}

func TestValidateDerivationPassesAGoodVariant(t *testing.T) {
	sys := System{Script: `onAction("do", () => ({ outcome: "weak" }))`, Mechanics: &core.MechanicsSpec{}}
	base := []systemtest.Scenario{{Name: "base", Steps: []systemtest.Step{
		{Action: "do", Expect: systemtest.Expectations{Outcome: "weak"}},
	}}}
	if got := ValidateDerivation(sys, base); !got.OK {
		t.Fatalf("gate = %+v", got)
	}
}
