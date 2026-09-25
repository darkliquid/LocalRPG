package storage

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func openMemoryTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSaveAndListMemories(t *testing.T) {
	store := openMemoryTestStore(t)
	id, err := store.SaveMemory(&entity.Memory{Turn: 2, Kind: entity.MemoryEvent, EntityRefs: []string{"kae", "player"}, Text: "Crossed the rope bridge.", Importance: 3, Tags: []string{"travel"}, Source: entity.SourceGM})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if id == 0 {
		t.Fatal("SaveMemory returned id 0")
	}
	got, err := store.ListMemoriesForEntity("kae", 10)
	if err != nil {
		t.Fatalf("ListMemoriesForEntity: %v", err)
	}
	if len(got) != 1 || got[0].Text != "Crossed the rope bridge." {
		t.Fatalf("memories = %+v", got)
	}
	if len(got[0].EntityRefs) != 2 {
		t.Fatalf("entity refs = %v", got[0].EntityRefs)
	}
	if len(got[0].Tags) != 1 || got[0].Tags[0] != "travel" {
		t.Fatalf("tags = %v", got[0].Tags)
	}
}

func TestSaveMemoryValidates(t *testing.T) {
	store := openMemoryTestStore(t)
	if _, err := store.SaveMemory(&entity.Memory{Turn: 1, Kind: entity.MemoryEvent, Importance: 3}); err == nil {
		t.Fatal("a memory with no text or refs should be rejected")
	}
}
