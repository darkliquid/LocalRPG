package sysgen

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/refsystems"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/systemtest"
)

// TestGeneratedPbtAProfileResolvesLikeTheCorpus guards that a ladder template
// built from a description resolves the same totals the reference narrative_2d6
// system's script does, so a generated system and the corpus stay in step.
func TestGeneratedPbtAProfileResolvesLikeTheCorpus(t *testing.T) {
	spec, err := Template{Resolution: "ladder", Health: "single", Notation: "2d6"}.Build(Params{
		Stats:      []core.StatSpec{{ID: "might"}},
		Notation:   "2d6",
		Ladder:     []core.LadderStep{{Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}, {Min: 0, Outcome: "miss"}},
		HealthStat: "might",
	})
	if err != nil {
		t.Fatal(err)
	}

	profile := spec.Checks.Profiles["ladder"]
	for total, want := range map[int]string{12: "strong", 10: "strong", 9: "weak", 7: "weak", 6: "miss", 2: "miss"} {
		got, decided := rules.ResolveProfile(profile, total, 0)
		if !decided || got != want {
			t.Errorf("total %d resolved to %q (decided=%v), want %q", total, got, decided, want)
		}
	}

	corpus, ok := refsystems.Get("narrative_2d6")
	if !ok {
		t.Fatal("narrative_2d6 should be shipped")
	}
	corpusLadder := corpus.Mechanics.Checks.Profiles["pbta"].Ladder
	if len(corpusLadder) != len(profile.Ladder) {
		t.Fatalf("generated ladder %+v does not match the corpus %+v", profile.Ladder, corpusLadder)
	}
	for i := range corpusLadder {
		if corpusLadder[i] != profile.Ladder[i] {
			t.Fatalf("generated ladder %+v does not match the corpus %+v", profile.Ladder, corpusLadder)
		}
	}

	sys := systemtest.System{ID: "generated-pbta", Mechanics: spec}
	scenario := systemtest.Scenario{Name: "smoke", Steps: []systemtest.Step{{Action: "check", Input: "ladder"}}}
	if failures := systemtest.Run(sys, scenario); len(failures) > 0 {
		t.Fatalf("generated system failed the harness: %+v", failures)
	}
}
