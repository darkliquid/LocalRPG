package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestFilteredEntitiesByQueryAndType(t *testing.T) {
	appState = &State{
		Loaded: true,
		Entities: []gui.EntitySummaryDTO{
			{ID: "hero", Name: "Vance", Type: "character", Tags: []string{"player"}},
			{ID: "market", Name: "Old Market", Type: "location"},
			{ID: "guild", Name: "Thieves Guild", Type: "faction"},
		},
		EntityQuery: "vance",
		EntityType:  "all",
	}
	if got := filteredEntities(); len(got) != 1 || appState.Entities[got[0]].ID != "hero" {
		t.Fatalf("query filter = %v", got)
	}

	appState.EntityQuery = ""
	appState.EntityType = "location"
	if got := filteredEntities(); len(got) != 1 || appState.Entities[got[0]].ID != "market" {
		t.Fatalf("type filter = %v", got)
	}
}

func TestMergeTargetsExcludeSelf(t *testing.T) {
	appState = &State{
		Loaded: true,
		Entities: []gui.EntitySummaryDTO{
			{ID: "hero", Name: "Vance"},
			{ID: "vance-alias", Name: "The Traveller"},
		},
		Entity: &gui.EntityDTO{ID: "hero", Name: "Vance"},
	}
	targets := mergeTargets()
	if len(targets) != 1 || targets[0].ID != "vance-alias" {
		t.Fatalf("mergeTargets = %+v", targets)
	}
}
