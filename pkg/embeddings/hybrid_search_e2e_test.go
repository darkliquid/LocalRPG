package embeddings_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/tools"
)

func TestHybridSearchEndToEnd(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	provider := embeddings.NewBuiltinHashProjectionProvider(384)
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

	// Concept query: "healing poultices herbs" - will match herbalist through vector and FTS
	call := harness.ToolCall{
		Name:      "search_entities",
		Arguments: `{"query": "healing poultices herbs"}`,
	}
	res, ok := exec.Execute(ctx, call)
	if !ok {
		t.Fatalf("search_entities failed: %s", res)
	}
	if !strings.Contains(res, "Willow Herbalist") {
		t.Errorf("expected Willow Herbalist in results, got: %s", res)
	}
}
