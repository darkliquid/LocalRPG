package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func newReplaceTimeline(t *testing.T) *Timeline {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	history := NewHistoryLogger(filepath.Join(dir, "history.jsonl"))
	return NewTimeline(core.NewPathResolver(dir), store, history, "campaign-01")
}

func TestReplaceTurnOnlyFinal(t *testing.T) {
	tl := newReplaceTimeline(t)
	ctx := context.Background()
	if err := tl.RecordTurnContext(ctx, &Turn{Number: 1, Narration: "one"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := tl.RecordTurnContext(ctx, &Turn{Number: 2, Narration: "two"}, nil); err != nil {
		t.Fatal(err)
	}

	// Replacing a non-final turn is refused.
	if err := tl.ReplaceTurn(ctx, &Turn{Number: 1, Narration: "nope"}); err == nil {
		t.Fatal("replacing a non-final turn should error")
	}

	// Replacing the final turn completes it in place.
	replacement := Turn{Number: 2, Narration: "two, revised", ResolvesCheckRef: "r"}
	if err := tl.ReplaceTurn(ctx, &replacement); err != nil {
		t.Fatal(err)
	}
	turns, err := tl.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[1].Narration != "two, revised" {
		t.Fatalf("turns = %+v", turns)
	}
}

func TestDraftTurnRoundTrips(t *testing.T) {
	tl := newReplaceTimeline(t)
	turn := Turn{Number: 1, Draft: true, PendingCheck: &harness.PendingCheck{Ref: "r"}}
	if err := tl.RecordTurnContext(context.Background(), &turn, nil); err != nil {
		t.Fatal(err)
	}
	turns, err := tl.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || !turns[0].Draft {
		t.Fatalf("the draft flag did not survive the round trip: %+v", turns)
	}
}
