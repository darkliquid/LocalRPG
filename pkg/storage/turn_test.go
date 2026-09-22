package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadTurn(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	rec := TurnRecord{
		Number:    1,
		Timestamp: time.Date(2026, 9, 21, 14, 3, 11, 0, time.UTC),
		Mode:      "Do",
		Input:     "I search the harbour",
		Narration: "The docks reek of brine.",
		RollJSON:  `{"notation":"1d20","total":15}`,
		Location:  "aldon-harbour",
		Outcome:   "clean_look",
		Entities: []TurnEntityRef{
			{EntityID: "hero", Mention: "player"},
			{EntityID: "aldon-harbour", Mention: "location"},
		},
	}

	if err := store.SaveTurn(rec); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}

	loaded, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn failed: %v", err)
	}
	if loaded.Narration != rec.Narration || loaded.Input != rec.Input || loaded.Mode != rec.Mode {
		t.Errorf("loaded turn mismatch: %+v", loaded)
	}
	if !loaded.Timestamp.Equal(rec.Timestamp) {
		t.Errorf("timestamp = %v, want %v", loaded.Timestamp, rec.Timestamp)
	}
	if loaded.RollJSON != rec.RollJSON {
		t.Errorf("json columns mismatch: %+v", loaded)
	}
	if len(loaded.Entities) != 2 {
		t.Fatalf("expected 2 entity links, got %+v", loaded.Entities)
	}

	// Re-saving replaces links rather than accumulating them.
	rec.Entities = []TurnEntityRef{{EntityID: "hero", Mention: "player"}}
	if err := store.SaveTurn(rec); err != nil {
		t.Fatalf("second SaveTurn failed: %v", err)
	}
	reloaded, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn failed: %v", err)
	}
	if len(reloaded.Entities) != 1 || reloaded.Entities[0].EntityID != "hero" {
		t.Errorf("expected links to be replaced, got %+v", reloaded.Entities)
	}
}

func TestGetTurnMissing(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	if _, err := store.GetTurn(7); err == nil {
		t.Errorf("expected an error for a missing turn")
	}
}

func TestTurnQueries(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	for n := 1; n <= 3; n++ {
		rec := TurnRecord{
			Number:    n,
			Timestamp: time.Date(2026, 9, 21, 10, n, 0, 0, time.UTC),
			Mode:      "Do",
			Input:     "input",
			Narration: "narration",
			Entities: []TurnEntityRef{
				{EntityID: "hero", Mention: "player"},
				{EntityID: "garrick", Mention: "extracted"},
			},
		}
		if n == 3 {
			rec.Entities = []TurnEntityRef{{EntityID: "hero", Mention: "player"}}
		}
		if err := store.SaveTurn(rec); err != nil {
			t.Fatalf("SaveTurn(%d) failed: %v", n, err)
		}
	}

	turns, err := store.ListTurns(0, 0)
	if err != nil {
		t.Fatalf("ListTurns failed: %v", err)
	}
	if len(turns) != 3 || turns[0].Number != 1 || turns[2].Number != 3 {
		t.Fatalf("expected turns 1..3 ascending, got %+v", turns)
	}

	limited, err := store.ListTurns(2, 1)
	if err != nil {
		t.Fatalf("ListTurns failed: %v", err)
	}
	if len(limited) != 2 || limited[0].Number != 2 {
		t.Errorf("expected turns 2..3, got %+v", limited)
	}

	heroTurns, err := store.ListTurnsForEntity("hero")
	if err != nil {
		t.Fatalf("ListTurnsForEntity failed: %v", err)
	}
	if len(heroTurns) != 3 || heroTurns[0] != 1 || heroTurns[2] != 3 {
		t.Errorf("expected hero in turns 1,2,3, got %v", heroTurns)
	}

	garrickTurns, err := store.ListTurnsForEntity("garrick")
	if err != nil {
		t.Fatalf("ListTurnsForEntity failed: %v", err)
	}
	if len(garrickTurns) != 2 {
		t.Errorf("expected garrick in turns 1,2, got %v", garrickTurns)
	}

	if max, err := store.MaxTurnNumber(); err != nil || max != 3 {
		t.Errorf("MaxTurnNumber = %d, %v; want 3", max, err)
	}
	if count, err := store.CountTurns(); err != nil || count != 3 {
		t.Errorf("CountTurns = %d, %v; want 3", count, err)
	}
}

