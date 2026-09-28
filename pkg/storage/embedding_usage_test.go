package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/storage"
)

type recordingEmbedder struct{}

func (recordingEmbedder) ID() string      { return "recording" }
func (recordingEmbedder) Dimensions() int { return 2 }
func (recordingEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1, 0}}, nil
}

func TestEmbeddingWorkerReportsUsage(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(dir + "/index.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	var got []storage.EmbeddingUsage
	worker := storage.NewEmbeddingWorker(store, recordingEmbedder{}, storage.EmbeddingWorkerOptions{BatchSize: 1})
	worker.SetUsageReporting(
		func(u storage.EmbeddingUsage) { got = append(got, u) },
		storage.EmbeddingUsage{ProviderKey: "embedding:gemini@default", Model: "text-embedding-004"},
	)
	worker.Start()
	defer worker.Stop()

	worker.Enqueue(storage.EmbeddingItem{TargetType: "entity", TargetID: "x", Text: "hello world"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected a usage record after a batch")
	}
	if got[0].ProviderKey != "embedding:gemini@default" {
		t.Errorf("provider key = %q", got[0].ProviderKey)
	}
	if got[0].Requests != 1 {
		t.Errorf("requests = %d, want 1", got[0].Requests)
	}
}

func TestEmbeddingWorkerWithoutASinkRecordsNothing(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStore(dir + "/index.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	worker := storage.NewEmbeddingWorker(store, recordingEmbedder{}, storage.EmbeddingWorkerOptions{BatchSize: 1})
	worker.Start()
	defer worker.Stop()

	worker.Enqueue(storage.EmbeddingItem{TargetType: "entity", TargetID: "y", Text: "no sink"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := worker.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
}
