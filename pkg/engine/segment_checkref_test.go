package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestBuildSegmentsCarriesCheckRef(t *testing.T) {
	sub := &harness.TurnSubmission{
		Segments: []harness.SegmentSpec{
			{Kind: "narration", Text: "One.", CheckRef: "chk_a"},
			{Kind: "speech", Speaker: "Elena", Text: "Two.", CheckRef: "chk_b"},
			{Kind: "speech", Speaker: "Nobody", Text: "Three.", CheckRef: "chk_c"},
		},
	}
	resolve := func(speaker string) (string, bool) {
		if speaker == "Elena" {
			return "elena", true
		}
		return "", false
	}
	_, segments := buildSegments(sub, resolve)
	if len(segments) != 3 {
		t.Fatalf("segments = %d, want 3", len(segments))
	}
	want := []string{"chk_a", "chk_b", "chk_c"}
	for i, ref := range want {
		if segments[i].CheckRef != ref {
			t.Errorf("segment %d CheckRef = %q, want %q", i, segments[i].CheckRef, ref)
		}
	}
}

type stubCheckResolver struct{}

func (stubCheckResolver) Resolve(context.Context, harness.CheckRequest, *entity.Entity) (*harness.CheckResult, error) {
	return &harness.CheckResult{CheckID: "chk_x", Outcome: "pass"}, nil
}

func TestResolveCheckCopiesKindAndStakes(t *testing.T) {
	o := &TurnOrchestrator{checkResolver: stubCheckResolver{}}
	got, err := o.resolveCheck(context.Background(), harness.CheckRequest{
		CheckKind: "stealth", Stakes: "The guard wakes",
	}, nil)
	if err != nil {
		t.Fatalf("resolveCheck failed: %v", err)
	}
	if got.CheckKind != "stealth" || got.Stakes != "The guard wakes" {
		t.Fatalf("kind=%q stakes=%q", got.CheckKind, got.Stakes)
	}
}
