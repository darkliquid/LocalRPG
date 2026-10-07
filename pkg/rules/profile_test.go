package rules

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestResolveProfile(t *testing.T) {
	pbta := core.ResolutionProfile{Ladder: []core.LadderStep{
		{Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}, {Min: 0, Outcome: "miss"}}}
	cases := []struct {
		total, succ int
		want        string
	}{
		{12, 0, "strong"}, {10, 0, "strong"}, {9, 0, "weak"}, {7, 0, "weak"}, {6, 0, "miss"},
	}
	for _, c := range cases {
		got, ok := ResolveProfile(pbta, c.total, c.succ)
		if !ok || got != c.want {
			t.Errorf("total %d: got %q ok %v, want %q", c.total, got, ok, c.want)
		}
	}
	d20 := core.ResolutionProfile{DC: 15}
	if got, _ := ResolveProfile(d20, 15, 0); got != "success" {
		t.Errorf("dc met: got %q", got)
	}
	if got, _ := ResolveProfile(d20, 14, 0); got != "fail" {
		t.Errorf("dc missed: got %q", got)
	}
	pool := core.ResolutionProfile{SuccessOn: ">=8", Outcomes: []core.SuccessOutcome{
		{Min: 3, Max: -1, Outcome: "strong"}, {Min: 1, Max: 2, Outcome: "weak"}, {Min: 0, Max: 0, Outcome: "miss"}}}
	if got, _ := ResolveProfile(pool, 0, 3); got != "strong" {
		t.Errorf("pool 3: got %q", got)
	}
	if _, ok := ResolveProfile(core.ResolutionProfile{}, 5, 0); ok {
		t.Error("an empty profile must not decide")
	}
}

func TestResolveProfileLadderIsOrderIndependent(t *testing.T) {
	unsorted := core.ResolutionProfile{Ladder: []core.LadderStep{
		{Min: 0, Outcome: "miss"}, {Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}}}
	for total, want := range map[int]string{12: "strong", 10: "strong", 9: "weak", 6: "miss"} {
		if got, ok := ResolveProfile(unsorted, total, 0); !ok || got != want {
			t.Errorf("total %d: got %q ok %v, want %q", total, got, ok, want)
		}
	}
}

func TestResolveProfileLadderCoversEveryTotal(t *testing.T) {
	// A ladder covering all totals must map every total to exactly one outcome.
	// "Exactly one" is asserted by the outcome always being a member of the set.
	p := core.ResolutionProfile{Ladder: []core.LadderStep{
		{Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}, {Min: 0, Outcome: "miss"}}}
	allowed := map[string]bool{"strong": true, "weak": true, "miss": true}
	for total := -5; total <= 25; total++ {
		got, ok := ResolveProfile(p, total, 0)
		if !ok {
			t.Fatalf("total %d: ladder covered all totals but decided nothing", total)
		}
		if !allowed[got] {
			t.Fatalf("total %d: got %q, not a declared outcome", total, got)
		}
	}
}

func TestClampTo(t *testing.T) {
	if got := ClampTo([]string{"risky", "controlled"}, "risky"); got != "risky" {
		t.Errorf("clamp kept: got %q", got)
	}
	if got := ClampTo([]string{"risky", "controlled"}, "nonsense"); got != "risky" {
		t.Errorf("clamp fell back: got %q", got)
	}
	if got := ClampTo(nil, "risky"); got != "" {
		t.Errorf("clamp empty list: got %q", got)
	}
}
