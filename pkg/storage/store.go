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
	db     *sql.DB
	shared bool
}

func NewStore(path string) (*Store, error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the underlying handle. A store handed out by a Pool is shared,
// so its lifetime belongs to the pool and Close is a no-op.
func (s *Store) Close() error {
	if s.shared {
		return nil
	}
	return s.db.Close()
}

func (s *Store) SaveEntity(e *entity.Entity) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	fmMeta := map[string]interface{}{
		"tags":             e.Tags,
		"voice":            e.Voice,
		"portrait":         e.Portrait,
		"portrait_version": e.PortraitVersion,
		"portrait_history": e.PortraitHistory,
		"location":         e.Location,
		"faction":          e.Faction,
		"appearance": e.Appearance,
		"age":        e.Age,
		"gender":     e.Gender,
		"aliases":    e.Aliases,
		"history":    e.History,
		"extra":      e.ExtraMeta,
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
		if voiceData, ok := meta["voice"]; ok && voiceData != nil {
			voiceBytes, _ := json.Marshal(voiceData)
			var v entity.VoiceConfig
			if err := json.Unmarshal(voiceBytes, &v); err == nil {
				ent.Voice = &v
			}
		}
		if loc, ok := meta["location"].(string); ok {
			ent.Location = loc
		}
		if fac, ok := meta["faction"].(string); ok {
			ent.Faction = fac
		}
		if appearance, ok := meta["appearance"].(string); ok {
			ent.Appearance = appearance
		}
		if age, ok := meta["age"].(string); ok {
			ent.Age = age
		}
		if gender, ok := meta["gender"].(string); ok {
			ent.Gender = gender
		}
		if extra, ok := meta["extra"].(map[string]interface{}); ok && len(extra) > 0 {
			ent.ExtraMeta = extra
		}
		if port, ok := meta["portrait"].(string); ok {
			ent.Portrait = port
		}
		if pv, ok := meta["portrait_version"].(float64); ok {
			ent.PortraitVersion = int(pv)
		}
		if ph, ok := meta["portrait_history"].([]interface{}); ok {
			for _, item := range ph {
				if path, ok := item.(string); ok {
					ent.PortraitHistory = append(ent.PortraitHistory, path)
				}
			}
		}
		if aliases, ok := meta["aliases"].([]interface{}); ok {
			for _, value := range aliases {
				if alias, ok := value.(string); ok {
					ent.Aliases = append(ent.Aliases, alias)
				}
			}
		}
		if tags, ok := meta["tags"].([]interface{}); ok {
			for _, value := range tags {
				if tag, ok := value.(string); ok {
					ent.Tags = append(ent.Tags, tag)
				}
			}
		}
		if history, ok := meta["history"].([]interface{}); ok {
			for _, value := range history {
				if number, ok := value.(float64); ok {
					ent.History = append(ent.History, int(number))
				}
			}
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return edges, nil
}

// DeleteEntity removes an entity and its edges from the index. The note on disk is
// the caller's business: the index is derived, and a merge deletes both.
func (s *Store) DeleteEntity(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM edges WHERE source_id = ? OR target_id = ?`, id, id); err != nil {
		return fmt.Errorf("clear edges: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM entities WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete entity: %w", err)
	}
	return tx.Commit()
}

// EntitySummary is a lightweight projection of an indexed entity.
type EntitySummary struct {
	ID       string
	Name     string
	Type     string
	Location string
	Tags     []string
	Aliases  []string
}

// ListEntities returns every indexed entity ordered by entity ID.
func (s *Store) ListEntities() ([]EntitySummary, error) {
	rows, err := s.db.Query(`SELECT id, name, type, frontmatter_json FROM entities ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := make([]EntitySummary, 0)
	for rows.Next() {
		var summary EntitySummary
		var fmJSON string
		if err := rows.Scan(&summary.ID, &summary.Name, &summary.Type, &fmJSON); err != nil {
			return nil, err
		}

		var meta struct {
			Location string   `json:"location"`
			Tags     []string `json:"tags"`
			Aliases  []string `json:"aliases"`
		}
		if err := json.Unmarshal([]byte(fmJSON), &meta); err == nil {
			summary.Location = meta.Location
			summary.Tags = meta.Tags
			summary.Aliases = meta.Aliases
		}

		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return summaries, nil
}

// WorkingSetRecord represents a single row in the working_set table.
type WorkingSetRecord struct {
	EntityID string
	Kind     string
	Weight   float64
	LastTurn int
	Role     string
}

// ReplaceWorkingSet replaces the campaign's working set in a single transaction.
func (s *Store) ReplaceWorkingSet(entries []WorkingSetRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM working_set`); err != nil {
		return fmt.Errorf("clear working_set: %w", err)
	}

	const insert = `INSERT INTO working_set (entity_id, kind, weight, last_turn, role) VALUES (?, ?, ?, ?, ?)`
	for _, e := range entries {
		if _, err := tx.Exec(insert, e.EntityID, e.Kind, e.Weight, e.LastTurn, emptyToNull(e.Role)); err != nil {
			return fmt.Errorf("insert working_set entry %q: %w", e.EntityID, err)
		}
	}
	return tx.Commit()
}

// LoadWorkingSet loads all entries from the working_set table.
func (s *Store) LoadWorkingSet() ([]WorkingSetRecord, error) {
	const query = `SELECT entity_id, kind, weight, last_turn, COALESCE(role, '') FROM working_set ORDER BY weight DESC, last_turn DESC, entity_id ASC`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("load working_set: %w", err)
	}
	defer rows.Close()

	entries := make([]WorkingSetRecord, 0)
	for rows.Next() {
		var e WorkingSetRecord
		if err := rows.Scan(&e.EntityID, &e.Kind, &e.Weight, &e.LastTurn, &e.Role); err != nil {
			return nil, fmt.Errorf("scan working_set entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load working_set: %w", err)
	}
	return entries, nil
}
