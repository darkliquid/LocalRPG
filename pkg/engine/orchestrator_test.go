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
	player.InitState(map[string]interface{}{"hp": 25})
	store.SaveEntity(player)

	history := NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	bridge := rules.NewHostBridge(store)
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
