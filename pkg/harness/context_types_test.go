package harness

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func newTestAssembler(t *testing.T) (*ContextAssembler, *storage.Store) {
	t.Helper()
	store := newTestEntityStore(t)
	saveEntity(t, store, &entity.Entity{
		ID:        "aldon-harbour",
		Name:      "Aldon Harbour",
		Type:      "location",
		Body:      "A bustling harbour with salt-stained docks.",
		Wikilinks: []string{"kaelen"},
	})
	saveEntity(t, store, &entity.Entity{
		ID:   "player-elena",
		Name: "Elena",
		Type: "character",
		Body: "A cunning rogue.",
	})
	saveEntity(t, store, &entity.Entity{
		ID:       "kaelen",
		Name:     "Captain Kaelen",
		Type:     "character",
		Location: "[[aldon-harbour]]",
		Body:     "A stern harbourmaster.",
	})
	return NewContextAssembler(store), store
}

func TestAssembleRecordsSectionRefs(t *testing.T) {
	assembler, _ := newTestAssembler(t)
	result, err := assembler.Assemble(ContextRequest{
		LocationID: "aldon-harbour",
		PlayerID:   "player-elena",
		Action:     "look around",
		TurnNumber: 3,
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	var canon SectionReport
	for _, section := range result.Context.Sections {
		if section.Name == "canon" {
			canon = section
		}
	}
	if canon.Name == "" {
		t.Fatalf("expected a canon section, got %+v", result.Context.Sections)
	}
	found := false
	for _, ref := range canon.Refs {
		if ref.Kind == RefEntity && ref.ID == "aldon-harbour" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the location ref in canon, got %+v", canon.Refs)
	}
	if result.Context.PromptHash == "" || result.Context.TurnNumber != 3 {
		t.Fatalf("incomplete context: %+v", result.Context)
	}
}
