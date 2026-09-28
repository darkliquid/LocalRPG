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

	// usageFn and usageMeta are set by SetUsageReporting; nil means no usage is
	// recorded, which is the case for every existing caller.
	usageFn   EmbeddingUsageFunc
	usageMeta EmbeddingUsage
}

// SetUsageReporting installs the sink a completed batch reports to, plus the
// static key and model the worker cannot know. It is a no-op until called.
func (w *EmbeddingWorker) SetUsageReporting(fn EmbeddingUsageFunc, meta EmbeddingUsage) {
	w.usageFn = fn
	w.usageMeta = meta
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
func (w *EmbeddingWorker) Enqueue(item EmbeddingItem) {
	if item.Text == "" || item.TargetID == "" {
		return
	}
	select {
	case w.queue <- item:
	default:
		// Drop to avoid blocking if queue is congested
	}
}

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
		drainLoop:
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
					break drainLoop
				}
			}

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

	if w.usageFn != nil {
		usage := w.usageMeta
		usage.Requests = 1
		usage.InputTokens = reporterTokens(w.provider)
		w.usageFn(usage)
	}
}

// EmbeddingUsage is one embedding call's consumption, reported after a batch.
type EmbeddingUsage struct {
	ProviderKey string
	Model       string
	InputTokens int
	Requests    int
}

// EmbeddingUsageFunc receives a usage record for each completed batch.
type EmbeddingUsageFunc func(u EmbeddingUsage)

// usageReportingProvider is the opt-in interface an embedding provider
// implements to report token usage. pkg/storage cannot import pkg/harness, so
// the method set is matched structurally.
type usageReportingProvider interface {
	LastUsage() usageTokens
}

type usageTokens struct {
	InputTokens int
}

func reporterTokens(provider embeddings.Provider) int {
	reporter, ok := provider.(usageReportingProvider)
	if !ok {
		return 0
	}
	return reporter.LastUsage().InputTokens
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
