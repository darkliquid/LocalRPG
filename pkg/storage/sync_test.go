package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDirectory(t *testing.T) {
	tempDir := t.TempDir()
	entitiesDir := filepath.Join(tempDir, "entities")
	dbPath := filepath.Join(tempDir, "index.db")

	if err := os.MkdirAll(entitiesDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	doc1 := `---
id: tavern
name: Alden Tavern
type: location
---
A rustic [[Tavern]] in [[Eldoria]].
`
	if err := os.WriteFile(filepath.Join(entitiesDir, "Tavern.md"), []byte(doc1), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	syncer := NewSyncer(store)
	result, err := syncer.Sync(entitiesDir)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if result.Added != 1 {
		t.Errorf("expected 1 added, got %d", result.Added)
	}

	// Sync again without changes - should be 1 unchanged
	result2, err := syncer.Sync(entitiesDir)
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if result2.Unchanged != 1 {
		t.Errorf("expected 1 unchanged, got %d", result2.Unchanged)
	}
}
