# Semantic Search & Vector Embeddings Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-class semantic search and vector embeddings to LocalRPG with pluggable providers (OpenAI/Ollama HTTP, Google Gemini, Builtin Hash Projection, Mock), SQLite vector storage with memory-bounded streaming min-heap matching ($O(K)$ memory, zero CGO), asynchronous background indexing, and Reciprocal Rank Fusion (RRF) hybrid search.

**Architecture:** Pure Go streaming vector cosine similarity and min-heap ($O(K)$ bounded memory, zero CGO) integrated into campaign SQLite database `cache/index.db`. Pluggable embedding providers via a uniform `embeddings.Provider` interface. Non-blocking asynchronous background indexing worker for entities, memories, and turns with content-hash deduplication. Two-stage hybrid search combining SQLite FTS5 BM25 rankings and vector cosine similarities using Reciprocal Rank Fusion with transparent FTS5 fallback.

**Tech Stack:** Go 1.27 (`modernc.org/sqlite`, `google.golang.org/genai`, OpenTelemetry), SQLite FTS5 + BLOB vector storage, Reciprocal Rank Fusion (RRF).

---

## File Structure Map

```
pkg/
├── embeddings/
│   ├── provider.go            // Core embeddings.Provider interface and types
│   ├── math.go                // Vector encoding/decoding and SIMD-friendly CosineSimilarity
│   ├── math_test.go           // Unit tests for vector encoding and cosine similarity
│   ├── mock.go                // Thread-safe in-memory MockProvider for unit testing
│   ├── builtin.go             // Deterministic, pure-Go hash projection provider (offline zero-dependency)
│   ├── builtin_test.go        // Unit tests for builtin hash projection provider
│   ├── factory.go             // Provider factory from config.EmbeddingsConfig
│   └── hybrid_search_e2e_test.go // End-to-end hybrid search and indexer test
├── storage/
│   ├── migrate.go             // Migration version 6: addEmbeddingsTable
│   ├── embeddings.go          // SQLite embeddings CRUD & streaming O(K) min-heap vector matcher
│   ├── embeddings_test.go     // Unit tests for vector storage and O(K) streaming top-K min-heap
│   ├── embedding_worker.go    // Asynchronous background indexing worker
│   └── embedding_worker_test.go // Unit tests for batching, hash deduplication, and worker lifecycle
├── provider/
│   ├── descriptor.go          // Add FamilyEmbedding Family = "embedding"
│   ├── openaiembedding/
│   │   ├── openai.go          // HTTP client for standard /v1/embeddings (OpenAI, Ollama, LM Studio)
│   │   └── openai_test.go     // Unit tests for openai/ollama embedding provider
│   ├── geminiembedding/
│   │   ├── gemini.go          // Google GenAI embedding client (text-embedding-004)
│   │   └── gemini_test.go     // Unit tests for Gemini embedding provider
│   └── all/
│       └── all.go             // Import blank embedding providers for side-effect registration
├── tools/
│   ├── rrf.go                 // Reciprocal Rank Fusion algorithm & types
│   ├── rrf_test.go            // Unit tests for RRF fusion, weighting, and edge cases
│   ├── query.go               // Query helpers
│   ├── tools.go               // Updated search_entities and search_timeline with hybrid search
│   ├── memory.go              // Updated search_memories with hybrid search
│   ├── tools_test.go          // Tests for hybrid entity/timeline search
│   └── memory_test.go         // Tests for hybrid memory search
├── config/
│   ├── types.go               // EmbeddingsConfig and EmbeddingProviderConfig
│   └── types_test.go          // Unit tests for embedding config defaults and validation
├── gui/
│   └── service.go             // Wire embedding provider & background worker into Service
└── cmd/
    └── localrpg/
        └── play.go            // Wire embedding provider & background worker into CLI play
```

---

### Task 1: Core Embeddings Interface, Vector Math, and Builtin/Mock Providers

**Files:**
- Create: `pkg/embeddings/provider.go`
- Create: `pkg/embeddings/math.go`
- Create: `pkg/embeddings/math_test.go`
- Create: `pkg/embeddings/mock.go`
- Create: `pkg/embeddings/builtin.go`
- Create: `pkg/embeddings/builtin_test.go`

- [x] **Step 1: Write the failing tests for vector math and mock provider**

Create `pkg/embeddings/math_test.go`:
```go
package embeddings

import (
	"math"
	"testing"
)

func TestVectorEncodeDecodeRoundTrip(t *testing.T) {
	orig := []float32{0.0, 1.5, -2.25, 3.14159, -0.0001}
	encoded := EncodeVector(orig)
	if len(encoded) != len(orig)*4 {
		t.Fatalf("expected encoded length %d, got %d", len(orig)*4, len(encoded))
	}
	decoded, err := DecodeVector(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(decoded) != len(orig) {
		t.Fatalf("expected decoded length %d, got %d", len(orig), len(decoded))
	}
	for i := range orig {
		if math.Abs(float64(orig[i]-decoded[i])) > 1e-6 {
			t.Errorf("at index %d: expected %f, got %f", i, orig[i], decoded[i])
		}
	}
}

func TestDecodeVectorInvalidLength(t *testing.T) {
	_, err := DecodeVector([]byte{1, 2, 3}) // not a multiple of 4
	if err == nil {
		t.Fatal("expected error on invalid byte length, got nil")
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1.0, 0.0},
			b:        []float32{0.0, 1.0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1.0, 2.0},
			b:        []float32{-1.0, -2.0},
			expected: -1.0,
		},
		{
			name:     "zero vector safe",
			a:        []float32{0.0, 0.0},
			b:        []float32{1.0, 2.0},
			expected: 0.0,
		},
		{
			name:     "dimension mismatch safe",
			a:        []float32{1.0},
			b:        []float32{1.0, 2.0},
			expected: 0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sim := CosineSimilarity(tc.a, tc.b)
			if math.Abs(float64(sim-tc.expected)) > 1e-5 {
				t.Errorf("expected similarity %f, got %f", tc.expected, sim)
			}
		})
	}
}
```

