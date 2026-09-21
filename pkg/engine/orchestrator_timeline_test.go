package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type mockTimelineModel struct {
	response string
}

func (m *mockTimelineModel) ID() string { return "mock-timeline" }

func (m *mockTimelineModel) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: m.response}, nil
}

func (m *mockTimelineModel) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	out <- harness.StreamChunk{Text: m.response, Done: true}
	return nil
}

func newWiredOrchestrator(t *testing.T, gmResponse string) (*TurnOrchestrator, *Timeline, *harness.Router) {
	t.Helper()

	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller."})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: gmResponse})
	router.AssignRole("gm", "mock-gm")

	return NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player"), timeline, router
}

func TestProcessActionRecordsEntitiesAndTimeline(t *testing.T) {
	orchestrator, timeline, _ := newWiredOrchestrator(t, "Garrick the Fence glances up.")
	orchestrator.SetExtractor(harness.NewExtractor(&mockTimelineModel{
		response: `[{"id":"garrick-the-fence","name":"Garrick the Fence","type":"character","body":"A shadowy broker."}]`,
	}))

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I look for Garrick")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	notePath := filepath.Join(timeline.EntitiesDir(), "garrick-the-fence.md")
	if _, err := os.Stat(notePath); err != nil {
		t.Fatalf("expected the extracted entity note on disk: %v", err)
	}

	indexed, err := orchestrator.store.ListTurnsForEntity("garrick-the-fence")
	if err != nil {
		t.Fatalf("ListTurnsForEntity failed: %v", err)
	}
	if len(indexed) != 1 || indexed[0] != turn.Number {
		t.Errorf("indexed turns = %v, want [%d]", indexed, turn.Number)
	}

	if len(turn.Entities) != 3 {
		t.Errorf("expected player, location, and extracted involvement, got %+v", turn.Entities)
	}

	recorded, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(recorded) != 1 || len(recorded[0].Entities) != 3 {
		t.Errorf("expected the turn to be recorded with involvement, got %+v", recorded)
	}
}

func TestProcessActionSurvivesExtractorFailure(t *testing.T) {
	orchestrator, _, _ := newWiredOrchestrator(t, "Nothing stirs.")
	orchestrator.SetExtractor(harness.NewExtractor(&mockTimelineModel{response: "not json at all"}))

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I listen")
	if err != nil {
		t.Fatalf("a failed extractor must not fail the turn: %v", err)
	}
	if turn.Number != 1 {
		t.Errorf("expected the turn to be recorded, got %+v", turn)
	}
	if len(turn.Entities) != 2 {
		t.Errorf("expected player and location involvement only, got %+v", turn.Entities)
	}
}
