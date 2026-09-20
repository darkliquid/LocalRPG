package harness

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestContextAssembler(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// 1. Scene location
	tavern := &entity.Entity{
		ID:        "alden-tavern",
		Name:      "Alden Tavern",
		Type:      "location",
		Body:      "A warm tavern smelling of ale.",
		Wikilinks: []string{"arc-siege", "lady-evelyn"},
		Hash:      "hash-tavern",
	}
	store.SaveEntity(tavern)

	// 2. Active NPC
	npc := &entity.Entity{
		ID:       "lady-evelyn",
		Name:     "Lady Evelyn",
		Type:     "character",
		Location: "[[alden-tavern]]",
		Body:     "Guarded former lieutenant.",
		Hash:     "hash-evelyn",
	}
	store.SaveEntity(npc)

	// 3. Living World Arc
	arc := &entity.Entity{
		ID:   "arc-siege",
		Name: "The Iron Siege",
		Type: "arc",
		Body: "Food supplies are depleted in the lower quarter.",
		Hash: "hash-arc",
	}
	store.SaveEntity(arc)

	assembler := NewContextAssembler(store)
	ctxPrompt, err := assembler.AssembleContext("alden-tavern", "player", "I speak with Evelyn")
	if err != nil {
		t.Fatalf("AssembleContext failed: %v", err)
	}

	// Verify all layers are assembled
	if !strings.Contains(ctxPrompt, "Alden Tavern") {
		t.Errorf("missing location in context")
	}
	if !strings.Contains(ctxPrompt, "Lady Evelyn") {
		t.Errorf("missing NPC in context")
	}
	if !strings.Contains(ctxPrompt, "The Iron Siege") {
		t.Errorf("missing living world arc in context")
	}
}
