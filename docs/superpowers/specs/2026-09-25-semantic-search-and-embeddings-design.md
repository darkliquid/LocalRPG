# Semantic Search & Vector Embeddings Design

- **Date:** 2026-09-25
- **Status:** Approved
- **Scope:** Search & vector embeddings architecture (`pkg/embeddings`, `pkg/storage`, `pkg/provider`, `pkg/tools`, `pkg/models`)
- **Related:** `docs/superpowers/specs/2026-09-25-entity-memories-and-memory-tools-design.md`, `pkg/storage/fts.go`, `pkg/storage/db.go`, `pkg/tools/query.go`

---

## 1. Overview & Goals

LocalRPG currently relies on SQLite FTS5 for searching entities (`entities_fts`), turn narration (`turns_fts`), and memories (`memories_fts`). Keyword matching works well for literal names and exact words, but fails on conceptual recall (e.g., retrieving "places associated with death", "healing herbs", or memories about "betrayal" when those specific words do not appear in the text).

This specification adds first-class vector embeddings and hybrid semantic search to LocalRPG. The architecture maintains LocalRPG's core constraints: **local-first**, **zero CGO (`modernc.org/sqlite`)**, **single-binary cross-platform portability**, and **disposable/rebuildable SQLite indexes**.

### 1.1 Goals

1. **Comprehensive Search Scope**: Embed and index:
   - Entities (characters, locations, factions, items, lore notes).
   - Memories (entity memories authored by GM or engine checks).
   - Turns (player input prompts and GM narrations).
2. **Pluggable Embedding Providers**: Uniform provider interface supporting:
   - Remote HTTP APIs: OpenAI (`text-embedding-3-small/large`), local Ollama (`/v1/embeddings`), LM Studio, vLLM.
   - Remote Cloud APIs: Google Gemini (`text-embedding-004`).
   - Local In-Process ONNX: Built-in inference via `sherpa-onnx` (e.g. `bge-small-en-v1.5`, 384 dimensions) managed via `models.Manager`.
   - Disabled / Mock: Clean degradation to pure FTS5 keyword search.
3. **Memory-Bounded Streaming Vector Scan**: Use pure-Go vector similarity with a streaming cursor and top-$K$ min-heap ($O(K)$ memory complexity, < 1 MB RAM) to prevent memory ballooning on large worlds and long-running campaigns.
4. **Two-Stage Hybrid Search**: Combine SQLite FTS5 BM25 keyword rankings with vector cosine similarities using Reciprocal Rank Fusion (RRF).
5. **Asynchronous Non-Blocking Indexing**: Background worker calculates embeddings without blocking turn playback or streaming.
6. **Graceful Degradation**: If embeddings are disabled, the model is uninstalled, or an API call fails, search transparently falls back to FTS5.

### 1.2 Non-Goals

1. Requiring external vector database servers (e.g., Chroma, Qdrant, Milvus). Everything stays inside the local SQLite database.
2. Compiling CGO SQLite extensions (`sqlite-vec`, `sqlite-vss`). All similarity computation and streaming heap operations are implemented in pure Go.
3. Making embeddings authoritative. Like `cache/index.db`, vector embeddings are disposable and can be regenerated from the canonical Markdown notes, `memories` table, and `history.jsonl`.

---

## 2. SQLite Vector Storage & Indexing

### 2.1 Database Schema (`cache/index.db`)

New table in `games/<id>/cache/index.db`:

```sql
CREATE TABLE IF NOT EXISTS embeddings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_type TEXT NOT NULL,         -- 'entity' | 'memory' | 'turn'
    target_id TEXT NOT NULL,           -- entity id (string), memory id (int string), turn number (string)
    chunk_index INTEGER NOT NULL DEFAULT 0,
    content_hash TEXT NOT NULL,        -- SHA256 of the embedded source text
    model_id TEXT NOT NULL,            -- e.g. 'text-embedding-3-small', 'bge-small-en-v1.5'
    dimensions INTEGER NOT NULL,       -- e.g. 384, 1536
    vector BLOB NOT NULL,              -- little-endian []float32 binary representation
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(target_type, target_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_embeddings_target ON embeddings(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_model_type ON embeddings(model_id, target_type);
```

### 2.2 Memory-Bounded Vector Matcher ($O(K)$ Min-Heap)

