package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/state"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// TestDefaultResolverOpposedReadsTheOpponent guards the engine's own resolver,
// which has no host bridge: it reads the opponent from the campaign index, so an
// opposed check works without mechanics.js.
func TestDefaultResolverOpposedReadsTheOpponent(t *testing.T) {
	store := newTestStore(t)
	dir := t.TempDir()
	writeTestEntityNote(t, dir, &entity.Entity{
		ID: "ogre", Name: "Ogre", Type: "character", Body: "Big.",
		State: state.NewState(map[string]interface{}{"might": 4}),
	})
	if _, err := storage.NewSyncer(store).Sync(dir); err != nil {
		t.Fatal(err)
	}

	resolver := defaultCheckResolver{
		mechanics: &core.MechanicsSpec{Checks: core.CheckConventions{
			Profiles: map[string]core.ResolutionProfile{"grapple": {Notation: "1d1", DC: 1}},
		}},
		store: store,
	}
	actorTotal := 2
	res, err := resolver.Resolve(context.Background(), harness.CheckRequest{
		Profile: "grapple", Target: "ogre", Opposed: "might", ForcedTotal: &actorTotal,
	}, &entity.Entity{ID: "player"})
	if err != nil {
		t.Fatal(err)
	}
	// 1d1 is 1, so the opponent's total is 1 + its Might, which beats the actor.
	if res.OpposedTotal != 5 || res.OpposedActor != "ogre" {
		t.Fatalf("opposed = %+v", res)
	}
	if res.Outcome != "fail" {
		t.Fatalf("outcome = %q, want fail", res.Outcome)
	}
}

func TestDefaultResolverOpposedUnknownOpponentIsFlat(t *testing.T) {
	resolver := defaultCheckResolver{
		mechanics: &core.MechanicsSpec{Checks: core.CheckConventions{
			Profiles: map[string]core.ResolutionProfile{"grapple": {Notation: "1d1", DC: 1}},
		}},
		store: newTestStore(t),
	}
	actorTotal := 5
	res, err := resolver.Resolve(context.Background(), harness.CheckRequest{
		Profile: "grapple", Target: "nobody", Opposed: "might", ForcedTotal: &actorTotal,
	}, &entity.Entity{ID: "player"})
	if err != nil {
		t.Fatal(err)
	}
	if res.OpposedTotal != 1 || res.Outcome != "success" {
		t.Fatalf("res = %+v, want a flat opponent beaten", res)
	}
}
