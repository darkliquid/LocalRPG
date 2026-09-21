package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS entities (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    frontmatter_json TEXT NOT NULL,
    body TEXT NOT NULL,
    file_hash TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS edges (
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    relation TEXT NOT NULL,
    PRIMARY KEY (source_id, target_id, relation)
);

CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);

CREATE TABLE IF NOT EXISTS turns (
    number       INTEGER PRIMARY KEY,
    timestamp    TEXT NOT NULL,
    mode         TEXT NOT NULL,
    input        TEXT NOT NULL,
    narration    TEXT NOT NULL,
    roll_json    TEXT,
    location     TEXT,
    outcome      TEXT
);

CREATE TABLE IF NOT EXISTS turn_entities (
    turn_number INTEGER NOT NULL,
    entity_id   TEXT NOT NULL,
    mention     TEXT NOT NULL,
    outcome     TEXT,
    PRIMARY KEY (turn_number, entity_id, mention)
);

CREATE INDEX IF NOT EXISTS idx_turn_entities_entity ON turn_entities(entity_id);
`

// pragmas are applied on every connection: WAL lets the GUI, the TUI, and a
// turn's extraction pass share one file, and the busy timeout turns lock
// contention into a short wait instead of an immediate failure.
const pragmas = "_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL"

func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?"+pragmas)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return db, nil
}
