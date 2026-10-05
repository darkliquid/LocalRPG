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

func TestSaveEntityRecordsFolder(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	ent := &entity.Entity{
		ID:     "silver-hand",
		Name:   "Silver Hand",
		Type:   "faction",
		Folder: "factions/orders",
		Body:   "A guild of smiths.\n",
	}
	if err := store.SaveEntity(ent); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}

	got, err := store.GetEntity("silver-hand")
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if got.Folder != "factions/orders" {
		t.Errorf("Folder = %q, want %q", got.Folder, "factions/orders")
	}
}

func TestSaveEntityRootFolderIsEmpty(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.SaveEntity(&entity.Entity{
		ID: "loose-note", Name: "Loose Note", Type: "item", Body: "x\n",
	}); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}

	got, err := store.GetEntity("loose-note")
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if got.Folder != "" {
		t.Errorf("Folder = %q, want the empty root folder", got.Folder)
	}
}

func TestListEntitiesCarriesFolder(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.SaveEntity(&entity.Entity{
		ID: "ashen-order", Name: "Ashen Order", Type: "faction",
		Folder: "factions/orders", Body: "Sworn to the flame.\n",
	}); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}

	summaries, err := store.ListEntities()
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	if summaries[0].Folder != "factions/orders" {
		t.Errorf("Folder = %q, want %q", summaries[0].Folder, "factions/orders")
	}
}

func TestResetDerivedStateClearsTimelineAndKeepsDurableRecords(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	if err := store.SaveTurn(TurnRecord{Number: 1, Mode: "Do", Input: "look"}); err != nil {
		t.Fatalf("SaveTurn: %v", err)
	}
	if err := store.SaveEntity(&entity.Entity{ID: "sean", Name: "Sean", Type: "character"}); err != nil {
		t.Fatalf("SaveEntity: %v", err)
	}
	if _, err := store.SaveMemory(&entity.Memory{
		Turn: 1, Kind: entity.MemoryEvent, Text: "met a sailor",
		EntityRefs: []string{"sean"}, Importance: 3, Source: entity.SourceGM,
	}); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	if err := store.SaveUsage(UsageRecord{GameID: "campaign-01", Role: "gm", Provider: "echo", Requests: 1}); err != nil {
		t.Fatalf("SaveUsage: %v", err)
	}
	if err := store.UpsertTTSJob(TTSJob{ID: "job-1", GameID: "campaign-01", Provider: "native-os", Status: "done"}); err != nil {
		t.Fatalf("UpsertTTSJob: %v", err)
	}

	if err := store.ResetDerivedState(); err != nil {
		t.Fatalf("ResetDerivedState: %v", err)
	}

	turns, err := store.CountTurns()
	if err != nil {
		t.Fatalf("CountTurns: %v", err)
	}
	if turns != 0 {
		t.Errorf("CountTurns = %d, want 0", turns)
	}
	if summaries, err := store.ListEntities(); err != nil || len(summaries) != 0 {
		t.Errorf("ListEntities = %d (err %v), want 0", len(summaries), err)
	}
	if hits, err := store.SearchMemories("sailor", "", "", 0, 10); err != nil || len(hits) != 0 {
		t.Errorf("SearchMemories = %d (err %v), want 0", len(hits), err)
	}
	if rows, err := store.UsageByGame("campaign-01"); err != nil || len(rows) != 1 {
		t.Errorf("UsageByGame = %d (err %v), want the ledger preserved", len(rows), err)
	}
	if jobs, err := store.ListTTSJobs("campaign-01"); err != nil || len(jobs) != 1 {
		t.Errorf("ListTTSJobs = %d (err %v), want job tracking preserved", len(jobs), err)
	}
}
