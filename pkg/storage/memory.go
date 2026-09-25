package storage

import (
	"fmt"
	"math"
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

// HasMemory reports whether a memory with the same turn, kind, text, and check
// id already exists. Rebuild keys on content because memory ids are volatile.
func (s *Store) HasMemory(turn int, kind, text, checkID string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM memories WHERE turn = ? AND kind = ? AND text = ? AND COALESCE(check_id, '') = ?`,
		turn, kind, text, checkID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("has memory: %w", err)
	}
	return count > 0, nil
}

// MemoryHit is one memory search match.
type MemoryHit struct {
	ID         int64
	Turn       int
	Kind       string
	Snippet    string
	Importance int
}

// SearchMemories runs an FTS5 MATCH over memory text and tags, with optional
// entity, kind, and importance filters. Results are in bm25 order; callers that
// want importance and recency weighting apply RankMemoryHits.
func (s *Store) SearchMemories(match, entityID, kind string, minImportance, limit int) ([]MemoryHit, error) {
	if limit <= 0 {
		limit = 10
	}
	query := `
		SELECT m.id, m.turn, m.kind, snippet(memories_fts, 0, '[', ']', '…', 12), m.importance
		FROM memories_fts
		JOIN memories m ON m.id = memories_fts.rowid
		WHERE memories_fts MATCH ?`
	args := []interface{}{match}
	if kind != "" {
		query += " AND m.kind = ?"
		args = append(args, kind)
	}
	if minImportance > 0 {
		query += " AND m.importance >= ?"
		args = append(args, minImportance)
	}
	if entityID != "" {
		query += " AND EXISTS (SELECT 1 FROM memory_entities me WHERE me.memory_id = m.id AND me.entity_id = ?)"
		args = append(args, entityID)
	}
	query += " ORDER BY bm25(memories_fts) LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("search memories: %w", err)
	}
	defer rows.Close()

	hits := make([]MemoryHit, 0)
	for rows.Next() {
		var hit MemoryHit
		if err := rows.Scan(&hit.ID, &hit.Turn, &hit.Kind, &hit.Snippet, &hit.Importance); err != nil {
			return nil, fmt.Errorf("scan memory hit: %w", err)
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

// RankMemoryHits re-ranks hits by importance and recency, so an important recent
// memory outranks an equal-text older one. halfLife is in turns; a non-positive
// value disables decay.
func RankMemoryHits(hits []MemoryHit, currentTurn, halfLife int) []MemoryHit {
	if len(hits) == 0 {
		return hits
	}
	if halfLife <= 0 {
		halfLife = 20
	}
	scored := make([]struct {
		hit   MemoryHit
		score float64
	}, len(hits))
	for i, hit := range hits {
		age := currentTurn - hit.Turn
		if age < 0 {
			age = 0
		}
		decay := 1.0
		if age > 0 {
			decay = math.Pow(0.5, float64(age)/float64(halfLife))
		}
		scored[i] = struct {
			hit   MemoryHit
			score float64
		}{hit: hit, score: float64(hit.Importance) * decay}
	}
	for i := 1; i < len(scored); i++ {
		for j := i; j > 0 && scored[j].score > scored[j-1].score; j-- {
			scored[j], scored[j-1] = scored[j-1], scored[j]
		}
	}
	ranked := make([]MemoryHit, len(scored))
	for i, s := range scored {
		ranked[i] = s.hit
	}
	return ranked
}
