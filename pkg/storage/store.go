package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type Edge struct {
	SourceID string
	TargetID string
	Relation string
}

type Store struct {
	db *sql.DB
}

func NewStore(path string) (*Store, error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveEntity(e *entity.Entity) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	fmMeta := map[string]interface{}{
		"tags":     e.Tags,
		"voice":    e.Voice,
		"portrait": e.Portrait,
		"location": e.Location,
		"faction":  e.Faction,
		"extra":    e.ExtraMeta,
	}
	if e.State != nil {
		fmMeta["state"] = e.State.Raw()
	}

	fmJSON, err := json.Marshal(fmMeta)
	if err != nil {
		return fmt.Errorf("marshal frontmatter: %w", err)
	}

	query := `
	INSERT INTO entities (id, name, type, frontmatter_json, body, file_hash, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		name = excluded.name,
		type = excluded.type,
		frontmatter_json = excluded.frontmatter_json,
		body = excluded.body,
		file_hash = excluded.file_hash,
		updated_at = CURRENT_TIMESTAMP
	`
	if _, err := tx.Exec(query, e.ID, e.Name, e.Type, string(fmJSON), e.Body, e.Hash); err != nil {
		return fmt.Errorf("upsert entity: %w", err)
	}

	// Recreate outgoing edges
	if _, err := tx.Exec(`DELETE FROM edges WHERE source_id = ?`, e.ID); err != nil {
		return fmt.Errorf("clear edges: %w", err)
	}

	for _, target := range e.Wikilinks {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO edges (source_id, target_id, relation) VALUES (?, ?, ?)`,
			e.ID, target, "references"); err != nil {
			return fmt.Errorf("insert edge: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetEntity(id string) (*entity.Entity, error) {
	query := `SELECT id, name, type, frontmatter_json, body, file_hash FROM entities WHERE id = ?`
	row := s.db.QueryRow(query, id)

	var ent entity.Entity
	var fmJSON string
	if err := row.Scan(&ent.ID, &ent.Name, &ent.Type, &fmJSON, &ent.Body, &ent.Hash); err != nil {
		return nil, err
	}

	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(fmJSON), &meta); err == nil {
		if stateData, ok := meta["state"].(map[string]interface{}); ok {
			ent.InitState(stateData)
		}
	}

	return &ent, nil
}

func (s *Store) GetEdgesFrom(sourceID string) ([]Edge, error) {
	rows, err := s.db.Query(`SELECT source_id, target_id, relation FROM edges WHERE source_id = ?`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.SourceID, &e.TargetID, &e.Relation); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, nil
}

func (s *Store) GetEdgesTo(targetID string) ([]Edge, error) {
	rows, err := s.db.Query(`SELECT source_id, target_id, relation FROM edges WHERE target_id = ?`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.SourceID, &e.TargetID, &e.Relation); err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	return edges, nil
}
