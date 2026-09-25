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
