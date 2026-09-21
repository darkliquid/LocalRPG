package storage

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestPoolSharesStorePerPath(t *testing.T) {
	pool := NewPool()
	dir := t.TempDir()

	first, err := pool.Store(filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatalf("Pool.Store failed: %v", err)
	}

	again, err := pool.Store(filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatalf("Pool.Store failed: %v", err)
	}
	if first != again {
		t.Errorf("expected the same store for the same path")
	}

	other, err := pool.Store(filepath.Join(dir, "b.db"))
	if err != nil {
		t.Fatalf("Pool.Store failed: %v", err)
	}
	if other == first {
		t.Errorf("expected a distinct store per path")
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("Pool.Close failed: %v", err)
	}
}

func TestPooledStoreIgnoresClose(t *testing.T) {
	pool := NewPool()
	store, err := pool.Store(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("Pool.Store failed: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("closing a pooled store failed: %v", err)
	}

	ent := &entity.Entity{ID: "hero", Name: "Hero", Type: "character", Hash: "hash-hero"}
	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("a pooled store must stay usable after a deferred Close: %v", err)
	}
	if _, err := store.GetEntity("hero"); err != nil {
		t.Fatalf("pooled store lookup failed: %v", err)
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("Pool.Close failed: %v", err)
	}
}
