package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestPlayerRollRequestUsesTheConventionsNotation(t *testing.T) {
	o := &TurnOrchestrator{playerID: "player", mechanics: &core.MechanicsSpec{
		Checks: core.CheckConventions{Notation: "2d6"},
	}}
	req := o.playerRollRequest("pick the lock")
	if req.Notation != "2d6" || req.Actor != "player" {
		t.Fatalf("request = %+v", req)
	}
	if req.Stakes != "pick the lock" {
		t.Fatalf("stakes = %q", req.Stakes)
	}
}

func TestRollModeUnderOffProducesNoCheck(t *testing.T) {
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

	o := NewTurnOrchestrator(store, timeline, nil, router, "tavern", "player")
	o.SetMechanicsEngagement("off")

	turn, err := o.ProcessAction(context.Background(), "Roll", "pick the lock")
	if err != nil {
		t.Fatal(err)
	}
	if turn.PendingCheck != nil {
		t.Fatalf("off should not pend, got %+v", turn.PendingCheck)
	}
}
