package storage

import (
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// SaveMemory stores one memory with its entity links, tags, and full-text row in
// a single transaction.
func (s *Store) SaveMemory(m *entity.Memory) (int64, error) {
	if err := entity.ValidateMemory(m); err != nil {
		return 0, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin memory: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(
		`INSERT INTO memories (turn, kind, text, importance, source, check_id) VALUES (?, ?, ?, ?, ?, ?)`,
		m.Turn, m.Kind, m.Text, m.Importance, string(m.Source), nullIfEmpty(m.CheckID),
	)
	if err != nil {
		return 0, fmt.Errorf("insert memory: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("memory id: %w", err)
	}

	for _, ref := range m.EntityRefs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO memory_entities (memory_id, entity_id) VALUES (?, ?)`, id, ref); err != nil {
			return 0, fmt.Errorf("link memory entity: %w", err)
		}
	}
	for _, tag := range m.Tags {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO memory_tags (memory_id, tag) VALUES (?, ?)`, id, tag); err != nil {
			return 0, fmt.Errorf("memory tag: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO memories_fts (rowid, text, tags) VALUES (?, ?, ?)`, id, m.Text, strings.Join(m.Tags, " ")); err != nil {
		return 0, fmt.Errorf("index memory: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit memory: %w", err)
	}
	return id, nil
}

// ListMemoriesForEntity returns an entity's memories newest-first.
func (s *Store) ListMemoriesForEntity(entityID string, limit int) ([]entity.Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT m.id, m.turn, m.kind, m.text, m.importance, m.source, COALESCE(m.check_id, '')
		FROM memories m
		JOIN memory_entities me ON me.memory_id = m.id
		WHERE me.entity_id = ?
		ORDER BY m.turn DESC, m.id DESC
		LIMIT ?`, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()

	memories := make([]entity.Memory, 0)
	ids := make([]int64, 0)
	for rows.Next() {
		var m entity.Memory
		var source string
		if err := rows.Scan(&m.ID, &m.Turn, &m.Kind, &m.Text, &m.Importance, &source, &m.CheckID); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		m.Source = entity.MemorySource(source)
		memories = append(memories, m)
		ids = append(ids, m.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachMemoryLinks(memories, ids); err != nil {
		return nil, err
	}
	return memories, nil
}

// attachMemoryLinks fills each memory's entity refs and tags.
func (s *Store) attachMemoryLinks(memories []entity.Memory, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	byID := make(map[int64]int, len(ids))
	for i, id := range ids {
		byID[id] = i
	}

	refRows, err := s.db.Query(`SELECT memory_id, entity_id FROM memory_entities`)
	if err != nil {
		return fmt.Errorf("memory refs: %w", err)
	}
	defer refRows.Close()
	for refRows.Next() {
		var id int64
		var ref string
		if err := refRows.Scan(&id, &ref); err != nil {
			return err
		}
		if i, ok := byID[id]; ok {
			memories[i].EntityRefs = append(memories[i].EntityRefs, ref)
		}
	}

	tagRows, err := s.db.Query(`SELECT memory_id, tag FROM memory_tags`)
	if err != nil {
		return fmt.Errorf("memory tags: %w", err)
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var id int64
		var tag string
		if err := tagRows.Scan(&id, &tag); err != nil {
			return err
		}
		if i, ok := byID[id]; ok {
			memories[i].Tags = append(memories[i].Tags, tag)
		}
	}
	return nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
