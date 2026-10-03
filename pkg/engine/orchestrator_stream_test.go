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
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// scriptedStreamProvider streams a fixed set of chunks, optionally failing or
// blocking, so the orchestrator's streaming behaviour is testable without a model.
type scriptedStreamProvider struct {
	chunks    []string
	toolCalls []harness.ToolCall
	err       error
	block     bool
	// chunksPerCall, when set, supplies a different chunk list for each Stream
	// call, so a continuation can be scripted separately from the first call.
	chunksPerCall [][]string
	calls         int
	// onRequest is called with each request the provider is given, so a test can
	// assert what it was asked rather than only what it replied.
	onRequest func(harness.GenerateRequest)
}

func (p *scriptedStreamProvider) ID() string { return "scripted" }

func (p *scriptedStreamProvider) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	if p.onRequest != nil {
		p.onRequest(req)
	}

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

	chunks := p.chunks
	if len(p.chunksPerCall) > 0 {
		chunks = nil
		if p.calls < len(p.chunksPerCall) {
			chunks = p.chunksPerCall[p.calls]
		}
		p.calls++
	}

	for _, chunk := range chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- harness.StreamChunk{Text: chunk}:
		}
	}
	out <- harness.StreamChunk{Done: true, ToolCalls: p.toolCalls}
	return p.err
}

func streamingOrchestrator(t *testing.T, provider harness.ModelProvider) (*TurnOrchestrator, *Timeline, *storage.Store) {
	t.Helper()

	tempDir := t.TempDir()
	store := newTestStore(t)
	paths := core.NewPathResolver(tempDir)
	// The log lives where the resolver says it does, so anything that reads a
	// campaign's history from disk sees the turns this fixture records.
	if err := os.MkdirAll(paths.GameDir("campaign-01"), 0755); err != nil {
		t.Fatal(err)
	}
	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(paths.GameDir("campaign-01"), "history.jsonl")), "campaign-01")

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

func TestCancellationRecordsNothing(t *testing.T) {
	provider := &scriptedStreamProvider{block: true}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)

	ctx, cancel := context.WithCancel(context.Background())
	go cancel()

	if _, err := orchestrator.ProcessActionStream(ctx, "Do", "I wait", nil); err == nil {
		t.Fatalf("expected a cancelled turn to fail")
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected no recorded turn, got %+v", turns)
	}

	if count, err := store.CountTurns(); err != nil || count != 0 {
		t.Errorf("CountTurns = %d, %v; want 0", count, err)
	}

	player, err := store.GetEntity("player")
	if err != nil {
		t.Fatal(err)
	}
	if len(player.History) != 0 {
		t.Errorf("expected no involvement recorded, got %v", player.History)
	}
}

func TestChunkFailureAbortsBeforeRecording(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"The docks ", "reek "}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)

	clientGone := errors.New("client disconnected")
	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look", func(string) error {
		return clientGone
	}); !errors.Is(err, clientGone) {
		t.Fatalf("expected the listener's error, got %v", err)
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected nothing recorded, got %+v", turns)
	}
	if count, _ := store.CountTurns(); count != 0 {
		t.Errorf("CountTurns = %d, want 0", count)
	}
}

func TestMidStreamProviderFailureKeepsPartialText(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"Steel rings, "}, err: errors.New("model exploded")}
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I swing", nil)
	if err != nil {
		t.Fatalf("expected the partial reply to survive, got %v", err)
	}
	if turn.Narration != "Steel rings, " {
		t.Errorf("Narration = %q, want the text that arrived", turn.Narration)
	}
	if !turn.Truncated {
		t.Errorf("expected the unrepaired reply to be marked incomplete")
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Errorf("expected the partial turn recorded, got %+v", turns)
	}
}

func TestProviderFailureBeforeAnyTextRecordsNothing(t *testing.T) {
	provider := &scriptedStreamProvider{err: errors.New("model exploded")}
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I swing", nil); err == nil {
		t.Fatalf("expected a failure with no text to surface")
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected nothing recorded, got %+v", turns)
	}
}

func TestEmptyNarrationRecordsNothing(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{""}}
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait", nil); err == nil {
		t.Fatalf("expected empty narration to fail")
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected nothing recorded, got %+v", turns)
	}
}

func TestGenerationStallsWhenNoChunkArrives(t *testing.T) {
	provider := &scriptedStreamProvider{block: true}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)
	orchestrator.SetChunkTimeout(20 * time.Millisecond)

	_, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I wait", nil)
	if !errors.Is(err, ErrGenerationStalled) {
		t.Fatalf("expected ErrGenerationStalled, got %v", err)
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected no recorded turn, got %+v", turns)
	}
	if count, err := store.CountTurns(); err != nil || count != 0 {
		t.Errorf("CountTurns = %d, %v; want 0", count, err)
	}
}

func TestOpeningModeEstablishesTheFirstTurnOnly(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"Rain hammers the market."}}
	orchestrator, timeline, _ := streamingOrchestrator(t, provider)
	orchestrator.SetOpeningPrompt("Begin at dusk in the market.")

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Opening", "", nil)
	if err != nil {
		t.Fatalf("opening turn failed: %v", err)
	}
	if turn.Mode != OpeningMode {
		t.Errorf("Mode = %q, want %q", turn.Mode, OpeningMode)
	}
	if turn.Number != 1 {
		t.Errorf("Number = %d, want 1", turn.Number)
	}

	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected one recorded turn, got %d", len(turns))
	}

	if _, err := orchestrator.ProcessActionStream(context.Background(), "Opening", "", nil); err == nil {
		t.Errorf("expected a second opening turn to be refused")
	}
}

func TestStreamSurfacesToolCalls(t *testing.T) {
	provider := &scriptedStreamProvider{
		chunks:    []string{"let me check"},
		toolCalls: []harness.ToolCall{{ID: "1", Name: "search_entities", Arguments: `{"query":"warden"}`}},
	}
	orchestrator, _, _ := streamingOrchestrator(t, provider)

	result, err := orchestrator.generateRequest(context.Background(), harness.GenerateRequest{
		Messages: []harness.Message{{Role: "user", Content: "who is the warden?"}},
	}, nil)
	if err != nil {
		t.Fatalf("generateRequest: %v", err)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "search_entities" {
		t.Errorf("ToolCalls = %+v, want the stream's call", result.ToolCalls)
	}
}
