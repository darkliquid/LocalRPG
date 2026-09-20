package tui

import (
	"context"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

	orch := engine.NewTurnOrchestrator(store, history, nil, router, "tavern", "player")

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
