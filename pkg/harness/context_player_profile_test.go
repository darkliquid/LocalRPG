package harness

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// TestContextIncludesProtagonistProfile checks that the narrator is told who the
// protagonist is, not just their name, so age, gender and pronouns stay
// consistent instead of being reinvented each turn.
func TestContextIncludesProtagonistProfile(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	tavern := &entity.Entity{
		ID:        "alden-tavern",
		Name:      "Alden Tavern",
		Type:      "location",
		Body:      "A warm tavern smelling of ale.",
		Wikilinks: []string{"sean"},
		Hash:      "hash-tavern",
	}
	if err := store.SaveEntity(tavern); err != nil {
		t.Fatal(err)
	}

	player := &entity.Entity{
		ID:         "sean",
		Name:       "Sean",
		Type:       "character",
		Location:   "[[alden-tavern]]",
		Appearance: "A weathered sailor with a storm-grey coat.",
		Age:        "34",
		Gender:     "male",
		Body:       "Sean grew up on the docks.",
		ExtraMeta:  map[string]interface{}{"pronouns": "he/him"},
		Hash:       "hash-sean",
	}
	if err := store.SaveEntity(player); err != nil {
		t.Fatal(err)
	}

	assembler := NewContextAssembler(store)
	assembled, err := assembler.Assemble(ContextRequest{
		LocationID: "alden-tavern",
		PlayerID:   "sean",
		PlayerName: "Sean",
		Action:     "I look around the room.",
	})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	prompt := assembled.Prompt
	for _, want := range []string{"34", "male", "he/him", "A weathered sailor with a storm-grey coat."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("protagonist profile missing %q from context:\n%s", want, prompt)
		}
	}
}
