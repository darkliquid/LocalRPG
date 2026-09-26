package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTurnsChecksJSONRoundTrip(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	exists, err := columnExists(store.db, "turns", "checks_json")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected turns.checks_json after migration")
	}

	rec := TurnRecord{
		Number: 1, Timestamp: time.Now().UTC(), Mode: "Do", Input: "look",
		Narration: "You look.", ChecksJSON: `[{"check_id":"chk_1","outcome":"pass"}]`,
	}
	if err := store.SaveTurn(rec); err != nil {
		t.Fatalf("SaveTurn: %v", err)
	}
	got, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if got.ChecksJSON != rec.ChecksJSON {
		t.Fatalf("ChecksJSON = %q, want %q", got.ChecksJSON, rec.ChecksJSON)
	}
}
