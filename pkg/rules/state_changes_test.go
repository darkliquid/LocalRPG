package rules

import (
	"path/filepath"
	"strings"
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
	notes, err := ApplyStateChanges(bridge, []harness.StateChangeDecl{
		{Entity: "player", Path: "hp", Op: "sub", Value: 2, Reason: "fall"},
	}, map[string]core.StatSpec{"hp": {ID: "hp", Type: "number"}}, false)
	if err != nil {
		t.Fatalf("ApplyStateChanges: %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none", notes)
	}
	got, _ := bridge.GetStat("player", "hp")
	if value, _ := toInt(got); value != 3 {
		t.Fatalf("hp = %v, want 3", got)
	}
}

// A GM writes the name it used in prose, so a state change must resolve one to the
// entity the index holds rather than looking the words up as an ID.
func TestApplyStateChangesResolvesADisplayName(t *testing.T) {
	store := newRulesTestStore(t)
	if err := store.SaveEntity(&entity.Entity{
		ID: "ser-griswald-the-pallbearer", Name: "Ser Griswald the Pallbearer", Type: "character",
	}); err != nil {
		t.Fatal(err)
	}
	bridge := NewHostBridge(store, nil, "player")

	notes, err := ApplyStateChanges(bridge, []harness.StateChangeDecl{
		{Entity: "Ser Griswald the Pallbearer", Path: "mental_instability", Op: "add", Value: 2, Reason: "the writ"},
	}, nil, true)
	if err != nil {
		t.Fatalf("ApplyStateChanges: %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want the change applied", notes)
	}

	got, err := bridge.GetStat("ser-griswald-the-pallbearer", "mental_instability")
	if err != nil {
		t.Fatalf("GetStat: %v", err)
	}
	if value, _ := toInt(got); value != 2 {
		t.Fatalf("mental_instability = %v, want 2", got)
	}
}

// An entity the index does not hold is reported, not fatal, and the changes around
// it still land: one bad reference must not rewrite the turn's outcome.
func TestApplyStateChangesSkipsAnUnknownEntity(t *testing.T) {
	bridge := NewHostBridge(newRulesTestStore(t), nil, "player")

	notes, err := ApplyStateChanges(bridge, []harness.StateChangeDecl{
		{Entity: "Nobody At All", Path: "hp", Op: "sub", Value: 1},
		{Entity: "Sean", Path: "hp", Op: "set", Value: 7},
	}, nil, true)
	if err != nil {
		t.Fatalf("an unresolvable entity must not fail the batch: %v", err)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "Nobody At All") {
		t.Fatalf("notes = %v, want the unknown entity reported", notes)
	}

	got, err := bridge.GetStat("player", "hp")
	if err != nil {
		t.Fatalf("GetStat: %v", err)
	}
	if value, _ := toInt(got); value != 7 {
		t.Fatalf("hp = %v, want the neighbouring change to have applied", got)
	}
}

func TestApplyStateChangesRejectsUndeclared(t *testing.T) {
	bridge := NewHostBridge(newRulesTestStore(t), nil, "player")

	notes, err := ApplyStateChanges(bridge, []harness.StateChangeDecl{
		{Entity: "player", Path: "gold", Op: "set", Value: 100},
	}, map[string]core.StatSpec{"hp": {ID: "hp", Type: "number"}}, false)
	if err != nil {
		t.Fatalf("an undeclared stat is rejected, not fatal: %v", err)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "undeclared") {
		t.Fatalf("notes = %v, want the undeclared stat reported", notes)
	}

	got, err := bridge.GetStat("player", "gold")
	if err != nil {
		t.Fatalf("GetStat: %v", err)
	}
	if got != nil {
		t.Fatalf("gold = %v, want the change rejected", got)
	}
}

func TestApplyStateChangesWithoutABridgeFails(t *testing.T) {
	if _, err := ApplyStateChanges(nil, []harness.StateChangeDecl{
		{Entity: "player", Path: "hp", Op: "set", Value: 1},
	}, nil, true); err == nil {
		t.Fatal("expected an error when no bridge is wired")
	}
}
