package engine

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type mockSessionOrchestratorModel struct {
	id          string
	model       string
	startCalls  int
	contCalls   int
	failCont    bool
	lastSession *harness.SessionHandle
}

func (m *mockSessionOrchestratorModel) ID() string    { return m.id }
func (m *mockSessionOrchestratorModel) Model() string { return m.model }

func (m *mockSessionOrchestratorModel) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: "Stateless reply."}, nil
}

func (m *mockSessionOrchestratorModel) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	out <- harness.StreamChunk{Text: "Streamed reply.", Done: true}
	return nil
}

func (m *mockSessionOrchestratorModel) StartSession(ctx context.Context, req harness.GenerateRequest) (*harness.SessionHandle, error) {
	m.startCalls++
	return &harness.SessionHandle{
		ID:           "sess-1",
		ThroughTurn:  1,
		CachedTokens: 10,
		Response: &harness.GenerateResponse{
			Text:         "Turn 1 narration from StartSession.",
			SessionID:    "sess-1",
			CachedTokens: 10,
		},
	}, nil
}

func (m *mockSessionOrchestratorModel) ContinueSession(ctx context.Context, session *harness.SessionHandle, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	m.contCalls++
	m.lastSession = session
	if m.failCont {
		return nil, errors.New("interaction session expired")
	}
	return &harness.GenerateResponse{
		Text:         "Turn 2 narration from ContinueSession.",
		SessionID:    "sess-2",
		CachedTokens: 42,
	}, nil
}

func TestOrchestratorSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.SaveEntity(&entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy tavern."})
	player := &entity.Entity{ID: "player", Name: "Sean", Type: "character"}
	store.SaveEntity(player)

	history := NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	bridge := rules.NewHostBridge(store, nil, "player")
	jsEngine := rules.NewJSEngine(bridge)

	model := &mockSessionOrchestratorModel{id: "mock-gemini", model: "gemini-2.5-flash"}
	router := harness.NewRouter()
	router.RegisterProvider(model)
	router.AssignRole("gm", "mock-gemini")

	timeline := NewTimeline(core.NewPathResolver(tempDir), store, history, "test-campaign")
	orchestrator := NewTurnOrchestrator(store, timeline, jsEngine, router, "tavern", "player")

	// Turn 1: starts session
	turn1, err := orchestrator.ProcessAction(ctx, "Do", "Look around")
	if err != nil {
		t.Fatalf("Turn 1 failed: %v", err)
	}
	if model.startCalls != 1 {
		t.Errorf("expected 1 StartSession call, got %d", model.startCalls)
	}
	if turn1.Context.Strategy != harness.StrategyFullPrompt {
		t.Errorf("turn 1 strategy = %s, want full_prompt", turn1.Context.Strategy)
	}
	if turn1.Context.Session == nil || turn1.Context.Session.ID != "sess-1" {
		t.Fatalf("expected turn 1 session sess-1, got %+v", turn1.Context.Session)
	}
	if turn1.Context.CachedTokens != 10 {
		t.Errorf("turn 1 cached tokens = %d, want 10", turn1.Context.CachedTokens)
	}

	// Turn 2: continues session
	turn2, err := orchestrator.ProcessAction(ctx, "Do", "Talk to bartender")
	if err != nil {
		t.Fatalf("Turn 2 failed: %v", err)
	}
	if model.contCalls != 1 {
		t.Errorf("expected 1 ContinueSession call, got %d", model.contCalls)
	}
	if turn2.Context.Strategy != harness.StrategyServerSession {
		t.Errorf("turn 2 strategy = %s, want server_session", turn2.Context.Strategy)
	}
	if turn2.Context.Session == nil || turn2.Context.Session.ID != "sess-2" {
		t.Fatalf("expected turn 2 session sess-2, got %+v", turn2.Context.Session)
	}
	if turn2.Context.CachedTokens != 42 {
		t.Errorf("turn 2 cached tokens = %d, want 42", turn2.Context.CachedTokens)
	}

	// Turn 3: continuation failure triggers fallback to full_prompt
	model.failCont = true
	turn3, err := orchestrator.ProcessAction(ctx, "Do", "Order a drink")
	if err != nil {
		t.Fatalf("Turn 3 failed: %v", err)
	}
	if turn3.Context.Strategy != harness.StrategyFullPrompt {
		t.Errorf("turn 3 strategy = %s, want full_prompt after fallback", turn3.Context.Strategy)
	}
	if model.startCalls != 2 {
		t.Errorf("expected StartSession to be called again on fallback, got %d", model.startCalls)
	}
}