To ensure long-running campaigns with 10,000+ turns and memories never exhaust memory:
1. Embeddings are stored as raw binary blobs (`[]float32` encoded via `binary.LittleEndian`).
2. Vector search executes a streaming cursor across matching rows in SQLite (`SELECT target_type, target_id, vector FROM embeddings WHERE model_id = ? AND target_type IN (...)`).
3. Rows are scanned in chunks (e.g. 500 at a time).
4. For each vector, cosine similarity with the query vector is computed in pure Go using unrolled SIMD-friendly dot products.
5. A priority min-heap of maximum size $K$ (e.g. $K=10$ or $K=20$) keeps only the top $K$ items.
6. Peak memory consumption is strictly $O(K)$ (< 1 MB RAM), completely independent of total vector count.
7. Scan performance: Scanning and dot-producting 15,000 vectors takes ~15–25ms in Go.

---

## 3. Provider Architecture

### 3.1 Uniform Interface (`pkg/embeddings/provider.go`)

```go
package embeddings

import "context"

type Provider interface {
    ID() string
    Dimensions() int
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

### 3.2 Provider Implementations

1. **`pkg/provider/openaiembedding`**:
   - HTTP POST to standard `/v1/embeddings` (`model`, `input`).
   - Supports OpenAI (`text-embedding-3-small`, `text-embedding-3-large`), Ollama (`http://localhost:11434/v1`), LM Studio, vLLM.
2. **`pkg/provider/geminiembedding`**:
   - Calls Google GenAI embedding API (`text-embedding-004`).
3. **`pkg/provider/onnxembedding`**:
   - Uses `sherpa-onnx` embedding extractor bindings.
   - Pinned default model: `bge-small-en-v1.5` (384 dimensions, compact ~70MB ONNX package).
   - Weights downloaded and managed via `models.Manager` (`pkg/models/manager.go`), supporting offline local play.
4. **`pkg/provider/mockembedding`**:
   - In-memory mock for unit tests.

### 3.3 Configuration (`pkg/config/types.go`)

```yaml
embeddings:
  enabled: true
  provider: "builtin-local" # "builtin-local", "openai", "gemini", "ollama", "disabled"
  model: "bge-small-en-v1.5"
  dimensions: 384
  batch_size: 16
  providers:
    builtin-local:
      type: "builtin"
      builtin_name: "sherpa-onnx-embedding"
    openai:
      type: "http"
      api_key: "${OPENAI_API_KEY}"
      url: "https://api.openai.com/v1"
      model: "text-embedding-3-small"
```

---

## 4. Hybrid Search & Indexing Lifecycle

### 4.1 Asynchronous Background Indexer (`pkg/storage/embedding_worker.go`)

- **Non-Blocking**: Turn recording (`Timeline.RecordTurn`) and file sync (`storage.Sync`) stream responses to the player without waiting for vector embeddings.
- **Worker Queue**: New/updated turns, memories, and entity Markdown notes are pushed to a buffered channel.
- **Content Hashing**: If `content_hash` matches an existing row with the active `model_id`, computation is skipped.
- **Batching**: The worker collects up to `batch_size` items (e.g. 16) or flushes after 200ms idle, minimizing HTTP or ONNX overhead.
- **Transaction Batching**: Generated vectors are upserted into SQLite within a single transaction.

### 4.2 Reciprocal Rank Fusion (RRF) Hybrid Search

When an agent tool (`search_entities`, `search_memories`, `search_timeline`) runs:
1. **Keyword Retrieval**: Execute FTS5 query to get top candidate documents ranked by BM25.
2. **Vector Retrieval**: Embed query text and scan SQLite vectors for top candidate documents ranked by cosine similarity.
3. **Score Combination**:
   $$\text{Score}(d) = \frac{0.5}{60 + \text{rank}_{\text{fts}}(d)} + \frac{0.5}{60 + \text{rank}_{\text{vec}}(d)}$$
4. **Fallback**: If the embedding provider is disabled or fails, the tool cleanly returns FTS5 keyword results.

---

## 5. Telemetry & Verification

1. **OpenTelemetry Spans**:
   - `embeddings.embed`: records text batch count, duration, token usage, model ID.
   - `embeddings.search`: records scan duration, candidate count, RRF scores.
2. **Testing**:
   - Unit tests for streaming min-heap cosine matcher.
   - Unit tests for RRF score fusion with edge cases (empty vector results, empty FTS results).
   - Mock provider tests for background worker batching and hash deduplication.
   - Regression tests ensuring campaigns without embeddings continue to work with pure FTS5.