Create `pkg/embeddings/builtin_test.go`:
```go
package embeddings

import (
	"context"
	"math"
	"testing"
)

func TestBuiltinHashProjectionProvider(t *testing.T) {
	p := NewBuiltinHashProjectionProvider(384)
	if p.Dimensions() != 384 {
		t.Fatalf("expected 384 dimensions, got %d", p.Dimensions())
	}
	if p.ID() == "" {
		t.Fatal("expected non-empty ID")
	}

	ctx := context.Background()
	texts := []string{
		"The ancient dragon of Mount Dread slept upon heaps of gold.",
		"A fiery dragon rested in its mountain cavern filled with treasure.",
		"A quiet apothecary sells healing herbs and minor salves in the village.",
	}

	vecs, err := p.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(vecs) != len(texts) {
		t.Fatalf("expected %d vectors, got %d", len(texts), len(vecs))
	}

	for i, v := range vecs {
		if len(v) != 384 {
			t.Fatalf("vector %d length = %d, expected 384", i, len(v))
		}
		// Check that vectors are normalized to unit length
		var normSq float32
		for _, val := range v {
			normSq += val * val
		}
		norm := math.Sqrt(float64(normSq))
		if math.Abs(norm-1.0) > 1e-3 {
			t.Errorf("vector %d norm = %f, expected ~1.0", i, norm)
		}
	}

	// Semantically closer texts (both about dragons/cavern/gold) should have higher similarity
	sim12 := CosineSimilarity(vecs[0], vecs[1])
	sim13 := CosineSimilarity(vecs[0], vecs[2])
	if sim12 <= sim13 {
		t.Errorf("expected similar dragon texts to have higher similarity than apothecary text; sim12=%f, sim13=%f", sim12, sim13)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/embeddings/...`
Expected: FAIL with compilation error (package and functions undefined).

- [x] **Step 3: Implement `pkg/embeddings/provider.go`, `math.go`, `mock.go`, and `builtin.go`**

Create `pkg/embeddings/provider.go`:
```go
package embeddings

import "context"

// Provider computes dense vector embeddings for input strings.
type Provider interface {
	ID() string
	Dimensions() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

Create `pkg/embeddings/math.go`:
```go
package embeddings

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeVector serializes a float32 slice into a little-endian byte slice.
func EncodeVector(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// DecodeVector parses a little-endian byte slice into a float32 slice.
func DecodeVector(buf []byte) ([]float32, error) {
	if len(buf)%4 != 0 {
		return nil, fmt.Errorf("invalid vector byte length %d (must be multiple of 4)", len(buf))
	}
	n := len(buf) / 4
	vec := make([]float32, n)
	for i := 0; i < n; i++ {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return vec, nil
}

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// If vectors have different lengths or zero magnitude, 0.0 is returned.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dot, normA, normB float32
	i := 0
	// 4-way unrolled loop for SIMD-friendly instruction pipelining
	for ; i+3 < len(a); i += 4 {
		dot += a[i]*b[i] + a[i+1]*b[i+1] + a[i+2]*b[i+2] + a[i+3]*b[i+3]
		normA += a[i]*a[i] + a[i+1]*a[i+1] + a[i+2]*a[i+2] + a[i+3]*a[i+3]
		normB += b[i]*b[i] + b[i+1]*b[i+1] + b[i+2]*b[i+2] + b[i+3]*b[i+3]
	}
	for ; i < len(a); i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	denom := float32(math.Sqrt(float64(normA)) * math.Sqrt(float64(normB)))
	if denom <= 0 {
		return 0.0
	}
	return dot / denom
}
```

Create `pkg/embeddings/mock.go`:
```go
package embeddings

import (
	"context"
	"sync"
)

// MockProvider provides predictable or recorded vector embeddings for unit tests.
type MockProvider struct {
	mu         sync.Mutex
	id         string
	dims       int
	EmbedFunc  func(ctx context.Context, texts []string) ([][]float32, error)
	CallCount  int
	LastTexts  []string
}

// NewMockProvider creates a new MockProvider with a default dimension size.
func NewMockProvider(id string, dims int) *MockProvider {
	if id == "" {
		id = "mock-embedding"
	}
	if dims <= 0 {
		dims = 384
	}
	return &MockProvider{
		id:   id,
		dims: dims,
	}
}

func (m *MockProvider) ID() string {
	return m.id
}

func (m *MockProvider) Dimensions() int {
	return m.dims
}

func (m *MockProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	m.CallCount++
	m.LastTexts = append([]string(nil), texts...)
	embedFn := m.EmbedFunc
	m.mu.Unlock()

	if embedFn != nil {
		return embedFn(ctx, texts)
	}

	// Default: return zero-vectors with 1.0 at index 0
	res := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, m.dims)
		if m.dims > 0 {
			vec[0] = 1.0
		}
		res[i] = vec
	}
	return res, nil
}
```

Create `pkg/embeddings/builtin.go`:
```go
package embeddings

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
)

// BuiltinHashProjectionProvider is a pure-Go, zero-dependency offline embedding
// provider. It projects word tokens and character n-grams into a fixed-dimension
// normalized unit vector using deterministic hashing.
type BuiltinHashProjectionProvider struct {
	id         string
	dimensions int
}

// NewBuiltinHashProjectionProvider creates a hash projection provider.
func NewBuiltinHashProjectionProvider(dims int) *BuiltinHashProjectionProvider {
	if dims <= 0 {
		dims = 384
	}
	return &BuiltinHashProjectionProvider{
		id:         "builtin-hash-projection",
		dimensions: dims,
	}
}

