package rules

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type recordingWriter struct {
	saved []*entity.Entity
}

func (w *recordingWriter) SaveEntity(ent *entity.Entity) error {
	w.saved = append(w.saved, ent)
	return nil
}

func TestHostBridgeMovesThePlayerThroughItsWriter(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, ent := range []*entity.Entity{
		{ID: "alden-tavern", Name: "Alden Tavern", Type: "location", Hash: "h1"},
		{ID: "aldon-harbour", Name: "Aldon Harbour", Type: "location", Hash: "h2"},
		{ID: "player", Name: "Sean", Type: "character", Hash: "h3", Location: "[[alden-tavern]]"},
	} {
		if err := store.SaveEntity(ent); err != nil {
			t.Fatal(err)
		}
	}

	writer := &recordingWriter{}
	bridge := NewHostBridge(store, writer, "player")

	current, err := bridge.GetLocation()
	if err != nil {
		t.Fatalf("GetLocation failed: %v", err)
	}
	if current != "alden-tavern" {
		t.Errorf("GetLocation = %q, want alden-tavern", current)
	}

	if err := bridge.SetLocation("aldon-harbour"); err != nil {
		t.Fatalf("SetLocation failed: %v", err)
	}
	if len(writer.saved) != 1 || writer.saved[0].Location != "[[aldon-harbour]]" {
		t.Fatalf("expected the move to go through the writer, got %+v", writer.saved)
	}

	if err := bridge.SetLocation("nowhere"); err == nil {
		t.Errorf("expected an error for an unknown location")
	}
	if err := bridge.SetLocation("player"); err == nil {
		t.Errorf("expected an error when the target is not a location")
	}

	// State changes go through the writer too, so they reach the Markdown note.
	if err := bridge.SetStat("player", "hp", 12); err != nil {
		t.Fatalf("SetStat failed: %v", err)
	}
	if len(writer.saved) != 2 {
		t.Errorf("expected SetStat to persist through the writer, got %d saves", len(writer.saved))
	}
}