func TestOrchestratorRewindClearsSession(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.SaveEntity(&entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy tavern."})
	player := &entity.Entity{ID: "player", Name: "Sean", Type: "character"}
	store.SaveEntity(player)

	history := NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	bridge := rules.NewHostBridge(store, nil, "player")
	jsEngine := rules.NewJSEngine(bridge)

	model := &mockSessionOrchestratorModel{id: "mock-gemini", model: "gemini-2.5-flash"}
	router := harness.NewRouter()
	router.RegisterProvider(model)
	router.AssignRole("gm", "mock-gemini")

	timeline := NewTimeline(core.NewPathResolver(tempDir), store, history, "test-campaign")
	orchestrator := NewTurnOrchestrator(store, timeline, jsEngine, router, "tavern", "player")

	_, err = orchestrator.ProcessAction(ctx, "Do", "Turn 1")
	if err != nil {
		t.Fatalf("Turn 1 failed: %v", err)
	}
	_, err = orchestrator.ProcessAction(ctx, "Do", "Turn 2")
	if err != nil {
		t.Fatalf("Turn 2 failed: %v", err)
	}

	// Rewind to turn 1
	if err := timeline.RewindToTurn(1); err != nil {
		t.Fatalf("RewindToTurn failed: %v", err)
	}

	// Turn 2 context was deleted from turn_contexts
	if _, _, err := store.GetTurnContext(2); err == nil {
		t.Fatalf("expected turn 2 context to be deleted after rewind")
	}

	// Now run turn 2 again: previous turn is turn 1 which had session sess-1.
	// It continues from turn 1's session cleanly!
	turn2Again, err := orchestrator.ProcessAction(ctx, "Do", "Turn 2 rerun")
	if err != nil {
		t.Fatalf("Turn 2 rerun failed: %v", err)
	}
	if turn2Again.Context.Strategy != harness.StrategyServerSession {
		t.Errorf("turn 2 rerun strategy = %s, want server_session", turn2Again.Context.Strategy)
	}
	if model.lastSession == nil || model.lastSession.ID != "sess-1" {
		t.Errorf("turn 2 rerun should have continued from turn 1 session sess-1, got %+v", model.lastSession)
	}
}

type mockSessionAndToolModel struct {
	mockSessionOrchestratorModel
	rounds int
}

func (m *mockSessionAndToolModel) ToolCallerCapable() bool { return true }

func (m *mockSessionAndToolModel) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	m.rounds++
	if len(req.Tools) == 0 {
		out <- harness.StreamChunk{Text: "No tools offered", Done: true}
		return nil
	}
	if m.rounds == 1 {
		out <- harness.StreamChunk{
			ToolCalls: []harness.ToolCall{{
				ID:        "call-1",
				Name:      "request_check",
				Arguments: `{"actor":"player","check_kind":"skill","stakes":"jump","outcomes":{"pass":"landed","fail":"fell"}}`,
			}},
			Done: true,
		}
		return nil
	}
	out <- harness.StreamChunk{Text: "You cleared the gap!", Done: true}
	return nil
}

func TestOrchestratorToolCallerWithSessionDoesNotBypassTools(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.SaveEntity(&entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy tavern."})
	player := &entity.Entity{ID: "player", Name: "Sean", Type: "character"}
	store.SaveEntity(player)

	history := NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	bridge := rules.NewHostBridge(store, nil, "player")
	jsEngine := rules.NewJSEngine(bridge)

	model := &mockSessionAndToolModel{
		mockSessionOrchestratorModel: mockSessionOrchestratorModel{id: "mock-tool-session", model: "gemini-2.5-flash"},
	}
	router := harness.NewRouter()
	router.RegisterProvider(model)
	router.AssignRole("gm", "mock-tool-session")

	timeline := NewTimeline(core.NewPathResolver(tempDir), store, history, "test-campaign")
	orchestrator := NewTurnOrchestrator(store, timeline, jsEngine, router, "tavern", "player")

	// Set tools so offersTools returns true
	fakeExec := &fakeExecutor{}
	orchestrator.SetTools(fakeExec, "yes")

	turn, err := orchestrator.ProcessAction(ctx, "Do", "I leap across the chasm")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}

	// Must have run tool loop and resolved the check
	if len(turn.Checks) != 1 {
		t.Fatalf("expected 1 check resolved via request_check in tool loop, got %d", len(turn.Checks))
	}
	if model.startCalls > 0 {
		t.Errorf("StartSession should not be called when tool calling is required, got %d calls", model.startCalls)
	}
}

