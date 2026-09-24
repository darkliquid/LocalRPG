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