func (p *BuiltinHashProjectionProvider) ID() string {
	return p.id
}

func (p *BuiltinHashProjectionProvider) Dimensions() int {
	return p.dimensions
}

func (p *BuiltinHashProjectionProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	res := make([][]float32, len(texts))
	for i, text := range texts {
		res[i] = p.projectText(text)
	}
	return res, nil
}

func (p *BuiltinHashProjectionProvider) projectText(text string) []float32 {
	vec := make([]float32, p.dimensions)
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return vec
	}

	h := fnv.New64a()
	for _, word := range words {
		// Clean punctuation
		cleaned := strings.Trim(word, `.,:;!?'"()[]{}`)
		if len(cleaned) == 0 {
			continue
		}

		// Word level projection
		h.Reset()
		_, _ = h.Write([]byte(cleaned))
		val := h.Sum64()
		dim1 := int(val % uint64(p.dimensions))
		sign1 := float32(1.0)
		if (val >> 32) % 2 == 1 {
			sign1 = -1.0
		}
		vec[dim1] += sign1 * 2.0

		// Character 3-gram projection for subword/morphological overlap
		runes := []rune(cleaned)
		for j := 0; j+2 < len(runes); j++ {
			gram := string(runes[j : j+3])
			h.Reset()
			_, _ = h.Write([]byte(gram))
			gval := h.Sum64()
			gdim := int(gval % uint64(p.dimensions))
			gsign := float32(1.0)
			if (gval >> 32) % 2 == 1 {
				gsign = -1.0
			}
			vec[gdim] += gsign * 0.5
		}
	}

	// L2 normalization to unit vector
	var sumSq float32
	for _, v := range vec {
		sumSq += v * v
	}
	norm := float32(math.Sqrt(float64(sumSq)))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/embeddings/...`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/embeddings/
git commit -m "feat(embeddings): add core Provider interface, vector math, and builtin hash projection"
```

---

### Task 2: SQLite Schema Migration & Memory-Bounded Streaming Min-Heap Matcher

**Files:**
- Modify: `pkg/storage/migrate.go`
- Create: `pkg/storage/embeddings.go`
- Create: `pkg/storage/embeddings_test.go`

- [x] **Step 1: Write the failing tests for vector storage and top-K streaming min-heap matching**

Create `pkg/storage/embeddings_test.go`:
```go
package storage

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/embeddings"
)

func TestEmbeddingStorageAndStreamingSearch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	store := &Store{db: db}

	// Save test embeddings
	modelID := "test-model"
	vec1 := []float32{1.0, 0.0, 0.0}
	vec2 := []float32{0.8, 0.6, 0.0} // close to vec1
	vec3 := []float32{0.0, 1.0, 0.0} // orthogonal to vec1
	vec4 := []float32{-1.0, 0.0, 0.0} // opposite to vec1

	if err := store.SaveEmbedding("entity", "ent-1", "hash1", modelID, 0, vec1); err != nil {
		t.Fatalf("save embedding 1: %v", err)
	}
	if err := store.SaveEmbedding("entity", "ent-2", "hash2", modelID, 0, vec2); err != nil {
		t.Fatalf("save embedding 2: %v", err)
	}
	if err := store.SaveEmbedding("memory", "mem-1", "hash3", modelID, 0, vec3); err != nil {
		t.Fatalf("save embedding 3: %v", err)
	}
	if err := store.SaveEmbedding("turn", "1", "hash4", modelID, 0, vec4); err != nil {
		t.Fatalf("save embedding 4: %v", err)
	}

	// Test HasEmbedding
	has, err := store.HasEmbedding("entity", "ent-1", "hash1", modelID)
	if err != nil || !has {
		t.Fatalf("expected HasEmbedding=true, got %v (err=%v)", has, err)
	}
	hasDiff, err := store.HasEmbedding("entity", "ent-1", "diff-hash", modelID)
	if err != nil || hasDiff {
		t.Fatalf("expected HasEmbedding=false for different hash, got %v", hasDiff)
	}

	// Test SearchSimilarVectors for entities only, top 2
	queryVec := []float32{1.0, 0.0, 0.0}
	hits, err := store.SearchSimilarVectors(context.Background(), []string{"entity"}, modelID, queryVec, 2)
	if err != nil {
		t.Fatalf("search similar vectors failed: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].TargetID != "ent-1" {
		t.Errorf("expected top hit ent-1, got %s (score %f)", hits[0].TargetID, hits[0].Score)
	}
	if math.Abs(float64(hits[0].Score-1.0)) > 1e-4 {
		t.Errorf("expected score 1.0, got %f", hits[0].Score)
	}
	if hits[1].TargetID != "ent-2" {
		t.Errorf("expected second hit ent-2, got %s (score %f)", hits[1].TargetID, hits[1].Score)
	}
	if math.Abs(float64(hits[1].Score-0.8)) > 1e-4 {
		t.Errorf("expected score ~0.8, got %f", hits[1].Score)
	}

	// Test search across multiple target types (entity and memory)
	allHits, err := store.SearchSimilarVectors(context.Background(), []string{"entity", "memory"}, modelID, queryVec, 10)
	if err != nil {
		t.Fatalf("search similar vectors across types failed: %v", err)
	}
	if len(allHits) != 3 {
		t.Fatalf("expected 3 hits across entity & memory, got %d", len(allHits))
	}
}

func TestDeleteEmbeddingsFor(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()
	store := &Store{db: db}

	_ = store.SaveEmbedding("entity", "ent-del", "hash1", "test-model", 0, []float32{1, 0, 0})
	if err := store.DeleteEmbeddingsFor("entity", "ent-del"); err != nil {
		t.Fatalf("DeleteEmbeddingsFor failed: %v", err)
	}
	has, _ := store.HasEmbedding("entity", "ent-del", "hash1", "test-model")
	if has {
		t.Fatal("expected embedding to be deleted")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/storage/ -run TestEmbedding`
