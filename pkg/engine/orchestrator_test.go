package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
	"time"
)

type mockOrchestratorModel struct {
	lastPrompt string
	response   string
}

func (m *mockOrchestratorModel) ID() string { return "mock-gm" }
func (m *mockOrchestratorModel) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	m.lastPrompt = req.Prompt
	return &harness.GenerateResponse{Text: m.response}, nil
}
func (m *mockOrchestratorModel) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	m.lastPrompt = req.Prompt
	out <- harness.StreamChunk{Text: m.response, Done: true}
	return nil
}

func TestTurnOrchestrator(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed location and player
	store.SaveEntity(&entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy tavern."})
	player := &entity.Entity{ID: "player", Name: "Sean", Type: "character"}
	player.InitState(map[string]any{"hp": 25})
	store.SaveEntity(player)

	history := NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	bridge := rules.NewHostBridge(store, nil, "player")
	jsEngine := rules.NewJSEngine(bridge)

	model := &mockOrchestratorModel{response: "You step inside the warm tavern."}
	router := harness.NewRouter()
	router.RegisterProvider(model)
	router.AssignRole("gm", "mock-gm")

	timeline := NewTimeline(core.NewPathResolver(tempDir), store, history, "test-campaign")
	orchestrator := NewTurnOrchestrator(store, timeline, jsEngine, router, "tavern", "player")

	// 1. Play standard turn
	turn, err := orchestrator.ProcessAction(ctx, "Do", "I open the door")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	if turn.Number != 1 || turn.Prose() != "You step inside the warm tavern." {
		t.Errorf("unexpected turn outcome: %+v", turn)
	}

	// 2. Test GM director steering (/gm command)
	model.response = "Correction: The door was locked, but you pick it open."
	corrected, err := orchestrator.ProcessAction(ctx, "GM", "/gm The door was supposed to be locked.")
	if err != nil {
		t.Fatalf("ProcessAction GM steering failed: %v", err)
	}

	if !strings.Contains(corrected.Prose(), "Correction: The door was locked") {
		t.Errorf("expected corrected output, got %q", corrected.Prose())
	}

	// 3. Test Rules and Lore prompt injection
	orchestrator.SetPrompts("2d6 resolution ladder", "Gothic peat bogs")
	_, err = orchestrator.ProcessAction(ctx, "Do", "I check the lock")
	if err != nil {
		t.Fatalf("ProcessAction with prompts failed: %v", err)
	}
	if !strings.Contains(model.lastPrompt, "## SYSTEM RULES & RESOLUTION MECHANICS") || !strings.Contains(model.lastPrompt, "2d6 resolution ladder") {
		t.Errorf("expected rules prompt in GM prompt, got: %s", model.lastPrompt)
	}
	if !strings.Contains(model.lastPrompt, "## WORLD LORE & ATMOSPHERE") || !strings.Contains(model.lastPrompt, "Gothic peat bogs") {
		t.Errorf("expected lore prompt in GM prompt, got: %s", model.lastPrompt)
	}
}

func TestRecapCommandPrintsTheCurrentSummary(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"A generated scene."}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)

	// A chronicler with summarisation switched off answers from the note, so this
	// asserts the printing path without a second model call.
	chronicler := NewChronicler(timeline, store, nil)
	chronicler.SetEvery(0)
	orchestrator.SetChronicler(chronicler)

	if err := WriteChronicle(store, timeline.EntitiesDir(), Chronicle{
		Summary:     "The party reached the harbour.",
		ThroughTurn: 4,
	}); err != nil {
		t.Fatal(err)
	}

	turn, err := orchestrator.ProcessActionStream(context.Background(), "System", "/recap", nil)
	if err != nil {
		t.Fatalf("/recap failed: %v", err)
	}
	if !strings.Contains(turn.Narration, "The party reached the harbour.") {
		t.Errorf("expected the recap as the reply, got %q", turn.Narration)
	}
	if strings.Contains(turn.Narration, "A generated scene.") {
		t.Errorf("/recap must not narrate a scene")
	}

	// A command is not a turn: nothing is recorded for it.
	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 0 {
		t.Errorf("expected /recap to record nothing, got %d turns", len(turns))
	}
}

func TestRecapCommandRegeneratesAStaleSummary(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"Kael guards the harbour gate."}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)

	chronicler := NewChronicler(timeline, store, harness.NewSummariser(provider))
	chronicler.SetEvery(1)
	orchestrator.SetChronicler(chronicler)

	// The chronicler reads the campaign's log, which the fixture writes where the
	// resolver says it is.
	recordTurnForRecap(t, timeline, 1, "The party reached the harbour.")
	recordTurnForRecap(t, timeline, 2, "Kael mentioned the oil was low.")
	if err := WriteChronicle(store, timeline.EntitiesDir(), Chronicle{Summary: "They left the tavern.", ThroughTurn: 0}); err != nil {
		t.Fatal(err)
	}

	turn, err := orchestrator.ProcessActionStream(context.Background(), "System", "/recap", nil)
	if err != nil {
		t.Fatalf("/recap failed: %v", err)
	}
	if !strings.Contains(turn.Narration, "Kael guards the harbour gate.") {
		t.Errorf("expected a regenerated recap, got %q", turn.Narration)
	}
}

func recordTurnForRecap(t *testing.T, timeline *Timeline, number int, narration string) {
	t.Helper()

	turn := Turn{Number: number, Timestamp: time.Now(), Mode: "Do", Input: "I look around", Narration: narration}
	if err := timeline.RecordTurn(&turn, nil); err != nil {
		t.Fatalf("record turn %d: %v", number, err)
	}
}

func TestATurnRecordsWhatItsProseContradicts(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"She remembers Oakhaven Tavern fondly."}}
	orchestrator, timeline, store := streamingOrchestrator(t, provider)
	orchestrator.SetContinuityChecks(true)
	orchestrator.SetChronicler(nil)

	if err := store.SaveEntity(&entity.Entity{ID: "oakhaven-tavern", Name: "Oakhaven Tavern", Type: "location", Body: "Ale."}); err != nil {
		t.Fatal(err)
	}

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil)
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	if len(turn.ContinuityNotes) == 0 {
		t.Fatalf("expected a continuity note about the named location")
	}

	// The note is persisted with the turn, so a reader sees it later.
	turns, err := timeline.history.LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || len(turns[0].ContinuityNotes) == 0 {
		t.Errorf("expected the note recorded, got %+v", turns)
	}
}

func TestContinuityChecksCanBeSwitchedOff(t *testing.T) {
	provider := &scriptedStreamProvider{chunks: []string{"She remembers Oakhaven Tavern fondly."}}
	orchestrator, _, store := streamingOrchestrator(t, provider)
	orchestrator.SetContinuityChecks(false)

	if err := store.SaveEntity(&entity.Entity{ID: "oakhaven-tavern", Name: "Oakhaven Tavern", Type: "location", Body: "Ale."}); err != nil {
		t.Fatal(err)
	}

	turn, err := orchestrator.ProcessActionStream(context.Background(), "Do", "I look around", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(turn.ContinuityNotes) != 0 {
		t.Errorf("expected no notes when the checks are off, got %v", turn.ContinuityNotes)
	}
}
