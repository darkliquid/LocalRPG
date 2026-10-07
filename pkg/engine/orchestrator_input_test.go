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

func TestProcessActionPreservesRawInput(t *testing.T) {
	tempDir := t.TempDir()

	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	store.SaveEntity(&entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy tavern.", Hash: "h1"})
	store.SaveEntity(&entity.Entity{ID: "player", Name: "Sean", Type: "character", Hash: "h2"})

	history := NewHistoryLogger(filepath.Join(tempDir, "history.jsonl"))
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, history, "test-campaign")
	model := &mockOrchestratorModel{response: "The dice settle."}
	router := harness.NewRouter()
	router.RegisterProvider(model)
	router.AssignRole("gm", "mock-gm")

	orchestrator := NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")

	turn, err := orchestrator.ProcessAction(context.Background(), "Roll", "1d20+5")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if turn.Input != "1d20+5" {
		t.Errorf("Input = %q, want the raw player entry", turn.Input)
	}
	// A player-initiated roll ends the turn on a pending check the player
	// resolves, rather than a dead proposal directive.
	if turn.PendingCheck == nil || turn.PendingCheck.ProposedBy != "player" {
		t.Fatalf("expected a player pending check, got %+v", turn.PendingCheck)
	}
	if !strings.Contains(model.lastPrompt, "[PLAYER ROLL REQUESTED: 1d20+5") {
		t.Errorf("expected the roll request in the generation prompt, got %q", model.lastPrompt)
	}
}