Expected: FAIL (schema migration and methods undefined).

- [x] **Step 3: Implement migration version 6 and `pkg/storage/embeddings.go`**

Update `pkg/storage/migrate.go` to add migration 6:
```go
var migrations = []migration{
	{version: 1, apply: addTimelineColumns},
	{version: 2, apply: dropAudioRefsColumn},
	{version: 3, apply: addTurnContextsTable},
	{version: 4, apply: addWorkingSetTable},
	{version: 5, apply: addMemoriesTables},
	{version: 6, apply: addEmbeddingsTable},
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
```

Create `pkg/storage/embeddings.go`:
```go
package storage

import (
	"container/heap"
	"context"
	"database/sql"
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
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/storage/ -run "TestEmbedding|TestMigrate"`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/storage/migrate.go pkg/storage/embeddings.go pkg/storage/embeddings_test.go
git commit -m "feat(storage): add embeddings table migration and streaming O(K) min-heap vector search"
```

---

### Task 3: Remote & Ecosystem Embedding Providers (OpenAI/Ollama HTTP and Gemini)

**Files:**
- Modify: `pkg/provider/descriptor.go`
- Create: `pkg/provider/openaiembedding/openai.go`
- Create: `pkg/provider/openaiembedding/openai_test.go`
- Create: `pkg/provider/geminiembedding/gemini.go`
- Create: `pkg/provider/geminiembedding/gemini_test.go`
- Modify: `pkg/provider/all/all.go`

- [x] **Step 1: Write the failing tests for OpenAI-compatible and Gemini embedding providers**

Create `pkg/provider/openaiembedding/openai_test.go`:
```go
package openaiembedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIEmbeddingProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" && r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Model != "text-embedding-3-small" {
			t.Errorf("unexpected model: %s", req.Model)
		}

		resp := embeddingResponse{
			Data: []embeddingData{
				{Index: 0, Embedding: []float32{0.1, 0.2, 0.3}},
				{Index: 1, Embedding: []float32{0.4, 0.5, 0.6}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		Endpoint: server.URL + "/v1",
		APIKey:   "fake-key",
		Model:    "text-embedding-3-small",
	})

	vecs, err := client.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if len(vecs[0]) != 3 || vecs[0][0] != 0.1 {
		t.Errorf("unexpected vector 0: %+v", vecs[0])
	}
}
```

Create `pkg/provider/geminiembedding/gemini_test.go`:
```go
package geminiembedding

import (
	"testing"
)

