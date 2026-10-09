package sysgen

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestBuildLadderTemplate(t *testing.T) {
	spec, err := Template{Resolution: "ladder", Health: "single"}.Build(Params{
		Stats:      []core.StatSpec{{ID: "might"}},
		Ladder:     []core.LadderStep{{Min: 10, Outcome: "strong"}, {Min: 0, Outcome: "miss"}},
		HealthStat: "might",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Checks.Profiles) != 1 {
		t.Fatalf("spec = %+v", spec)
	}
	if spec.Health == nil || spec.Health.Stat != "might" {
		t.Fatalf("health = %+v", spec.Health)
	}
	if len(spec.Checks.Outcome) != 2 || spec.Checks.Outcome[0] != "strong" {
		t.Fatalf("outcomes = %+v", spec.Checks.Outcome)
	}
}

func TestBuildRejectsBadParams(t *testing.T) {
	if _, err := (Template{Resolution: "ladder"}).Build(Params{}); err == nil {
		t.Fatal("an empty ladder should error")
	}
	if _, err := (Template{Resolution: "dc"}).Build(Params{}); err == nil {
		t.Fatal("a zero DC should error")
	}
	if _, err := (Template{Resolution: "pool"}).Build(Params{SuccessOn: ">=8"}); err == nil {
		t.Fatal("a pool without outcomes should error")
	}
	if _, err := (Template{Resolution: "pool"}).Build(Params{Outcomes: []core.SuccessOutcome{{Min: 1, Max: -1, Outcome: "strong"}}}); err == nil {
		t.Fatal("a pool without a success target should error")
	}
	if _, err := (Template{Resolution: "ladder", Health: "single"}).Build(Params{
		Ladder: []core.LadderStep{{Min: 7, Outcome: "weak"}},
	}); err == nil {
		t.Fatal("a health template without a health stat should error")
	}
	if _, err := (Template{Resolution: "nonsense"}).Build(Params{}); err == nil {
		t.Fatal("an unknown resolution should error")
	}
}

func TestBuildDCPoolAndAdvancement(t *testing.T) {
	dc, err := Template{Resolution: "dc", Notation: "1d20"}.Build(Params{DC: 15})
	if err != nil {
		t.Fatal(err)
	}
	profile := dc.Checks.Profiles["dc"]
	if profile.DC != 15 || dc.Checks.Notation != "1d20" {
		t.Fatalf("dc profile = %+v", profile)
	}

	pool, err := (Template{Resolution: "pool", Health: "none"}).Build(Params{
		SuccessOn: ">=8",
		Outcomes: []core.SuccessOutcome{
			{Min: 3, Max: -1, Outcome: "strong"},
			{Min: 1, Max: 2, Outcome: "weak"},
			{Min: 0, Max: 0, Outcome: "miss"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := pool.Checks.Profiles["pool"].SuccessOn; got != ">=8" {
		t.Fatalf("success_on = %q", got)
	}
	if len(pool.Checks.Outcome) != 3 || pool.Checks.Outcome[0] != "strong" {
		t.Fatalf("outcomes = %+v", pool.Checks.Outcome)
	}

	adv, err := (Template{Resolution: "dc", Health: "none", Advancement: "spend"}).Build(Params{
		DC:              12,
		AdvancementStat: "xp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if adv.Advancement == nil || adv.Advancement.Mode != "spend" || adv.Advancement.Currency.Stat != "xp" {
		t.Fatalf("advancement = %+v", adv.Advancement)
	}
}

func TestTemplatesAllBuild(t *testing.T) {
	for _, tpl := range Templates() {
		params := Params{
			Stats:           []core.StatSpec{{ID: "might"}},
			Notation:        tpl.Notation,
			Ladder:          []core.LadderStep{{Min: 10, Outcome: "strong"}, {Min: 0, Outcome: "miss"}},
			DC:              15,
			SuccessOn:       ">=8",
			Outcomes:        []core.SuccessOutcome{{Min: 1, Max: -1, Outcome: "strong"}, {Min: 0, Max: 0, Outcome: "miss"}},
			HealthStat:      "might",
			AdvancementStat: "xp",
		}
		spec, err := tpl.Build(params)
		if err != nil {
			t.Fatalf("template %s did not build: %v", tpl.ID, err)
		}
		if len(spec.Checks.Profiles) != 1 {
			t.Fatalf("template %s built %d profiles", tpl.ID, len(spec.Checks.Profiles))
		}
		if problems := spec.Checks.Validate(); len(problems) > 0 {
			t.Fatalf("template %s built an invalid spec: %v", tpl.ID, problems)
		}
	}
}

func TestMatchTemplateFallsBackToClosest(t *testing.T) {
	if _, ok := MatchTemplate("ladder", "single", "none"); !ok {
		t.Fatal("an exact match should be found")
	}
	tpl, ok := MatchTemplate("ladder", "wounds", "none")
	if !ok || tpl.Resolution != "ladder" {
		t.Fatalf("a near-miss should fall back to the resolution: %+v %v", tpl, ok)
	}
	if _, ok := MatchTemplate("nonsense", "none", "none"); ok {
		t.Fatal("an unknown resolution should not match")
	}
}
