package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func advancementOrchestrator(t *testing.T, spec *core.AdvancementSpec) (*TurnOrchestrator, *storage.Store) {
	t.Helper()

	tempDir := t.TempDir()
	store := newTestStore(t)
	paths := core.NewPathResolver(tempDir)
	if err := os.MkdirAll(paths.GameDir("campaign-01"), 0755); err != nil {
		t.Fatal(err)
	}
	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(paths.GameDir("campaign-01"), "history.jsonl")), "campaign-01")

	entitiesDir := timeline.EntitiesDir()
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "tavern", Name: "Alden Tavern", Type: "location", Body: "Cozy."})
	writeTestEntityNote(t, entitiesDir, &entity.Entity{ID: "player", Name: "Sean", Type: "character", Body: "A traveller.", Location: "[[tavern]]"})
	if _, err := storage.NewSyncer(store).Sync(entitiesDir); err != nil {
		t.Fatal(err)
	}

	jsEngine := rules.NewJSEngine(rules.NewHostBridge(store, nil, "player"))
	orchestrator := NewTurnOrchestrator(store, timeline, jsEngine, harness.NewRouter(), "tavern", "player")
	orchestrator.SetMechanics(&core.MechanicsSpec{Advancement: spec})
	return orchestrator, store
}

func stateInt(t *testing.T, store *storage.Store, id, path string) int {
	t.Helper()
	ent, err := store.GetEntity(id)
	if err != nil {
		t.Fatalf("GetEntity(%q): %v", id, err)
	}
	if ent.State == nil {
		return 0
	}
	value, ok := ent.State.Get(path)
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	}
	t.Fatalf("state %s.%s = %v, not a number", id, path, value)
	return 0
}

func TestApplyAdvancementAwardsOnMiss(t *testing.T) {
	orchestrator, store := advancementOrchestrator(t, &core.AdvancementSpec{
		Currency: core.CurrencySpec{Stat: "xp", Label: "Experience"},
		Earn:     []core.EarnRule{{On: "miss", Amount: 1}},
	})

	turn := &Turn{Number: 1, Checks: []harness.CheckResult{{CheckID: "c1", Actor: "player", Outcome: "miss"}}}
	orchestrator.applyAdvancement(context.Background(), turn)

	if got := stateInt(t, store, "player", "xp"); got != 1 {
		t.Fatalf("xp = %d, want 1", got)
	}
	found := false
	for _, memory := range turn.Memories {
		if memory.Kind == "advancement" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an advancement memory, got %+v", turn.Memories)
	}
}

func TestApplyAdvancementThresholdAppliesLevelOnce(t *testing.T) {
	orchestrator, store := advancementOrchestrator(t, &core.AdvancementSpec{
		Currency: core.CurrencySpec{Stat: "xp"},
		Mode:     "threshold",
		Earn:     []core.EarnRule{{On: "turn_end", Amount: 10}},
		Levels: []core.LevelSpec{{
			At:      10,
			Label:   "Level 2",
			Effects: []core.EffectSpec{{Type: "stat_increase", Stat: "might", Amount: 1}},
		}},
	})

	orchestrator.applyAdvancement(context.Background(), &Turn{Number: 1})
	if got := stateInt(t, store, "player", "xp"); got != 10 {
		t.Fatalf("xp = %d, want 10", got)
	}
	if got := stateInt(t, store, "player", "might"); got != 1 {
		t.Fatalf("might = %d, want 1", got)
	}

	orchestrator.applyAdvancement(context.Background(), &Turn{Number: 2})
	if got := stateInt(t, store, "player", "xp"); got != 20 {
		t.Fatalf("xp = %d, want 20", got)
	}
	if got := stateInt(t, store, "player", "might"); got != 1 {
		t.Fatalf("a crossed level must not re-apply; might = %d, want 1", got)
	}
}
