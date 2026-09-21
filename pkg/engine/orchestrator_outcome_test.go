package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestProcessActionRecordsTheSystemsOutcomeLabel(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t)
	timeline := NewTimeline(core.NewPathResolver(tempDir), store, NewHistoryLogger(filepath.Join(tempDir, "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Body: "Salt air."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller.", Location: "[[aldon-harbour]]"})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	jsEngine := rules.NewJSEngine(rules.NewHostBridge(store, timeline, "player"))
	if err := jsEngine.LoadScript(`
		onAction("attack", (ctx) => ({ success: false, outcome: "glancing_blow", message: "A glancing blow." }));
	`); err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	router := harness.NewRouter()
	router.RegisterProvider(&mockOrchestratorModel{response: "Steel skitters off the wall."})
	router.AssignRole("gm", "mock-gm")

	o := NewTurnOrchestrator(store, timeline, jsEngine, router, "aldon-harbour", "player")

	turn, err := o.ProcessAction(context.Background(), "Attack", "I swing at the cultist")
	if err != nil {
		t.Fatalf("ProcessAction failed: %v", err)
	}
	if turn.Outcome != "glancing_blow" {
		t.Fatalf("Outcome = %q, want glancing_blow", turn.Outcome)
	}

	// The label reaches the index both on the turn and on every link.
	rec, err := store.GetTurn(turn.Number)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Outcome != "glancing_blow" {
		t.Errorf("indexed Outcome = %q", rec.Outcome)
	}

	turns, err := store.ListTurnEntitiesByOutcome("player", "glancing_blow")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0] != turn.Number {
		t.Errorf("expected the player's link to carry the outcome, got %v", turns)
	}
}
