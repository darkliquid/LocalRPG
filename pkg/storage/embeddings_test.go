package storage

import (
	"context"
	"math"
	"path/filepath"
	"testing"
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
	vec2 := []float32{0.8, 0.6, 0.0}  // close to vec1
	vec3 := []float32{0.0, 1.0, 0.0}  // orthogonal to vec1
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
