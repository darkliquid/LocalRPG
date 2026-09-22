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

func TestListEntities(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	tavern := &entity.Entity{
		ID:       "alden-tavern",
		Name:     "Alden Tavern",
		Type:     "location",
		Body:     "A warm tavern.",
		Tags:     []string{"tavern", "safehouse"},
		Location: "[[Eldoria]]",
		Hash:     "hash-tavern",
	}
	if err := store.SaveEntity(tavern); err != nil {
		t.Fatalf("SaveEntity failed: %v", err)
	}

	summaries, err := store.ListEntities()
	if err != nil {
		t.Fatalf("ListEntities failed: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	got := summaries[0]
	if got.ID != tavern.ID || got.Name != tavern.Name || got.Type != tavern.Type {
		t.Errorf("summary mismatch: %+v", got)
	}
	if got.Location != tavern.Location {
		t.Errorf("expected location %q, got %q", tavern.Location, got.Location)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "tavern" {
		t.Errorf("expected tags from frontmatter, got %+v", got.Tags)
	}
}

func TestListEntitiesCarriesAliases(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SaveEntity(&entity.Entity{
		ID: "guard-kael", Name: "Guard Kael", Type: "character", Body: "A warden.",
		Aliases: []string{"The Ember Warden"}, Hash: "h1",
	}); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.ListEntities()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	if len(summaries[0].Aliases) != 1 || summaries[0].Aliases[0] != "The Ember Warden" {
		t.Errorf("Aliases = %v, want the note's alias", summaries[0].Aliases)
	}
}
