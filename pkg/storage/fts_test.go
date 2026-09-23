package storage

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestFTSTracksEntityWrites(t *testing.T) {
	store := openTestDB(t)

	warden := &entity.Entity{ID: "warden", Name: "The Warden", Type: "character", Body: "A grim warden of the eastern gate.", Tags: []string{"guard"}}
	if err := store.SaveEntity(warden); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}

	hits, err := store.SearchEntities("warden", "", 10)
	if err != nil {
		t.Fatalf("SearchEntities: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "warden" {
		t.Fatalf("hits = %+v, want the warden", hits)
	}

	// Porter stemming makes an inflected query find the same note.
	if hits, err := store.SearchEntities("wardens", "", 10); err != nil || len(hits) != 1 {
		t.Errorf("stemmed query hits = %+v, err = %v", hits, err)
	}

	// An update is reflected, not duplicated.
	warden.Body = "A kindly warden of the western gate."
	if err := store.SaveEntity(warden); err != nil {
		t.Fatalf("SaveEntity update: %v", err)
	}
	if hits, err := store.SearchEntities("kindly", "", 10); err != nil || len(hits) != 1 {
		t.Errorf("after update hits = %+v, err = %v", hits, err)
	}
	if hits, _ := store.SearchEntities("grim", "", 10); len(hits) != 0 {
		t.Errorf("the old body should no longer match, got %+v", hits)
	}

	if err := store.DeleteEntity("warden"); err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}
	if hits, _ := store.SearchEntities("warden", "", 10); len(hits) != 0 {
		t.Errorf("a deleted entity must not be searchable, got %+v", hits)
	}
}

func TestFSTSearchesTurnProse(t *testing.T) {
	store := openTestDB(t)
	if err := store.SaveTurn(TurnRecord{Number: 1, Mode: "Do", Input: "I look around", Narration: "The guttered lanterns flicker."}); err != nil {
		t.Fatalf("SaveTurn: %v", err)
	}

	hits, err := store.SearchTurns("guttering", "", 10)
	if err != nil {
		t.Fatalf("SearchTurns: %v", err)
	}
	if len(hits) != 1 || hits[0].Number != 1 {
		t.Fatalf("hits = %+v, want turn 1", hits)
	}
}

func TestEnsureFTSBackfillsAnExistingIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	// A database written before FTS5 existed: schema only, no virtual tables and
	// no triggers. The triggers must go too, or an insert would fail on the
	// missing table rather than being repaired on the next open.
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	for _, statement := range []string{
		`DROP TABLE IF EXISTS entities_fts`,
		`DROP TABLE IF EXISTS turns_fts`,
		`DROP TRIGGER IF EXISTS entities_fts_insert`,
		`DROP TRIGGER IF EXISTS entities_fts_update`,
		`DROP TRIGGER IF EXISTS entities_fts_delete`,
		`DROP TRIGGER IF EXISTS turns_fts_insert`,
		`DROP TRIGGER IF EXISTS turns_fts_update`,
		`DROP TRIGGER IF EXISTS turns_fts_delete`,
	} {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if err := store.SaveEntity(&entity.Entity{ID: "old", Name: "Old Note", Type: "lore", Body: "An ancient bridge."}); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}
	_ = store.Close()

	// Reopening must restore search without rescanning Markdown.
	reopened, err := NewStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	hits, err := reopened.SearchEntities("ancient", "", 10)
	if err != nil {
		t.Fatalf("SearchEntities after backfill: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "old" {
		t.Errorf("hits = %+v, want the backfilled note", hits)
	}
}