func TestTurnQueriesOnEmptyStore(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	max, err := store.MaxTurnNumber()
	if err != nil {
		t.Fatalf("MaxTurnNumber failed: %v", err)
	}
	if max != 0 {
		t.Errorf("MaxTurnNumber = %d, want 0", max)
	}
}

func TestDeleteTurnsFrom(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	for n := 1; n <= 3; n++ {
		rec := TurnRecord{
			Number:    n,
			Timestamp: time.Date(2026, 9, 21, 10, n, 0, 0, time.UTC),
			Mode:      "Do",
			Input:     "input",
			Narration: "narration",
			Entities:  []TurnEntityRef{{EntityID: "hero", Mention: "player"}},
		}
		if err := store.SaveTurn(rec); err != nil {
			t.Fatalf("SaveTurn(%d) failed: %v", n, err)
		}
	}

	if err := store.DeleteTurnsFrom(3); err != nil {
		t.Fatalf("DeleteTurnsFrom failed: %v", err)
	}

	count, err := store.CountTurns()
	if err != nil {
		t.Fatalf("CountTurns failed: %v", err)
	}
	if count != 2 {
		t.Errorf("CountTurns = %d, want 2", count)
	}

	heroTurns, err := store.ListTurnsForEntity("hero")
	if err != nil {
		t.Fatalf("ListTurnsForEntity failed: %v", err)
	}
	if len(heroTurns) != 2 {
		t.Errorf("expected links for turns 1,2 only, got %v", heroTurns)
	}

	// Deleting beyond the highest turn is a no-op, not an error.
	if err := store.DeleteTurnsFrom(99); err != nil {
		t.Fatalf("DeleteTurnsFrom(99) failed: %v", err)
	}
}

func TestDeleteTurnsFromMissingRowsIsSafe(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	if err := store.DeleteTurnsFrom(1); err != nil {
		t.Fatalf("DeleteTurnsFrom on an empty store failed: %v", err)
	}
}

func TestTurnRecordCarriesLocationAndOutcome(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	rec := TurnRecord{
		Number:    1,
		Timestamp: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC),
		Mode:      "Do",
		Input:     "I swing at the cultist",
		Narration: "Steel rings.",
		Location:  "alden-tavern",
		Outcome:   "glancing_blow",
		Entities: []TurnEntityRef{
			{EntityID: "player", Mention: "player", Outcome: "glancing_blow"},
			{EntityID: "alden-tavern", Mention: "location", Outcome: "glancing_blow"},
		},
	}

	if err := store.SaveTurn(rec); err != nil {
		t.Fatalf("SaveTurn failed: %v", err)
	}

	loaded, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn failed: %v", err)
	}
	if loaded.Location != "alden-tavern" {
		t.Errorf("Location = %q, want alden-tavern", loaded.Location)
	}
	if loaded.Outcome != "glancing_blow" {
		t.Errorf("Outcome = %q, want glancing_blow", loaded.Outcome)
	}
	if len(loaded.Entities) != 2 {
		t.Fatalf("expected 2 links, got %+v", loaded.Entities)
	}

	// The per-entity copy is what makes an outcome query single-table.
	outcomes, err := store.ListTurnEntitiesByOutcome("player", "glancing_blow")
	if err != nil {
		t.Fatalf("ListTurnEntitiesByOutcome failed: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0] != 1 {
		t.Errorf("expected turn 1 for a glancing_blow involving player, got %v", outcomes)
	}

	none, err := store.ListTurnEntitiesByOutcome("player", "clean_hit")
	if err != nil {
		t.Fatalf("ListTurnEntitiesByOutcome failed: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected no turns for a clean_hit, got %v", none)
	}
}

func newTestTurnStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seedTurns(t *testing.T, store *Store, turns ...TurnRecord) {
	t.Helper()
	for _, turn := range turns {
		if turn.Timestamp.IsZero() {
			turn.Timestamp = time.Date(2026, 9, 22, 12, 0, turn.Number, 0, time.UTC)
		}
		if err := store.SaveTurn(turn); err != nil {
			t.Fatalf("save turn %d: %v", turn.Number, err)
		}
	}
}

func TestTurnsAtLocationReturnsTheNewestFirstWithinTheLimit(t *testing.T) {
	store := newTestTurnStore(t)
	seedTurns(t, store,
		TurnRecord{Number: 1, Mode: "Do", Input: "a", Narration: "at the harbour", Location: "aldon-harbour"},
		TurnRecord{Number: 2, Mode: "Do", Input: "b", Narration: "at the tavern", Location: "oakhaven-tavern"},
		TurnRecord{Number: 3, Mode: "Do", Input: "c", Narration: "back at the harbour", Location: "aldon-harbour"},
		TurnRecord{Number: 4, Mode: "Do", Input: "d", Narration: "later at the harbour", Location: "aldon-harbour"},
	)

	turns, err := store.TurnsAtLocation("aldon-harbour", 4, 5)
	if err != nil {
		t.Fatalf("TurnsAtLocation failed: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns before turn 4, got %d", len(turns))
	}
	// Oldest first, so the caller can render them in order.
	if turns[0].Number != 1 || turns[1].Number != 3 {
		t.Errorf("expected turns 1 then 3, got %d then %d", turns[0].Number, turns[1].Number)
	}
	if turns[1].Narration != "back at the harbour" {
		t.Errorf("narration did not survive the query: %q", turns[1].Narration)
	}
}

func TestTurnsMentioningEntitiesRanksByOverlapThenRecency(t *testing.T) {
	store := newTestTurnStore(t)
	seedTurns(t, store,
		TurnRecord{Number: 1, Mode: "Do", Narration: "one", Entities: []TurnEntityRef{{EntityID: "kael", Mention: "wikilink"}}},
		TurnRecord{Number: 2, Mode: "Do", Narration: "two", Entities: []TurnEntityRef{
			{EntityID: "kael", Mention: "wikilink"}, {EntityID: "miasma", Mention: "wikilink"},
		}},
		TurnRecord{Number: 3, Mode: "Do", Narration: "three", Entities: []TurnEntityRef{{EntityID: "kael", Mention: "wikilink"}}},
	)

	turns, err := store.TurnsMentioningEntities([]string{"kael", "miasma"}, 4, 2)
	if err != nil {
		t.Fatalf("TurnsMentioningEntities failed: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	// The turn mentioning both entities outranks the newer one mentioning only one.
	if turns[0].Number != 2 {
		t.Errorf("expected the two-entity turn to rank first, got turn %d", turns[0].Number)
	}
	if turns[1].Number != 3 {
		t.Errorf("expected the newer single-entity turn second, got turn %d", turns[1].Number)
	}
}

func TestEntitiesInTurnsReturnsDistinctIDs(t *testing.T) {
	store := newTestTurnStore(t)
	seedTurns(t, store,
		TurnRecord{Number: 1, Mode: "Do", Narration: "one", Entities: []TurnEntityRef{
			{EntityID: "kael", Mention: "wikilink"}, {EntityID: "miasma", Mention: "extracted"},
		}},
		TurnRecord{Number: 2, Mode: "Do", Narration: "two", Entities: []TurnEntityRef{{EntityID: "kael", Mention: "speech"}}},
	)

	ids, err := store.EntitiesInTurns([]int{1, 2})
	if err != nil {
		t.Fatalf("EntitiesInTurns failed: %v", err)
	}
	if len(ids) != 2 || ids[0] != "kael" || ids[1] != "miasma" {
		t.Errorf("expected kael and miasma once each, got %v", ids)
	}

	if empty, err := store.EntitiesInTurns(nil); err != nil || len(empty) != 0 {
		t.Errorf("expected no ids for no turns, got %v, %v", empty, err)
	}
}
