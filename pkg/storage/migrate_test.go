package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// legacyDB builds a database with the pre-migration shape: turn tables without
// the location and outcome columns.
func legacyDB(t *testing.T, path string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+path+"?"+pragmas)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}

	const legacySchema = `
CREATE TABLE IF NOT EXISTS turns (
    number       INTEGER PRIMARY KEY,
    timestamp    TEXT NOT NULL,
    mode         TEXT NOT NULL,
    input        TEXT NOT NULL,
    narration    TEXT NOT NULL,
    roll_json    TEXT,
    audio_refs_json TEXT
);
CREATE TABLE IF NOT EXISTS turn_entities (
    turn_number INTEGER NOT NULL,
    entity_id   TEXT NOT NULL,
    mention     TEXT NOT NULL,
    PRIMARY KEY (turn_number, entity_id, mention)
);`
	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO turns (number, timestamp, mode, input, narration) VALUES (1, '2026-09-21T10:00:00Z', 'Do', 'look', 'You look around.')`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	return db
}

func TestMigrationsUpgradeALegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	legacy := legacyDB(t, path)
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore failed on a legacy database: %v", err)
	}
	defer store.Close()

	for _, column := range []string{"location", "outcome"} {
		exists, err := columnExists(store.db, "turns", column)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("expected turns.%s to exist after migration", column)
		}
	}

	exists, err := columnExists(store.db, "turn_entities", "outcome")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("expected turn_entities.outcome to exist after migration")
	}

	// Existing rows survive.
	rec, err := store.GetTurn(1)
	if err != nil {
		t.Fatalf("GetTurn on a migrated database failed: %v", err)
	}
	if rec.Narration != "You look around." {
		t.Errorf("Narration = %q after migration", rec.Narration)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	first, err := OpenDB(path)
	if err != nil {
		t.Fatalf("first OpenDB failed: %v", err)
	}
	var version int
	if err := first.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := OpenDB(path)
	if err != nil {
		t.Fatalf("second OpenDB failed: %v", err)
	}
	defer second.Close()

	var again int
	if err := second.QueryRow("PRAGMA user_version").Scan(&again); err != nil {
		t.Fatal(err)
	}
	if again != version {
		t.Errorf("user_version moved from %d to %d on reopen", version, again)
	}
	if version != len(migrations) {
		t.Errorf("user_version = %d, want %d (one per migration)", version, len(migrations))
	}
}

func TestMigrationRetiresTheVestigialAudioColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")

	legacy := legacyDB(t, path)
	if _, err := legacy.Exec(`INSERT INTO turns (number, timestamp, mode, input, narration, audio_refs_json) VALUES (2, '2026-09-21T11:00:00Z', 'Say', 'hello', 'She nods.', '["audio/x.wav"]')`); err != nil {
		t.Fatalf("seed legacy audio row: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore failed on a legacy database: %v", err)
	}
	defer store.Close()

	exists, err := columnExists(store.db, "turns", "audio_refs_json")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Errorf("expected turns.audio_refs_json to have been dropped")
	}

	// The row survives the column being dropped, and the new columns are usable.
	rec, err := store.GetTurn(2)
	if err != nil {
		t.Fatalf("GetTurn after the drop failed: %v", err)
	}
	if rec.Narration != "She nods." {
		t.Errorf("Narration = %q", rec.Narration)
	}
	if rec.Location != "" || rec.Outcome != "" {
		t.Errorf("expected empty new columns, got %q / %q", rec.Location, rec.Outcome)
	}

	if err := store.SaveTurn(TurnRecord{
		Number: 2, Timestamp: time.Now(), Mode: "Say", Input: "hello", Narration: "She nods.",
		Location: "alden-tavern", Outcome: "kind_word",
	}); err != nil {
		t.Fatalf("SaveTurn after the migration failed: %v", err)
	}
}

func TestAddEntityFolderColumnIsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "index.db")+"?"+pragmas)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE entities (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create entities: %v", err)
	}

	if err := addEntityFolderColumn(db); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := addEntityFolderColumn(db); err != nil {
		t.Fatalf("second apply must be a no-op, got: %v", err)
	}

	exists, err := columnExists(db, "entities", "folder")
	if err != nil {
		t.Fatalf("columnExists: %v", err)
	}
	if !exists {
		t.Fatal("entities.folder missing after the migration")
	}
}
