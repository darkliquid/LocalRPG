package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mustWriteNote writes a minimal, valid entity note.
func mustWriteNote(t *testing.T, path, id, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	body := fmt.Sprintf("---\nid: %s\nname: %s\ntype: character\n---\n\nA note.\n", id, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

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

func TestSyncWalksNestedFolders(t *testing.T) {
	tempDir := t.TempDir()
	entitiesDir := filepath.Join(tempDir, "entities")

	mustWriteNote(t, filepath.Join(entitiesDir, "silver-hand.md"), "silver-hand", "Silver Hand")
	mustWriteNote(t, filepath.Join(entitiesDir, "factions", "orders", "ashen-order.md"), "ashen-order", "Ashen Order")
	// Hidden directories and non-markdown files are not notes.
	mustWriteNote(t, filepath.Join(entitiesDir, ".obsidian", "hidden.md"), "hidden", "Hidden")
	mustWriteNote(t, filepath.Join(entitiesDir, "assets", "art.md"), "art", "Art")
	if err := os.WriteFile(filepath.Join(entitiesDir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write notes.txt: %v", err)
	}

	store, err := NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	res, err := NewSyncer(store).Sync(entitiesDir)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Added != 2 {
		t.Fatalf("Added = %d, want 2", res.Added)
	}

	root, err := store.GetEntity("silver-hand")
	if err != nil {
		t.Fatalf("GetEntity(silver-hand): %v", err)
	}
	if root.Folder != "" {
		t.Errorf("root Folder = %q, want the empty root", root.Folder)
	}

	nested, err := store.GetEntity("ashen-order")
	if err != nil {
		t.Fatalf("GetEntity(ashen-order): %v", err)
	}
	if nested.Folder != "factions/orders" {
		t.Errorf("nested Folder = %q, want %q", nested.Folder, "factions/orders")
	}

	if _, err := store.GetEntity("hidden"); err == nil {
		t.Error("a hidden directory must not be indexed")
	}
	if _, err := store.GetEntity("art"); err == nil {
		t.Error("assets/ must not be indexed")
	}
}

func TestSyncDetectsAFolderMove(t *testing.T) {
	tempDir := t.TempDir()
	entitiesDir := filepath.Join(tempDir, "entities")

	note := filepath.Join(entitiesDir, "port-vel.md")
	mustWriteNote(t, note, "port-vel", "Port Vel")

	store, err := NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	syncer := NewSyncer(store)
	if _, err := syncer.Sync(entitiesDir); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	// Moving the note changes no bytes, so only the folder tells the index the
	// note has moved.
	moved := filepath.Join(entitiesDir, "places", "port-vel.md")
	if err := os.MkdirAll(filepath.Dir(moved), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Rename(note, moved); err != nil {
		t.Fatalf("rename: %v", err)
	}

	res, err := syncer.Sync(entitiesDir)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("Updated = %d, want 1 (a move must be an update, not a no-op)", res.Updated)
	}

	got, err := store.GetEntity("port-vel")
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if got.Folder != "places" {
		t.Errorf("Folder = %q, want %q", got.Folder, "places")
	}
}

func TestSyncFileRecordsFolder(t *testing.T) {
	tempDir := t.TempDir()
	entitiesDir := filepath.Join(tempDir, "entities")

	path := filepath.Join(entitiesDir, "places", "port-vel.md")
	mustWriteNote(t, path, "port-vel", "Port Vel")

	store, err := NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := NewSyncer(store).SyncFile(path); err != nil {
		t.Fatalf("SyncFile: %v", err)
	}

	got, err := store.GetEntity("port-vel")
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if got.Folder != "places" {
		t.Errorf("Folder = %q, want %q", got.Folder, "places")
	}
}
