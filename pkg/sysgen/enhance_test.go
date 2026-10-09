package sysgen

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestProposeReturnsAdditions(t *testing.T) {
	g := &jsonGen{responses: []string{`{"proposals":[
	  {"kind":"stat","title":"Sanity","stat":{"id":"sanity"}},
	  {"kind":"skill","title":"Occult","skill":{"id":"occult","stat":"sanity"}}]}`}}
	got, err := Propose(context.Background(), g, System{ID: "s", Mechanics: &core.MechanicsSpec{}}, "add sanity", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Kind != "stat" {
		t.Fatalf("proposals = %+v", got)
	}
}

func TestProposeDropsEmptyProposals(t *testing.T) {
	g := &jsonGen{responses: []string{`{"proposals":[{"kind":"stat"},{"kind":"bogus","title":"x"},{"kind":"skill","skill":{"id":"occult"}}]}`}}
	got, err := Propose(context.Background(), g, System{ID: "s"}, "add", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != "skill" {
		t.Fatalf("proposals = %+v", got)
	}
}

func TestProposeCapsTheList(t *testing.T) {
	var items []string
	for i := range MaxProposals + 5 {
		items = append(items, fmt.Sprintf(`{"kind":"stat","stat":{"id":"stat_%d"}}`, i))
	}
	g := &jsonGen{responses: []string{`{"proposals":[` + strings.Join(items, ",") + `]}`}}
	got, err := Propose(context.Background(), g, System{ID: "s"}, "add stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxProposals {
		t.Fatalf("proposals = %d, want %d", len(got), MaxProposals)
	}
}

func TestApplyAdditionsAppends(t *testing.T) {
	spec := &core.MechanicsSpec{Stats: []core.StatSpec{{ID: "might"}}}
	got, err := ApplyAdditions(spec, []Proposal{{Kind: "stat", Stat: &core.StatSpec{ID: "sanity"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Stats) != 2 || got.Stats[0].ID != "might" || got.Stats[1].ID != "sanity" {
		t.Fatalf("stats = %+v", got.Stats)
	}
	if len(spec.Stats) != 1 {
		t.Fatalf("the original spec should be untouched, got %+v", spec.Stats)
	}
}

func TestApplyAdditionsRefusesDuplicates(t *testing.T) {
	spec := &core.MechanicsSpec{Stats: []core.StatSpec{{ID: "might"}}}
	if _, err := ApplyAdditions(spec, []Proposal{{Kind: "stat", Stat: &core.StatSpec{ID: "might"}}}); err == nil {
		t.Fatal("a duplicate id should be refused")
	}
}

func TestApplyAdditionsNoProposalsUnchanged(t *testing.T) {
	spec := &core.MechanicsSpec{
		Stats:  []core.StatSpec{{ID: "might"}},
		Skills: []core.SkillSpec{{ID: "brawl", Stat: "might"}},
		Checks: core.CheckConventions{
			Notation: "2d6",
			Profiles: map[string]core.ResolutionProfile{"pbta": {Ladder: []core.LadderStep{{Min: 7, Outcome: "weak"}}}},
		},
	}
	got, err := ApplyAdditions(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, spec) {
		t.Fatalf("applying nothing changed the spec:\n got %+v\nwant %+v", got, spec)
	}
}

func TestApplyAdditionsAddsAProfile(t *testing.T) {
	spec := &core.MechanicsSpec{}
	got, err := ApplyAdditions(spec, []Proposal{{Kind: "profile", Profile: &ProfileAddition{
		Name: "d20", DC: 15, Notation: "1d20",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := got.Checks.Profiles["d20"]
	if !ok || profile.DC != 15 {
		t.Fatalf("profiles = %+v", got.Checks.Profiles)
	}
}

func TestValidateEnhancementCatchesABrokenAddition(t *testing.T) {
	sys := System{Script: `onAction("do", () => ({ outcome: "miss" }))`, Mechanics: &core.MechanicsSpec{}}
	got := ValidateEnhancement(sys, []Proposal{{Kind: "skill", Skill: &core.SkillSpec{ID: "occult", Stat: "missing"}}})
	if got.OK {
		t.Fatal("a dangling reference should fail validation")
	}
	if got.FailureText() == "" {
		t.Fatal("a failed validation should explain why")
	}
}

func TestValidateEnhancementPassesAGoodAddition(t *testing.T) {
	sys := System{Mechanics: &core.MechanicsSpec{Checks: core.CheckConventions{Notation: "2d6"}}}
	got := ValidateEnhancement(sys, []Proposal{
		{Kind: "stat", Stat: &core.StatSpec{ID: "might"}},
		{Kind: "skill", Skill: &core.SkillSpec{ID: "brawl", Stat: "might"}},
	})
	if !got.OK {
		t.Fatalf("a good addition should validate, got %+v", got)
	}
}
