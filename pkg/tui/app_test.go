package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type mockTUIModel struct {
	output string
}

func (m *mockTUIModel) ID() string { return "tui-mock" }
func (m *mockTUIModel) Generate(ctx context.Context, req harness.GenerateRequest) (*harness.GenerateResponse, error) {
	return &harness.GenerateResponse{Text: m.output}, nil
}
func (m *mockTUIModel) Stream(ctx context.Context, req harness.GenerateRequest, out chan<- harness.StreamChunk) error {
	defer close(out)
	out <- harness.StreamChunk{Text: m.output, Done: true}
	return nil
}

func TestTUIModelInitializationAndInput(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewStore(filepath.Join(tempDir, "index.db"))
	defer store.Close()

	store.SaveEntity(&entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Quiet place."})
	store.SaveEntity(&entity.Entity{ID: "player", Name: "Sean", Type: "character"})

	history := engine.NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	router := harness.NewRouter()
	router.RegisterProvider(&mockTUIModel{output: "Welcome traveler."})
	router.AssignRole("gm", "tui-mock")

	timeline := engine.NewTimeline(core.NewPathResolver(tempDir), store, history, "test-campaign")
	orch := engine.NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")

	app := NewAppModel(orch, 80, 24)
	if app.mode != "Do" {
		t.Errorf("expected initial mode 'Do', got %q", app.mode)
	}

	// Test switching mode via Tab
	msg := tea.KeyMsg{Type: tea.KeyTab}
	newModel, _ := app.Update(msg)
	updatedApp := newModel.(*AppModel)

	if updatedApp.mode != "Say" {
		t.Errorf("expected mode 'Say' after Tab, got %q", updatedApp.mode)
	}
}

func TestTUIViewShowsTheLocation(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewStore(filepath.Join(tempDir, "index.db"))
	defer store.Close()

	store.SaveEntity(&entity.Entity{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Body: "Quiet place.", Hash: "h1"})
	store.SaveEntity(&entity.Entity{ID: "player", Name: "Sean", Type: "character", Location: "[[alden-tavern]]", Hash: "h2"})

	history := engine.NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	timeline := engine.NewTimeline(core.NewPathResolver(tempDir), store, history, "test-campaign")
	router := harness.NewRouter()
	router.RegisterProvider(&mockTUIModel{output: "hi"})
	router.AssignRole("gm", "tui-mock")

	orch := engine.NewTurnOrchestrator(store, timeline, nil, router, "alden-tavern", "player")
	app := NewAppModel(orch, 80, 24)

	if !strings.Contains(app.View(), "Alden Tavern") {
		t.Errorf("expected the view to name the current location, got %q", app.View())
	}
}

func TestSeedShowsAnEarlierTurn(t *testing.T) {
	model := NewAppModel(nil, 80, 24)
	model.Seed([]engine.Turn{{Number: 1, Mode: "Opening", Narration: "The market burns."}})

	if len(model.history) != 1 {
		t.Fatalf("history = %d turns, want 1", len(model.history))
	}
	if model.history[0].Mode != "Opening" {
		t.Errorf("Mode = %q, want %q", model.history[0].Mode, "Opening")
	}
}
