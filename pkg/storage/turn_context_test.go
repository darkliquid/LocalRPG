package storage

import (
	"bytes"
	"testing"
)

func TestTurnContextRoundTrip(t *testing.T) {
	store := openTestDB(t)
	raw := []byte(`{"turn_number":1,"prompt_hash":"abc","strategy":"full_prompt"}`)
	if err := store.SaveTurnContext(1, "the prompt", raw); err != nil {
		t.Fatalf("SaveTurnContext: %v", err)
	}
	got, prompt, err := store.GetTurnContext(1)
	if err != nil {
		t.Fatalf("GetTurnContext: %v", err)
	}
	if prompt != "the prompt" || !bytes.Equal(got, raw) {
		t.Fatalf("round trip mismatch: %q %s", prompt, string(got))
	}
}

func TestWorkingSetRoundTrip(t *testing.T) {
	store := openTestDB(t)
	entries := []WorkingSetRecord{
		{EntityID: "elena", Kind: "entity", Weight: 2.5, LastTurn: 5, Role: "action"},
		{EntityID: "kaelen", Kind: "entity", Weight: 1.2, LastTurn: 3, Role: "present"},
	}
	if err := store.ReplaceWorkingSet(entries); err != nil {
		t.Fatalf("ReplaceWorkingSet: %v", err)
	}

	loaded, err := store.LoadWorkingSet()
	if err != nil {
		t.Fatalf("LoadWorkingSet: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded entries, got %d", len(loaded))
	}
	if loaded[0].EntityID != "elena" || loaded[0].Weight != 2.5 {
		t.Errorf("unexpected top entry: %+v", loaded[0])
	}
	if loaded[1].EntityID != "kaelen" || loaded[1].Weight != 1.2 {
		t.Errorf("unexpected second entry: %+v", loaded[1])
	}

	// Test replacement clears prior entries
	newEntries := []WorkingSetRecord{
		{EntityID: "seraphine", Kind: "entity", Weight: 1.0, LastTurn: 6, Role: "present"},
	}
	if err := store.ReplaceWorkingSet(newEntries); err != nil {
		t.Fatalf("second ReplaceWorkingSet: %v", err)
	}
	loaded, err = store.LoadWorkingSet()
	if err != nil {
		t.Fatalf("LoadWorkingSet: %v", err)
	}
	if len(loaded) != 1 || loaded[0].EntityID != "seraphine" {
		t.Fatalf("expected only seraphine, got %+v", loaded)
	}
}

func TestDeleteTurnsCleansTurnContexts(t *testing.T) {
	store := openTestDB(t)
	raw := []byte(`{"turn_number":1,"prompt_hash":"abc"}`)
	if err := store.SaveTurnContext(1, "prompt 1", raw); err != nil {
		t.Fatalf("SaveTurnContext: %v", err)
	}
	if err := store.SaveTurnContext(2, "prompt 2", raw); err != nil {
		t.Fatalf("SaveTurnContext: %v", err)
	}

	if err := store.DeleteTurnsFrom(2); err != nil {
		t.Fatalf("DeleteTurnsFrom: %v", err)
	}

	if _, _, err := store.GetTurnContext(1); err != nil {
		t.Errorf("turn 1 context should still exist: %v", err)
	}
	if _, _, err := store.GetTurnContext(2); err == nil {
		t.Errorf("turn 2 context should have been deleted")
	}
}

