package rules

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func newRulesTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.SaveEntity(&entity.Entity{ID: "player", Name: "Sean", Type: "character"}); err != nil {
		t.Fatalf("seed player: %v", err)
	}
	return store
}

func TestApplyStateChanges(t *testing.T) {
	store := newRulesTestStore(t)
	bridge := NewHostBridge(store, nil, "player")
	if err := bridge.SetStat("player", "hp", 5); err != nil {
		t.Fatal(err)
	}
	err := ApplyStateChanges(bridge, []harness.StateChangeDecl{
		{Entity: "player", Path: "hp", Op: "sub", Value: 2, Reason: "fall"},
	}, map[string]core.StatSpec{"hp": {ID: "hp", Type: "number"}}, false)
	if err != nil {
		t.Fatalf("ApplyStateChanges: %v", err)
	}
	got, _ := bridge.GetStat("player", "hp")
	if value, _ := toInt(got); value != 3 {
		t.Fatalf("hp = %v, want 3", got)
	}
}

func TestApplyStateChangesRejectsUndeclared(t *testing.T) {
	bridge := NewHostBridge(newRulesTestStore(t), nil, "player")
	err := ApplyStateChanges(bridge, []harness.StateChangeDecl{
		{Entity: "player", Path: "gold", Op: "set", Value: 100},
	}, map[string]core.StatSpec{"hp": {ID: "hp", Type: "number"}}, false)
	if err == nil {
		t.Fatal("undeclared stat should be rejected when a schema declares stats")
	}
}