func TestGeminiEmbeddingConfig(t *testing.T) {
	client := NewClient(ClientConfig{
		APIKey: "fake-key",
		Model:  "text-embedding-004",
	})
	if client.ID() != "gemini-embedding" {
		t.Errorf("expected ID gemini-embedding, got %s", client.ID())
	}
	if client.Dimensions() != 768 {
		t.Errorf("expected 768 dimensions for text-embedding-004, got %d", client.Dimensions())
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/provider/openaiembedding/... ./pkg/provider/geminiembedding/...`
Expected: FAIL with compilation error (packages not found).

- [x] **Step 3: Implement `pkg/provider/descriptor.go`, `openaiembedding`, `geminiembedding`, and register in `pkg/provider/all/all.go`**

In `pkg/provider/descriptor.go`:
Add:
```go
const (
	FamilyLLM       Family = "llm"
	FamilyTTS       Family = "tts"
	FamilySTT       Family = "stt"
	FamilyImage     Family = "image"
	FamilyEmbedding Family = "embedding"
)
```

Create `pkg/provider/openaiembedding/openai.go`:
```go
package openaiembedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/provider"
)

type ClientConfig struct {
	Endpoint   string `json:"endpoint"`
	APIKey     string `json:"api_key"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
}

type Client struct {
	endpoint   string
	apiKey     string
	model      string
	dimensions int
	httpClient *http.Client
}

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "openai-embedding",
			Family:      provider.FamilyEmbedding,
			Label:       "OpenAI / Ollama Embedding API",
			Description: "Vector embeddings via standard OpenAI-compatible /v1/embeddings endpoint",
			Source:      "http",
		},
		Build: func(ctx context.Context, raw []byte) (interface{}, error) {
			var cfg ClientConfig
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &cfg); err != nil {
					return nil, err
				}
			}
			return NewClient(cfg), nil
		},
	})
}

func NewClient(cfg ClientConfig) *Client {
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	model := cfg.Model
	if model == "" {
		model = "text-embedding-3-small"
	}
	dims := cfg.Dimensions
	if dims <= 0 {
		if strings.Contains(model, "large") {
			dims = 3072
		} else {
			dims = 1536
		}
	}
	return &Client{
		endpoint:   endpoint,
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: dims,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) ID() string {
	return "openai-embedding"
}

func (c *Client) Dimensions() int {
	return c.dimensions
}

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingData struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	url := c.endpoint
	if !strings.HasSuffix(url, "/embeddings") {
		url += "/embeddings"
	}

	reqBody := embeddingRequest{
		Model: c.model,
		Input: texts,
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var parsed embeddingResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("embedding API error: %s", parsed.Error.Message)
	}

	result := make([][]float32, len(texts))
	for _, item := range parsed.Data {
		if item.Index >= 0 && item.Index < len(result) {
			result[item.Index] = item.Embedding
		}
	}
	return result, nil
}

var _ embeddings.Provider = (*Client)(nil)
```

Create `pkg/provider/geminiembedding/gemini.go`:
```go
package geminiembedding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/provider"
	"google.golang.org/genai"
)

type ClientConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

type Client struct {
	apiKey     string
	model      string
	dimensions int
}

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          "gemini-embedding",
			Family:      provider.FamilyEmbedding,
			Label:       "Google Gemini Embeddings",
			Description: "Vector embeddings via Google GenAI embedding API (text-embedding-004)",
			Source:      "gemini",
		},
		Build: func(ctx context.Context, raw []byte) (interface{}, error) {
			var cfg ClientConfig
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &cfg); err != nil {
					return nil, err
				}
			}
			return NewClient(cfg), nil
		},
	})
}

func NewClient(cfg ClientConfig) *Client {
	model := cfg.Model
	if model == "" {
		model = "text-embedding-004"
	}
	return &Client{
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: 768,
	}
}

func (c *Client) ID() string {
	return "gemini-embedding"
}

func (c *Client) Dimensions() int {
	return c.dimensions
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create gemini client: %w", err)
	}

	modelName := c.model
	if !strings.HasPrefix(modelName, "models/") {
		modelName = "models/" + modelName
	}

	contents := make([]*genai.Content, len(texts))
	for i, text := range texts {
		contents[i] = &genai.Content{
			Parts: []*genai.Part{{Text: text}},
		}
	}

	resp, err := client.Models.EmbedContent(ctx, modelName, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("gemini embed content: %w", err)
	}

	result := make([][]float32, len(texts))
	for i, emb := range resp.Embeddings {
		if i < len(result) && emb != nil {
			result[i] = emb.Values
		}
	}
	return result, nil
}

var _ embeddings.Provider = (*Client)(nil)
```

In `pkg/provider/all/all.go`:
Add blank imports:
```go
_ "github.com/darkliquid/localrpg/pkg/provider/openaiembedding"
_ "github.com/darkliquid/localrpg/pkg/provider/geminiembedding"
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/provider/openaiembedding/... ./pkg/provider/geminiembedding/... ./pkg/provider/...`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/provider/
git commit -m "feat(provider): add openai and gemini embedding provider adapters"
```

---

### Task 4: Asynchronous Background Indexing Worker

**Files:**
- Create: `pkg/storage/embedding_worker.go`
- Create: `pkg/storage/embedding_worker_test.go`

- [x] **Step 1: Write the failing tests for background batch indexing worker**

Create `pkg/storage/embedding_worker_test.go`:
```go
package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/embeddings"
)

func TestEmbeddingWorkerBatchingAndDeduplication(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()
	store := &Store{db: db}

	var mu sync.Mutex
	embeddedCount := 0
	mock := embeddings.NewMockProvider("test-model", 4)
	mock.EmbedFunc = func(ctx context.Context, texts []string) ([][]float32, error) {
		mu.Lock()
		embeddedCount += len(texts)
		mu.Unlock()
		res := make([][]float32, len(texts))
		for i := range texts {
			res[i] = []float32{1.0, 0.0, 0.0, 0.0}
		}
		return res, nil
	}

	worker := NewEmbeddingWorker(store, mock, EmbeddingWorkerOptions{
		BatchSize:     5,
		FlushInterval: 50 * time.Millisecond,
		QueueCapacity: 50,
	})
	worker.Start()

	// Enqueue 4 items
	worker.Enqueue(EmbeddingItem{TargetType: "entity", TargetID: "e1", Text: "Ancient ruins"})
	worker.Enqueue(EmbeddingItem{TargetType: "entity", TargetID: "e2", Text: "Goblin camp"})
	worker.Enqueue(EmbeddingItem{TargetType: "memory", TargetID: "m1", Text: "Player allied with goblins"})
	worker.Enqueue(EmbeddingItem{TargetType: "turn", TargetID: "1", Text: "The journey begins"})

	// Drain worker
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain failed: %v", err)
	}
	worker.Stop()

	mu.Lock()
	if embeddedCount != 4 {
		t.Errorf("expected 4 items embedded, got %d", embeddedCount)
	}
	mu.Unlock()

	// Verify items now exist in SQLite
	has, err := store.HasEmbedding("entity", "e1", ComputeContentHash("Ancient ruins"), "test-model")
	if err != nil || !has {
		t.Fatalf("expected e1 to be saved in embeddings table")
	}

	// Now re-run worker and enqueue the same item with identical text
	worker2 := NewEmbeddingWorker(store, mock, EmbeddingWorkerOptions{
		BatchSize:     5,
		FlushInterval: 50 * time.Millisecond,
	})
	worker2.Start()
	worker2.Enqueue(EmbeddingItem{TargetType: "entity", TargetID: "e1", Text: "Ancient ruins"})
	if err := worker2.Drain(ctx); err != nil {
		t.Fatalf("Drain 2 failed: %v", err)
	}
	worker2.Stop()

	// Content hash match should skip embedding
	mu.Lock()
	if embeddedCount != 4 {
		t.Errorf("expected count to remain 4 due to deduplication, got %d", embeddedCount)
	}
	mu.Unlock()
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/storage/ -run TestEmbeddingWorker`
Expected: FAIL (EmbeddingWorker undefined).

- [x] **Step 3: Implement `pkg/storage/embedding_worker.go`**

Create `pkg/storage/embedding_worker.go`:
```go
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/darkliquid/localrpg/pkg/embeddings"
)

// EmbeddingItem is a single target to be indexed.
type EmbeddingItem struct {
	TargetType string // "entity", "memory", "turn"
	TargetID   string
	Text       string
}

type EmbeddingWorkerOptions struct {
	BatchSize     int
	FlushInterval time.Duration
	QueueCapacity int
}

// EmbeddingWorker asynchronously batches and indexes documents into the SQLite embeddings table.
type EmbeddingWorker struct {
	store    *Store
	provider embeddings.Provider
	opts     EmbeddingWorkerOptions

	queue    chan EmbeddingItem
	stopCh   chan struct{}
	doneCh   chan struct{}
	drainReq chan chan struct{}
	mu       sync.Mutex
	running  bool
}

