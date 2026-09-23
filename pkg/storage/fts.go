package storage

import (
	"database/sql"
	"fmt"
)

// ftsSchema creates the derived search tables and the triggers that keep them in
// step with the content tables. Tags are read out of the entity frontmatter JSON,
// which is why these are plain FTS5 tables keyed by rowid rather than
// external-content tables: an external-content table can only index columns that
// exist on its content table. The tokenizer is porter, so model-written prose
// matches on inflection.
const ftsSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS entities_fts USING fts5(
    name, body, tags, tokenize='porter'
);

CREATE VIRTUAL TABLE IF NOT EXISTS turns_fts USING fts5(
    input, narration, tokenize='porter'
);

CREATE TRIGGER IF NOT EXISTS entities_fts_insert AFTER INSERT ON entities BEGIN
    INSERT INTO entities_fts(rowid, name, body, tags)
    VALUES (new.rowid, new.name, new.body, coalesce(json_extract(new.frontmatter_json, '$.tags'), ''));
END;

CREATE TRIGGER IF NOT EXISTS entities_fts_update AFTER UPDATE ON entities BEGIN
    DELETE FROM entities_fts WHERE rowid = old.rowid;
    INSERT INTO entities_fts(rowid, name, body, tags)
    VALUES (new.rowid, new.name, new.body, coalesce(json_extract(new.frontmatter_json, '$.tags'), ''));
END;

CREATE TRIGGER IF NOT EXISTS entities_fts_delete AFTER DELETE ON entities BEGIN
    DELETE FROM entities_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS turns_fts_insert AFTER INSERT ON turns BEGIN
    INSERT INTO turns_fts(rowid, input, narration)
    VALUES (new.rowid, new.input, new.narration);
END;

CREATE TRIGGER IF NOT EXISTS turns_fts_update AFTER UPDATE ON turns BEGIN
    DELETE FROM turns_fts WHERE rowid = old.rowid;
    INSERT INTO turns_fts(rowid, input, narration)
    VALUES (new.rowid, new.input, new.narration);
END;

CREATE TRIGGER IF NOT EXISTS turns_fts_delete AFTER DELETE ON turns BEGIN
    DELETE FROM turns_fts WHERE rowid = old.rowid;
END;
`

// EnsureFTS makes the search tables agree with the content tables. It is
// idempotent and runs on every open: rows the index is missing are inserted from
// the tables, and rows whose content is gone are removed, so an index written
// before FTS5 existed is repaired without re-reading any Markdown.
func EnsureFTS(db *sql.DB) error {
	if _, err := db.Exec(ftsSchema); err != nil {
		return fmt.Errorf("apply fts schema: %w", err)
	}

	backfill := []string{
		`INSERT INTO entities_fts(rowid, name, body, tags)
		 SELECT e.rowid, e.name, e.body, coalesce(json_extract(e.frontmatter_json, '$.tags'), '')
		 FROM entities e
		 WHERE e.rowid NOT IN (SELECT rowid FROM entities_fts)`,
		`DELETE FROM entities_fts
		 WHERE rowid NOT IN (SELECT rowid FROM entities)`,
		`INSERT INTO turns_fts(rowid, input, narration)
		 SELECT t.rowid, t.input, t.narration
		 FROM turns t
		 WHERE t.rowid NOT IN (SELECT rowid FROM turns_fts)`,
		`DELETE FROM turns_fts
		 WHERE rowid NOT IN (SELECT rowid FROM turns)`,
	}
	for _, statement := range backfill {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("backfill fts: %w", err)
		}
	}
	return nil
}

// SearchEntityHit is one entity match, with a snippet that shows why it matched.
type SearchEntityHit struct {
	ID      string
	Name    string
	Type    string
	Snippet string
}

// SearchTurnHit is one turn match, with a narration snippet.
type SearchTurnHit struct {
	Number  int
	Snippet string
}

// SearchEntities runs an FTS5 MATCH expression. The expression is built by the
// caller, because building it from a model's words is a tool concern.
func (s *Store) SearchEntities(match, entityType string, limit int) ([]SearchEntityHit, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT e.id, e.name, e.type, snippet(entities_fts, 1, '[', ']', '...', 12)
		FROM entities_fts
		JOIN entities e ON e.rowid = entities_fts.rowid
		WHERE entities_fts MATCH ?
		  AND (? = '' OR e.type = ?)
		ORDER BY bm25(entities_fts)
		LIMIT ?`, match, entityType, entityType, limit)
	if err != nil {
		return nil, fmt.Errorf("search entities: %w", err)
	}
	defer rows.Close()

	hits := make([]SearchEntityHit, 0)
	for rows.Next() {
		var hit SearchEntityHit
		if err := rows.Scan(&hit.ID, &hit.Name, &hit.Type, &hit.Snippet); err != nil {
			return nil, fmt.Errorf("scan entity hit: %w", err)
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

// SearchTurns runs an FTS5 MATCH expression over turn prose, optionally limited
// to turns that mention one entity.
func (s *Store) SearchTurns(match, entityID string, limit int) ([]SearchTurnHit, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT t.number, snippet(turns_fts, 1, '[', ']', '...', 12)
		FROM turns_fts
		JOIN turns t ON t.rowid = turns_fts.rowid
		WHERE turns_fts MATCH ?
		  AND (? = '' OR EXISTS (
		      SELECT 1 FROM turn_entities te
		      WHERE te.turn_number = t.number AND te.entity_id = ?
		  ))
		ORDER BY bm25(turns_fts)
		LIMIT ?`, match, entityID, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("search turns: %w", err)
	}
	defer rows.Close()

	hits := make([]SearchTurnHit, 0)
	for rows.Next() {
		var hit SearchTurnHit
		if err := rows.Scan(&hit.Number, &hit.Snippet); err != nil {
			return nil, fmt.Errorf("scan turn hit: %w", err)
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}
