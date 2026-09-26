package engine

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// TestWriteEntitiesIndexesWrittenEntities guards the correctness the sync change
// must keep: every entity the turn writes is present in the index afterwards.
func TestWriteEntitiesIndexesWrittenEntities(t *testing.T) {
	paths := writeTestCampaignScaffold(t, t.TempDir(), nil)
	if _, err := InitGame(paths, InitOptions{
		GameID:     "campaign-01",
		SystemID:   "d20-test",
		WorldID:    "fantasy-realm",
		PlayerName: "Sean",
	}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.OpenGameStore(paths, "campaign-01")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	timeline := NewTimeline(paths, store, NewHistoryLogger(filepath.Join(paths.GameDir("campaign-01"), "history.jsonl")), "campaign-01")
	pending := map[string]*entity.Entity{
		"mira": {ID: "mira", Name: "Mira", Type: "character", Body: "A scout.", Hash: "mira-hash"},
	}
	if err := timeline.writeEntities(pending); err != nil {
		t.Fatalf("writeEntities: %v", err)
	}

	loaded, err := store.GetEntity("mira")
	if err != nil || loaded == nil || loaded.Name != "Mira" {
		t.Fatalf("written entity not indexed: %+v (%v)", loaded, err)
	}
}