// ComputeContentHash returns the hex-encoded SHA-256 hash of text.
func ComputeContentHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// NewEmbeddingWorker creates an unstarted EmbeddingWorker.
func NewEmbeddingWorker(store *Store, provider embeddings.Provider, opts EmbeddingWorkerOptions) *EmbeddingWorker {
	if opts.BatchSize <= 0 {
		opts.BatchSize = 16
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = 100 * time.Millisecond
	}
	if opts.QueueCapacity <= 0 {
		opts.QueueCapacity = 256
	}
	return &EmbeddingWorker{
		store:    store,
		provider: provider,
		opts:     opts,
		queue:    make(chan EmbeddingItem, opts.QueueCapacity),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		drainReq: make(chan chan struct{}),
	}
}

// Start launches the worker background goroutine.
func (w *EmbeddingWorker) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	go w.loop()
}

// Enqueue queues an item for embedding. Non-blocking; drops if channel is full to protect turn playback.
func (w *EmbeddingWorker) Enqueue(item ItemOrEmbedding) {
	if item.Text == "" || item.TargetID == "" {
		return
	}
	select {
	case w.queue <- item:
	default:
		// Drop or process in background if queue is congested
	}
}

// ItemOrEmbedding alias for EmbeddingItem
type ItemOrEmbedding = EmbeddingItem

