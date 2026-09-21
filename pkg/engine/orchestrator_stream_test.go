package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// scriptedStreamProvider streams a fixed set of chunks, optionally failing or
// blocking, so the orchestrator's streaming behaviour is testable without a model.
type scriptedStreamProvider struct {
	chunks []string
	err    error
	block  bool
}

func (p *scriptedStreamProvider) ID() string { return "scripted" }

func (p *scriptedStreamProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	var sb strings.Builder
	for _, chunk := range p.chunks {
		sb.WriteString(chunk)
	}
	return &harness.GenerateResponse{Text: sb.String()}, p.err
}

func (p *scriptedStreamProvider) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)

	if p.block {
		<-ctx.Done()
		return ctx.Err()
	}

	for _, chunk := range p.chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- harness.StreamChunk{Text: chunk}:
		}
	}
	return p.err
}

func streamingOrchestrator(t *testing.T, provider harness.ModelProvider) (*TurnOrchestrator, *Timeline, *storage.Store) {
	t.Helper()

	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Warm."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller.", Location: "[[alden-tavern]]"})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(provider)
	router.AssignRole("gm", provider.ID())

	return NewTurnOrchestrator(store, timeline, nil, router, "alden-tavern", "player"), timeline, store
}

func TestProcessActionStreamDeliversChunksInOrder(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"The docks ", "reek of ", "brine."}}
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)

	var received []string
	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", func(text string) error {
		received = append(received, text)
		return nil
	})
	if err != nil {
		t.Fatalf("ProcessActionStream failed: %v", err)
	}

	if strings.Join(received, "") != "The docks reek of brine." {
		t.Errorf("chunks = %v, want the narration in order", received)
	}
	if turn.Narration != strings.Join(received, "") {
		t.Errorf("Narration = %q, want the concatenated chunks", turn.Narration)
	}

	recorded, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 || recorded[0].Narration != turn.Narration {
		t.Errorf("expected the streamed turn recorded, got %+v", recorded)
	}
}

func TestProcessActionMatchesTheStreamingPipeline(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"Steel ", "rings."}}

	streamed, streamedTimeline, _ := streamingOrchestrator(t, provider)
	streamTurn, err := streamed.ProcessActionStream(context.Background(), "Do", "I swing", nil)
	if err != nil {
		t.Fatalf("ProcessActionStream failed: %v", err)
	}

	plain, plainTimeline, _ := streamingOrchestrator(t, provider)
	plainTurn, err := plain.ProcessAction(context.Background(), "Do", "I swing")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	if streamTurn.Narration != plainTurn.Narration || streamTurn.Mode != plainTurn.Mode {
		t.Errorf("pipelines disagree: streamed %+v, plain %+v", streamTurn, plainTurn)
	}
	if streamTurn.Location != plainTurn.Location {
		t.Errorf("Location differs: %q vs %q", streamTurn.Location, plainTurn.Location)
	}
	if len(streamTurn.Entities) != len(plainTurn.Entities) {
		t.Errorf("involvement differs: %+v vs %+v", streamTurn.Entities, plainTurn.Entities)
	}

	for _, timeline := range []*Timeline{streamedTimeline, plainTimeline} {
		turns, err := timeline.history.LoadHistory()
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) != 1 {
			t.Errorf("expected one recorded turn, got %d", len(turns))
		}
	}
}

func TestProcessActionStreamEmitsNothingForShortCircuitModes(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"should not be used"}}
	orchestrator, _, _ := streamingOrchestrator(t, provider)

	calls := 0
	if _, err := orchestrator.ProcessActionStream(context.Background(), "System", "/undo", func(string) error {
		calls++
		return nil
	}); err == nil {
		t.Errorf("expected /undo without turns to fail")
	}
	if calls != 0 {
		t.Errorf("/undo produced %d chunks, want none", calls)
	}

	if _, err := orchestrator.ProcessActionStream(context.Background(), "System", "/go Alden Tavern", func(string) error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("/go failed: %v", err)
	}
	if calls != 0 {
		t.Errorf("/go produced %d chunks, want none", calls)
	}
}
