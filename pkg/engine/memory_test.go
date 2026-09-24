package engine

import (
	"fmt"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestWorkingSetDecaysAndSelects(t *testing.T) {
	set := WorkingSet{}
	set.Apply(1, []harness.Ref{{Kind: harness.RefEntity, ID: "kaelen", Relation: "present"}})
	set.Apply(5, []harness.Ref{{Kind: harness.RefEntity, ID: "elena", Relation: "action"}})

	top := set.Select(1)
	if len(top) != 1 || top[0].ID != "elena" {
		t.Fatalf("expected elena to outrank the decayed kaelen, got %+v", top)
	}
}

func TestWorkingSetEvictsPastAgeCap(t *testing.T) {
	set := WorkingSet{}
	set.Apply(1, []harness.Ref{{Kind: harness.RefEntity, ID: "ancient", Relation: "present"}})
	// Turn 1 + 21 = Turn 22, age is 21 > 20
	set.Apply(22, []harness.Ref{{Kind: harness.RefEntity, ID: "fresh", Relation: "action"}})

	for _, e := range set.Entries {
		if e.ID == "ancient" {
			t.Fatalf("expected ancient to be evicted after exceeding MaxAgeTurns")
		}
	}
}

func TestWorkingSetTiesBreakByLastTurnThenID(t *testing.T) {
	set := WorkingSet{}
	// Same weight (1.0), different last turn
	set.Apply(1, []harness.Ref{{Kind: harness.RefEntity, ID: "beta", Relation: "present"}})
	set.Apply(2, []harness.Ref{{Kind: harness.RefEntity, ID: "alpha", Relation: "present"}})
	// Now give them both a boost so their weights are equal
	set.Entries[0].Weight = 1.0
	set.Entries[1].Weight = 1.0

	// alpha has LastTurn 2, beta has LastTurn 1
	top := set.Select(2)
	if len(top) != 2 || top[0].ID != "alpha" {
		t.Fatalf("expected newer turn alpha to beat beta, got %+v", top)
	}

	// Now make LastTurn equal
	set.Entries[0].LastTurn = 2
	set.Entries[1].LastTurn = 2
	set.Apply(2, nil) // trigger re-sort without modifying weights
	top = set.Select(2)
	if top[0].ID != "alpha" || top[1].ID != "beta" {
		t.Fatalf("expected alphabetical tie breaker, got %+v", top)
	}
}

func TestWorkingSetRederive(t *testing.T) {
	turns := []Turn{
		{
			Number: 1,
			Entities: []entity.Mention{
				{ID: "kaelen", Kind: "present"},
			},
		},
		{
			Number: 2,
			Context: &harness.TurnContext{
				Refs: []harness.Ref{
					{Kind: harness.RefEntity, ID: "elena", Relation: "action"},
				},
			},
		},
	}

	set := WorkingSet{}
	rederived := set.Rederive(turns)
	if len(rederived.Entries) != 2 {
		t.Fatalf("expected 2 rederived entries, got %d", len(rederived.Entries))
	}
	top := rederived.Select(1)
	if len(top) != 1 || top[0].ID != "elena" {
		t.Fatalf("expected elena to be top entry, got %+v", top)
	}
}

func TestWorkingSetCapsMaxEntries(t *testing.T) {
	set := WorkingSet{}
	var refs []harness.Ref
	for i := 0; i < 40; i++ {
		refs = append(refs, harness.Ref{Kind: harness.RefEntity, ID: fmt.Sprintf("ent-%02d", i)})
	}
	set.Apply(1, refs)
	if len(set.Entries) != DefaultMaxEntries {
		t.Fatalf("expected capped at %d entries, got %d", DefaultMaxEntries, len(set.Entries))
	}
}