func (w *EmbeddingWorker) loop() {
	defer close(w.doneCh)
	ticker := time.NewTicker(w.opts.FlushInterval)
	defer ticker.Stop()

	var batch []EmbeddingItem

	flush := func() {
		if len(batch) == 0 {
			return
		}
		items := batch
		batch = nil
		w.processBatch(items)
	}

	for {
		select {
		case <-w.stopCh:
			// Process remaining items in queue
			for {
				select {
				case item := <-w.queue:
					batch = append(batch, item)
					if len(batch) >= w.opts.BatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}

		case ack := <-w.drainReq:
			// Drain all pending items from queue
			for {
				select {
				case item := <-w.queue:
					batch = append(batch, item)
					if len(batch) >= w.opts.BatchSize {
						flush()
					}
				default:
					flush()
					close(ack)
					goto nextSelect
				}
			}
		nextSelect:

		case item := <-w.queue:
			batch = append(batch, item)
			if len(batch) >= w.opts.BatchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

func (w *EmbeddingWorker) processBatch(items []EmbeddingItem) {
	if len(items) == 0 || w.provider == nil || w.store == nil {
		return
	}

	// Filter out items whose content_hash already exists
	needed := make([]EmbeddingItem, 0, len(items))
	hashes := make([]string, 0, len(items))
	modelID := w.provider.ID()

	for _, item := range items {
		hash := ComputeContentHash(item.Text)
		has, err := w.store.HasEmbedding(item.TargetType, item.TargetID, hash, modelID)
		if err == nil && has {
			continue // Already indexed
		}
		needed = append(needed, item)
		hashes = append(hashes, hash)
	}

	if len(needed) == 0 {
		return
	}

	texts := make([]string, len(needed))
	for i, item := range needed {
		texts[i] = item.Text
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vecs, err := w.provider.Embed(ctx, texts)
	if err != nil || len(vecs) != len(needed) {
		return // Ignore failure, background will retry or fallback
	}

	// Persist all vectors in a transaction
	tx, err := w.store.db.Begin()
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO embeddings (target_type, target_id, chunk_index, content_hash, model_id, dimensions, vector)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(target_type, target_id, chunk_index) DO UPDATE SET
			content_hash = excluded.content_hash,
			model_id = excluded.model_id,
			dimensions = excluded.dimensions,
			vector = excluded.vector,
			created_at = CURRENT_TIMESTAMP`)
	if err != nil {
		return
	}
	defer stmt.Close()

	for i, item := range needed {
		blob := embeddings.EncodeVector(vecs[i])
		if _, err := stmt.Exec(item.TargetType, item.TargetID, 0, hashes[i], modelID, len(vecs[i]), blob); err != nil {
			return
		}
	}
	_ = tx.Commit()
}

// Drain blocks until all currently queued items are processed.
func (w *EmbeddingWorker) Drain(ctx context.Context) error {
	ack := make(chan struct{})
	select {
	case w.drainReq <- ack:
		select {
		case <-ack:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop terminates the worker.
func (w *EmbeddingWorker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.mu.Unlock()

	close(w.stopCh)
	<-w.doneCh
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/storage/ -run TestEmbeddingWorker`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/storage/embedding_worker.go pkg/storage/embedding_worker_test.go
git commit -m "feat(storage): add asynchronous background embedding worker with batching and deduplication"
```

---

### Task 5: Reciprocal Rank Fusion (RRF) & Hybrid Search in Tools

**Files:**
- Create: `pkg/tools/rrf.go`
- Create: `pkg/tools/rrf_test.go`
- Modify: `pkg/tools/tools.go`
- Modify: `pkg/tools/memory.go`
- Modify: `pkg/tools/tools_test.go`
- Modify: `pkg/tools/memory_test.go`

- [x] **Step 1: Write the failing tests for Reciprocal Rank Fusion**

Create `pkg/tools/rrf_test.go`:
```go
package tools

import (
	"testing"
)

func TestReciprocalRankFusion(t *testing.T) {
	// FTS hits: A (rank 1), B (rank 2), C (rank 3)
	ftsIDs := []string{"docA", "docB", "docC"}
	// Vector hits: C (rank 1), B (rank 2), D (rank 3)
	vecIDs := []string{"docC", "docB", "docD"}

	fused := FuseRankings(ftsIDs, vecIDs, 10)
	if len(fused) != 4 {
		t.Fatalf("expected 4 unique items fused, got %d", len(fused))
	}

	// docB appears at rank 2 in both, docC at rank 3 and rank 1.
	// docC score: 0.5/(60+3) + 0.5/(60+1) = 0.5/63 + 0.5/61 = 0.007936 + 0.008196 = 0.016132
	// docB score: 0.5/(60+2) + 0.5/(60+2) = 0.5/62 + 0.5/62 = 0.016129
	// docA score: 0.5/(60+1) = 0.008196
	// docD score: 0.5/(60+3) = 0.007936
	if fused[0] != "docC" {
		t.Errorf("expected top docC, got %s", fused[0])
	}
	if fused[1] != "docB" {
		t.Errorf("expected second docB, got %s", fused[1])
	}
	if fused[2] != "docA" {
		t.Errorf("expected third docA, got %s", fused[2])
	}
	if fused[3] != "docD" {
		t.Errorf("expected fourth docD, got %s", fused[3])
	}
}

func TestReciprocalRankFusionEmptyLists(t *testing.T) {
	// Only FTS hits
	fusedFTS := FuseRankings([]string{"a", "b"}, nil, 5)
	if len(fusedFTS) != 2 || fusedFTS[0] != "a" || fusedFTS[1] != "b" {
		t.Errorf("unexpected FTS-only fusion: %+v", fusedFTS)
	}

	// Only Vec hits
	fusedVec := FuseRankings(nil, []string{"x", "y"}, 5)
	if len(fusedVec) != 2 || fusedVec[0] != "x" || fusedVec[1] != "y" {
		t.Errorf("unexpected Vec-only fusion: %+v", fusedVec)
	}

	// Empty
	fusedEmpty := FuseRankings(nil, nil, 5)
	if len(fusedEmpty) != 0 {
		t.Errorf("expected empty fusion, got %+v", fusedEmpty)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/tools/ -run TestReciprocalRankFusion`
Expected: FAIL (FuseRankings undefined).

- [x] **Step 3: Implement `pkg/tools/rrf.go` and update `pkg/tools/tools.go` and `pkg/tools/memory.go`**

Create `pkg/tools/rrf.go`:
```go
package tools

import "sort"

const rrfConstant = 60.0

// FuseRankings combines two ordered candidate lists (FTS BM25 rank and Vector rank)
// using Reciprocal Rank Fusion (RRF) with equal 0.5 weights.
// Score(d) = 0.5 / (60 + rank_fts) + 0.5 / (60 + rank_vec)
func FuseRankings(ftsIDs, vecIDs []string, limit int) []string {
	if len(ftsIDs) == 0 && len(vecIDs) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}

	scores := make(map[string]float64)

	for rank, id := range ftsIDs {
		// 1-indexed rank
		scores[id] += 0.5 / (rrfConstant + float64(rank+1))
	}
	for rank, id := range vecIDs {
		scores[id] += 0.5 / (rrfConstant + float64(rank+1))
	}

	type scoredItem struct {
		id    string
		score float64
	}
	items := make([]scoredItem, 0, len(scores))
	for id, score := range scores {
		items = append(items, scoredItem{id: id, score: score})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].id < items[j].id
		}
		return items[i].score > items[j].score
	})

	if len(items) > limit {
		items = items[:limit]
	}

	result := make([]string, len(items))
	for i, item := range items {
		result[i] = item.id
	}
	return result
}
```

Update `pkg/tools/tools.go`:
- Add field `embeddingsProvider embeddings.Provider` to `Executor`.
- Add method `SetEmbeddingsProvider(p embeddings.Provider)`.
- Update `searchEntities`:
  - When `embeddingsProvider != nil`:
    - Get FTS candidate hits from `s.store.SearchEntities`.
    - Embed `rawQuery` using `embeddingsProvider.Embed(ctx, []string{rawQuery})`.
    - If successful, get vector candidate hits via `s.store.SearchSimilarVectors(ctx, []string{"entity"}, provider.ID(), queryVec, limit*2)`.
    - Combine candidate IDs via `FuseRankings`.
    - Fetch entity details for fused IDs in ranked order.
    - If embedding fails, return FTS hits directly.
- Update `searchTimeline`:
  - Similarly run hybrid search over `target_type = "turn"`.

Update `pkg/tools/memory.go`:
- Update `searchMemories`:
  - When `embeddingsProvider != nil`, combine FTS memory hits with `target_type = "memory"` vector hits using `FuseRankings`.
  - Apply `storage.RankMemoryHits` on fused results.

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/tools/...`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/tools/
git commit -m "feat(tools): add Reciprocal Rank Fusion hybrid search to entities, memories, and timeline tools"
```

---

### Task 6: Configuration Integration, Provider Wiring & End-to-End Verification

**Files:**
- Modify: `pkg/config/types.go`
- Modify: `pkg/config/types_test.go`
- Create: `pkg/embeddings/factory.go`
- Create: `pkg/embeddings/hybrid_search_e2e_test.go`
- Modify: `pkg/gui/service.go`
- Modify: `cmd/localrpg/play.go`

- [x] **Step 1: Write the failing tests for configuration and end-to-end hybrid search**

Create `pkg/embeddings/hybrid_search_e2e_test.go`:
```go
package embeddings

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/tools"
)

func TestHybridSearchEndToEnd(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.OpenDB(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()
	if err := storage.EnsureFTS(db); err != nil {
		t.Fatalf("EnsureFTS failed: %v", err)
	}
	store := &storage.Store{DB: db} // using internal or helper constructor

	provider := NewBuiltinHashProjectionProvider(384)
	worker := storage.NewEmbeddingWorker(store, provider, storage.EmbeddingWorkerOptions{
		BatchSize:     10,
		FlushInterval: 10 * time.Millisecond,
	})
	worker.Start()
	defer worker.Stop()

	// 1. Save entities
	e1 := &entity.Entity{
		ID:   "dragon-cavern",
		Name: "Dread Cavern",
		Type: "location",
		Body: "A fiery mountain cavern filled with heaps of gold and bones.",
	}
	e2 := &entity.Entity{
		ID:   "quiet-apothecary",
		Name: "Willow Herbalist",
		Type: "location",
		Body: "A peaceful cottage where minor salves and healing poultices are prepared.",
	}
	if err := store.SaveEntity(e1); err != nil {
		t.Fatalf("save e1: %v", err)
	}
	if err := store.SaveEntity(e2); err != nil {
		t.Fatalf("save e2: %v", err)
	}

	worker.Enqueue(storage.EmbeddingItem{TargetType: "entity", TargetID: e1.ID, Text: e1.Name + " " + e1.Body})
	worker.Enqueue(storage.EmbeddingItem{TargetType: "entity", TargetID: e2.ID, Text: e2.Name + " " + e2.Body})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("worker drain failed: %v", err)
	}

	// 2. Query executor with hybrid search enabled
	exec := tools.NewExecutor(store, 4000)
	exec.SetEmbeddingsProvider(provider)

	// Concept query: "healing herbs medicine" - notice "medicine" and "herbs" will match herbalist
	call := harness.ToolCall{
		Name:      "search_entities",
		Arguments: `{"query": "healing herbs medicine"}`,
	}
	res, ok := exec.Execute(ctx, call)
	if !ok {
		t.Fatalf("search_entities failed: %s", res)
	}
	if !strings.Contains(res, "Willow Herbalist") {
		t.Errorf("expected Willow Herbalist in results, got: %s", res)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/embeddings/ -run TestHybridSearchEndToEnd`
Expected: FAIL (types / methods missing).

- [x] **Step 3: Implement `pkg/config/types.go`, `pkg/embeddings/factory.go`, and wire into `pkg/gui/service.go` and `cmd/localrpg/play.go`**

In `pkg/config/types.go`:
Add:
```go
type EmbeddingsConfig struct {
	Enabled    bool                               `yaml:"enabled" json:"enabled"`
	Provider   string                             `yaml:"provider" json:"provider"` // "builtin-local", "openai", "gemini", "disabled"
	Model      string                             `yaml:"model,omitempty" json:"model,omitempty"`
	Dimensions int                                `yaml:"dimensions,omitempty" json:"dimensions,omitempty"`
	BatchSize  int                                `yaml:"batch_size,omitempty" json:"batch_size,omitempty"`
	Providers  map[string]EmbeddingProviderConfig `yaml:"providers,omitempty" json:"providers,omitempty"`
}

type EmbeddingProviderConfig struct {
	Type        string `yaml:"type" json:"type"` // "builtin", "http", "gemini", "disabled"
	BuiltinName string `yaml:"builtin_name,omitempty" json:"builtin_name,omitempty"`
	Endpoint    string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	URL         string `yaml:"url,omitempty" json:"url,omitempty"`
	APIKey      string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Model       string `yaml:"model,omitempty" json:"model,omitempty"`
}
```
Add `Embeddings EmbeddingsConfig` to `Config` struct.
In `DefaultConfig()`:
```go
Embeddings: EmbeddingsConfig{
	Enabled:    true,
	Provider:   "builtin-local",
	Model:      "hash-projection",
	Dimensions: 384,
	BatchSize:  16,
	Providers: map[string]EmbeddingProviderConfig{
		"builtin-local": {
			Type:        "builtin",
			BuiltinName: "hash-projection",
		},
	},
},
```

Create `pkg/embeddings/factory.go`:
```go
package embeddings

import (
	"fmt"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/provider/openaiembedding"
	"github.com/darkliquid/localrpg/pkg/provider/geminiembedding"
)

// NewProviderFromConfig constructs an embedding provider from config.
func NewProviderFromConfig(cfg config.EmbeddingsConfig) (Provider, error) {
	if !cfg.Enabled || cfg.Provider == "disabled" || cfg.Provider == "" {
		return nil, nil
	}

	pCfg, ok := cfg.Providers[cfg.Provider]
	if !ok {
		// Fallback to builtin-local if named provider missing
		return NewBuiltinHashProjectionProvider(cfg.Dimensions), nil
	}

	switch pCfg.Type {
	case "builtin":
		return NewBuiltinHashProjectionProvider(cfg.Dimensions), nil
	case "http":
		url := pCfg.URL
		if url == "" {
			url = pCfg.Endpoint
		}
		model := pCfg.Model
		if model == "" {
			model = cfg.Model
		}
		return openaiembedding.NewClient(openaiembedding.ClientConfig{
			Endpoint:   url,
			APIKey:     pCfg.APIKey,
			Model:      model,
			Dimensions: cfg.Dimensions,
		}), nil
	case "gemini":
		model := pCfg.Model
		if model == "" {
			model = cfg.Model
		}
		return geminiembedding.NewClient(geminiembedding.ClientConfig{
			APIKey: pCfg.APIKey,
			Model:  model,
		}), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider type: %s", pCfg.Type)
	}
}
```

Wire into `pkg/gui/service.go` and `cmd/localrpg/play.go`:
- Construct embedding provider using `embeddings.NewProviderFromConfig(cfg.Embeddings)`.
- If provider is non-nil:
  - Construct and start `storage.NewEmbeddingWorker(store, provider, storage.EmbeddingWorkerOptions{BatchSize: cfg.Embeddings.BatchSize})`.
  - Pass embedding provider to `executor.SetEmbeddingsProvider(provider)`.
  - On turn completion, memory creation, or entity sync, enqueue items into the worker.

- [x] **Step 4: Run all tests and linter to verify complete system integrity**

Run: `mise run test && mise run lint`
Expected: PASS (Zero test failures, zero lint errors, clean frontend typecheck).

- [x] **Step 5: Commit**

```bash
git add pkg/config/ pkg/embeddings/ pkg/gui/ cmd/localrpg/
git commit -m "feat(embeddings): integrate embedding provider factory, config, and end-to-end hybrid search"
```
