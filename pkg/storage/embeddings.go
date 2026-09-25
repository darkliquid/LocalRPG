package storage

import (
	"container/heap"
	"context"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/embeddings"
)

// EmbeddingRecord represents a stored vector embedding.
type EmbeddingRecord struct {
	ID          int64
	TargetType  string
	TargetID    string
	ChunkIndex  int
	ContentHash string
	ModelID     string
	Dimensions  int
	Vector      []float32
}

// VectorHit is a ranked vector similarity match.
type VectorHit struct {
	TargetType string
	TargetID   string
	Score      float32
}

// SaveEmbedding upserts an embedding into SQLite.
func (s *Store) SaveEmbedding(targetType, targetID, contentHash, modelID string, chunkIndex int, vector []float32) error {
	blob := embeddings.EncodeVector(vector)
	query := `
		INSERT INTO embeddings (target_type, target_id, chunk_index, content_hash, model_id, dimensions, vector)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(target_type, target_id, chunk_index) DO UPDATE SET
			content_hash = excluded.content_hash,
			model_id = excluded.model_id,
			dimensions = excluded.dimensions,
			vector = excluded.vector,
			created_at = CURRENT_TIMESTAMP`
	_, err := s.db.Exec(query, targetType, targetID, chunkIndex, contentHash, modelID, len(vector), blob)
	if err != nil {
		return fmt.Errorf("save embedding: %w", err)
	}
	return nil
}

// HasEmbedding reports whether an exact embedding already exists matching target and content hash.
func (s *Store) HasEmbedding(targetType, targetID, contentHash, modelID string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM embeddings WHERE target_type = ? AND target_id = ? AND content_hash = ? AND model_id = ?`
	err := s.db.QueryRow(query, targetType, targetID, contentHash, modelID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("has embedding: %w", err)
	}
	return count > 0, nil
}

// DeleteEmbeddingsFor removes all embeddings for a target.
func (s *Store) DeleteEmbeddingsFor(targetType, targetID string) error {
	query := `DELETE FROM embeddings WHERE target_type = ? AND target_id = ?`
	if _, err := s.db.Exec(query, targetType, targetID); err != nil {
		return fmt.Errorf("delete embeddings: %w", err)
	}
	return nil
}

// vectorMinHeap is a priority queue min-heap of size K for bounded memory matching.
type vectorMinHeap []VectorHit

func (h vectorMinHeap) Len() int           { return len(h) }
func (h vectorMinHeap) Less(i, j int) bool { return h[i].Score < h[j].Score } // smallest score at root
func (h vectorMinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *vectorMinHeap) Push(x interface{}) {
	*h = append(*h, x.(VectorHit))
}
func (h *vectorMinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// SearchSimilarVectors scans matching vectors in a streaming cursor and maintains a top-K min-heap.
// Memory is strictly O(K) regardless of the number of rows in the table.
func (s *Store) SearchSimilarVectors(ctx context.Context, targetTypes []string, modelID string, queryVec []float32, topK int) ([]VectorHit, error) {
	if topK <= 0 {
		topK = 10
	}
	if len(targetTypes) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(targetTypes))
	args := make([]interface{}, 0, len(targetTypes)+1)
	args = append(args, modelID)
	for i, tt := range targetTypes {
		placeholders[i] = "?"
		args = append(args, tt)
	}

	query := fmt.Sprintf(`
		SELECT target_type, target_id, vector
		FROM embeddings
		WHERE model_id = ? AND target_type IN (%s)`, strings.Join(placeholders, ", "))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query embeddings: %w", err)
	}
	defer rows.Close()

	h := &vectorMinHeap{}
	heap.Init(h)

	var (
		tType string
		tID   string
		blob  []byte
	)

	for rows.Next() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err := rows.Scan(&tType, &tID, &blob); err != nil {
			return nil, fmt.Errorf("scan embedding row: %w", err)
		}
		vec, err := embeddings.DecodeVector(blob)
		if err != nil {
			continue // skip malformed row
		}
		sim := embeddings.CosineSimilarity(queryVec, vec)
		hit := VectorHit{
			TargetType: tType,
			TargetID:   tID,
			Score:      sim,
		}

		if h.Len() < topK {
			heap.Push(h, hit)
		} else if sim > (*h)[0].Score {
			heap.Pop(h)
			heap.Push(h, hit)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Extract sorted descending by score
	result := make([]VectorHit, h.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(h).(VectorHit)
	}
	return result, nil
}
