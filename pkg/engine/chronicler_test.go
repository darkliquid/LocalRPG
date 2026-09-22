package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// chroniclerFixture wires a campaign whose history lives where the resolver says it
// does, because the chronicler reads the log rather than the timeline's handle.
func chroniclerFixture(t *testing.T, every int) (*Chronicler, *Timeline, *scriptedStreamProvider, *storage.Store) {
	t.Helper()

	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)
	store := newTestStore(t)
	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(paths.GameDir("campaign-01"), "history.jsonl")), "campaign-01")

	// The history log lives under the campaign's directory, which nothing creates
	// for a fixture.
	entitiesDir := timeline.EntitiesDir()
	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	provider := &scriptedStreamProvider{chunks: []string{"The party reached the harbour."}}
	chronicler := NewChronicler(timeline, store, harness.NewSummariser(provider))
	chronicler.SetEvery(every)
	return chronicler, timeline, provider, store
}

func recordTurn(t *testing.T, timeline *Timeline, number int, narration string) {
	t.Helper()

	turn := Turn{
		Number:    number,
		Timestamp: time.Now(),
		Mode:      "Do",
		Input:     "I look around",
		Narration: narration,
	}
	if err := timeline.RecordTurn(&turn, nil); err != nil {
		t.Fatalf("record turn %d: %v", number, err)
	}
}

func TestChroniclerIsNotDueBeforeTheCadence(t *testing.T) {
	chronicler, timeline, _, _ := chroniclerFixture(t, 3)
	for i := 1; i <= 2; i++ {
		recordTurn(t, timeline, i, "Nothing much happens.")
	}

	due, err := chronicler.Due("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if due {
		t.Errorf("expected no regeneration before the cadence is reached")
	}
}

func TestChroniclerRegeneratesAtTheCadenceAndRecordsHowFarItReached(t *testing.T) {
	chronicler, timeline, _, _ := chroniclerFixture(t, 3)
	for i := 1; i <= 3; i++ {
		recordTurn(t, timeline, i, "Nothing much happens.")
	}

	due, err := chronicler.Due("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if !due {
		t.Fatalf("expected the cadence to make a regeneration due")
	}

	changed, err := chronicler.Regenerate(context.Background(), "campaign-01")
	if err != nil {
		t.Fatalf("Regenerate failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected the chronicle to be written")
	}

	chronicle, err := chronicler.Recap("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if chronicle.ThroughTurn != 3 {
		t.Errorf("ThroughTurn = %d, want 3", chronicle.ThroughTurn)
	}
	if !strings.Contains(chronicle.Summary, "harbour") {
		t.Errorf("summary = %q", chronicle.Summary)
	}

	due, err = chronicler.Due("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if due {
		t.Errorf("expected the chronicle to be current once regenerated")
	}
}

func TestChroniclerSummarisesOnlyWhatTheSummaryDoesNotCover(t *testing.T) {
	chronicler, timeline, provider, store := chroniclerFixture(t, 1)

	var prompt string
	provider.onRequest = func(req harness.GenerateRequest) {
		prompt = req.Prompt
	}

	recordTurn(t, timeline, 1, "The first turn.")
	recordTurn(t, timeline, 2, "The second turn.")

	if err := WriteChronicle(store, timeline.EntitiesDir(), Chronicle{Summary: "A previous summary.", ThroughTurn: 1}); err != nil {
		t.Fatal(err)
	}

	if _, err := chronicler.Regenerate(context.Background(), "campaign-01"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(prompt, "The second turn.") {
		t.Errorf("expected the uncovered turn in the prompt:\n%s", prompt)
	}
	if strings.Contains(prompt, "The first turn.") {
		t.Errorf("a covered turn must not be summarised again:\n%s", prompt)
	}
	if !strings.Contains(prompt, "A previous summary.") {
		t.Errorf("expected the previous summary to prime the rewrite:\n%s", prompt)
	}
}

func TestChroniclerLeavesThroughTurnAloneWhenTheProviderFails(t *testing.T) {
	chronicler, timeline, provider, _ := chroniclerFixture(t, 1)
	provider.err = errors.New("model unavailable")

	recordTurn(t, timeline, 1, "Nothing much happens.")

	if _, err := chronicler.Regenerate(context.Background(), "campaign-01"); err == nil {
		t.Fatalf("expected the provider failure to surface")
	}

	chronicle, err := chronicler.Recap("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if chronicle.ThroughTurn != 0 || chronicle.Summary != "" {
		t.Errorf("a failed regeneration must leave the chronicle untouched, got %+v", chronicle)
	}

	// The next trigger retries rather than the range being lost.
	due, err := chronicler.Due("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if !due {
		t.Errorf("expected the campaign to still be due after a failure")
	}
}

func TestChroniclerDisabledWritesNothing(t *testing.T) {
	chronicler, timeline, _, _ := chroniclerFixture(t, 0)
	recordTurn(t, timeline, 1, "Nothing much happens.")

	due, err := chronicler.Due("campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if due {
		t.Errorf("zero cadence must disable summarisation")
	}

	changed, err := chronicler.Regenerate(context.Background(), "campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Errorf("a disabled chronicler must write nothing")
	}
}
