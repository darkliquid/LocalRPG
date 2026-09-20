package storage

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestStorageOperations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "index.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ent := &entity.Entity{
		ID:        "alden-tavern",
		Name:      "Alden Tavern",
		Type:      "location",
		Body:      "A warm tavern.",
		Wikilinks: []string{"Eldoria"},
		Hash:      "hash-123",
	}
	ent.InitState(map[string]interface{}{"cozy": true})

	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity failed: %v", err)
	}

	loaded, err := store.GetEntity("alden-tavern")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if loaded.ID != ent.ID || loaded.Name != ent.Name {
		t.Errorf("loaded entity mismatch: %+v", loaded)
	}

	edges, err := store.GetEdgesFrom("alden-tavern")
	if err != nil {
		t.Fatalf("GetEdgesFrom failed: %v", err)
	}
	if len(edges) != 1 || edges[0].TargetID != "Eldoria" {
		t.Errorf("expected edge to Eldoria, got: %+v", edges)
	}
}
