package storage

import (
	"database/sql"
	"fmt"
)

// migration is one ordered, idempotent step. Versions must be contiguous starting
// at 1: user_version records the highest applied version.
type migration struct {
	version int
	apply   func(*sql.DB) error
}

var migrations = []migration{
	{version: 1, apply: addTimelineColumns},
	{version: 2, apply: dropAudioRefsColumn},
	{version: 3, apply: addTurnContextsTable},
	{version: 4, apply: addWorkingSetTable},
	{version: 5, apply: addMemoriesTables},
	{version: 6, apply: addEmbeddingsTable},
	{version: 7, apply: addChecksColumn},
	{version: 8, apply: addUsageTable},
}

// addUsageTable records what each provider call cost, per campaign, so spend can
// be broken down by provider, role, and turn without re-deriving it.
func addUsageTable(db *sql.DB) error {
	const ddl = `
	CREATE TABLE IF NOT EXISTS usage_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		turn_number INTEGER NOT NULL DEFAULT 0,
		role TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL DEFAULT '',
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		characters INTEGER NOT NULL DEFAULT 0,
		requests INTEGER NOT NULL DEFAULT 0,
		estimated INTEGER NOT NULL DEFAULT 0,
		cost_micros INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_usage_turn ON usage_records(turn_number);
	CREATE INDEX IF NOT EXISTS idx_usage_provider ON usage_records(provider, role);`
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("create usage table: %w", err)
	}
	return nil
}

// addEmbeddingsTable creates the table and indexes for vector embeddings.
func addEmbeddingsTable(db *sql.DB) error {
	const ddl = `
	CREATE TABLE IF NOT EXISTS embeddings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		target_type TEXT NOT NULL,
		target_id TEXT NOT NULL,
		chunk_index INTEGER NOT NULL DEFAULT 0,
		content_hash TEXT NOT NULL,
		model_id TEXT NOT NULL,
		dimensions INTEGER NOT NULL,
		vector BLOB NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(target_type, target_id, chunk_index)
	);
	CREATE INDEX IF NOT EXISTS idx_embeddings_target ON embeddings(target_type, target_id);
	CREATE INDEX IF NOT EXISTS idx_embeddings_model_type ON embeddings(model_id, target_type);`
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("create embeddings table: %w", err)
	}
	return nil
}

// addMemoriesTables creates the per-entity memory store: the records, their
// entity links, their tags, and a full-text index over text and tags.
func addMemoriesTables(db *sql.DB) error {
	const ddl = `
	CREATE TABLE IF NOT EXISTS memories (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		turn        INTEGER NOT NULL,
		kind        TEXT NOT NULL,
		text        TEXT NOT NULL,
		importance  INTEGER NOT NULL DEFAULT 3,
		source      TEXT NOT NULL,
		check_id    TEXT,
		created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_memories_turn ON memories(turn);
	CREATE TABLE IF NOT EXISTS memory_entities (
		memory_id INTEGER NOT NULL,
		entity_id TEXT NOT NULL,
		PRIMARY KEY (memory_id, entity_id)
	);
	CREATE INDEX IF NOT EXISTS idx_memory_entities_entity ON memory_entities(entity_id);
	CREATE TABLE IF NOT EXISTS memory_tags (
		memory_id INTEGER NOT NULL,
		tag       TEXT NOT NULL,
		PRIMARY KEY (memory_id, tag)
	);
	CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(text, tags, tokenize='porter');`
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("create memories tables: %w", err)
	}
	return nil
}

func addTurnContextsTable(db *sql.DB) error {
	const create = `
	CREATE TABLE IF NOT EXISTS turn_contexts (
		turn_number INTEGER PRIMARY KEY,
		prompt      TEXT NOT NULL,
		context_json TEXT NOT NULL,
		created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(create); err != nil {
		return fmt.Errorf("create turn_contexts: %w", err)
	}
	return nil
}

func addWorkingSetTable(db *sql.DB) error {
	const create = `
	CREATE TABLE IF NOT EXISTS working_set (
		entity_id TEXT PRIMARY KEY,
		kind      TEXT NOT NULL,
		weight    REAL NOT NULL,
		last_turn INTEGER NOT NULL,
		role      TEXT
	);`
	if _, err := db.Exec(create); err != nil {
		return fmt.Errorf("create working_set: %w", err)
	}
	return nil
}

// migrate applies every migration a database has not seen. It runs on every open,
// after the baseline schema, because CREATE TABLE IF NOT EXISTS silently does
// nothing to a table that already exists: without this, a new column would never
// appear on an existing campaign database and the queries reading it would fail at
// runtime.
func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= version {
			continue
		}
		if err := m.apply(db); err != nil {
			return fmt.Errorf("migration %d: %w", m.version, err)
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
	}
	return nil
}

// addTimelineColumns brings the turn tables up to the shape the timeline records:
// where a turn happened and how its check resolved.
func addTimelineColumns(db *sql.DB) error {
	columns := []struct{ table, column, definition string }{
		{"turns", "location", "TEXT"},
		{"turns", "outcome", "TEXT"},
		{"turn_entities", "outcome", "TEXT"},
	}

	for _, c := range columns {
		exists, err := columnExists(db, c.table, c.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", c.table, c.column, c.definition)); err != nil {
			return fmt.Errorf("add %s.%s: %w", c.table, c.column, err)
		}
	}
	return nil
}

// addChecksColumn stores the checks a turn resolved so the index mirrors
// history.jsonl and a rebuilt database regains them.
func addChecksColumn(db *sql.DB) error {
	exists, err := columnExists(db, "turns", "checks_json")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE turns ADD COLUMN checks_json TEXT"); err != nil {
		return fmt.Errorf("add turns.checks_json: %w", err)
	}
	return nil
}

// dropAudioRefsColumn removes the column nothing has ever written. It is a
// separate step from the timeline columns because it has to land in the same change
// that removes its only reader, or GetTurn would select a column that is gone.
func dropAudioRefsColumn(db *sql.DB) error {
	exists, err := columnExists(db, "turns", "audio_refs_json")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE turns DROP COLUMN audio_refs_json"); err != nil {
		return fmt.Errorf("drop turns.audio_refs_json: %w", err)
	}
	return nil
}

// columnExists reports whether a table already has a column, which is what makes a
// partially applied migration converge instead of failing on the second run.
func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid          int
			name         string
			ctype        string
			notNull      int
			defaultValue interface{}
			pk           int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan %s columns: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
