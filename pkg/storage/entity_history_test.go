package storage

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestStoreRoundTripsEntityHistory(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ent := &entity.Entity{ID: "garrick", Name: "Garrick", Type: "character", Hash: "hash-garrick", History: []int{2, 5}}
	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity failed: %v", err)
	}

	loaded, err := store.GetEntity("garrick")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(loaded.History) != 2 || loaded.History[0] != 2 || loaded.History[1] != 5 {
		t.Errorf("History = %v, want [2 5]", loaded.History)
	}
}
