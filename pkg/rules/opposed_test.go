package rules

import (
	"context"
	"fmt"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/state"
)

// fakeBridge is a GameHostAPI with only GetEntity and GetStat answering, which
// is all the opposed path reads. The embedded interface is nil, so any other
// call panics rather than silently returning a zero.
type fakeBridge struct {
	GameHostAPI
	entities map[string]*entity.Entity
}

func (f fakeBridge) GetEntity(id string) (*entity.Entity, error) {
	if ent, ok := f.entities[id]; ok {
		return ent, nil
	}
	return nil, fmt.Errorf("unknown entity %q", id)
}

func (f fakeBridge) GetStat(entityID, path string) (interface{}, error) {
	ent, ok := f.entities[entityID]
	if !ok || ent.State == nil {
		return nil, fmt.Errorf("unknown entity %q", entityID)
	}
	raw, ok := ent.State.Get(path)
	if !ok {
		return nil, fmt.Errorf("unknown stat %q", path)
	}
	return raw, nil
}

func TestResolveOpposedHigherTotalWins(t *testing.T) {
	// 1d1 always rolls 1, so the opponent's total is 1 + its bonus.
	roll, total, actorWon, err := ResolveOpposed("1d1", 10, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || !actorWon {
		t.Fatalf("total = %d, actorWon = %v, want 2 and a win", total, actorWon)
	}
	if roll == nil || roll.Total != 2 || roll.Notation != "1d1" {
		t.Fatalf("roll = %+v", roll)
	}
}

func TestResolveOpposedTieFollowsTheRule(t *testing.T) {
	if _, _, actorWon, _ := ResolveOpposed("1d1", 1, 0, ""); !actorWon {
		t.Fatal("a tie must default to the actor")
	}
	if _, _, actorWon, _ := ResolveOpposed("1d1", 1, 0, core.TieActor); !actorWon {
		t.Fatal("ties: actor must keep the tie with the actor")
	}
	if _, _, actorWon, _ := ResolveOpposed("1d1", 1, 0, core.TieOpponent); actorWon {
		t.Fatal("ties: opponent must hand the tie to the opponent")
	}
}

// TestOpposedIsSymmetric guards the property the contest rests on: with both
// sides rolling the same notation, swapping who is the actor swaps the outcome.
func TestOpposedIsSymmetric(t *testing.T) {
	_, _, aWins, err := ResolveOpposed("1d1", 1+5, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, bWins, err := ResolveOpposed("1d1", 1+2, 5, "")
	if err != nil {
		t.Fatal(err)
	}
	if aWins == bWins {
		t.Fatalf("swapping the sides must swap the outcome: a=%v b=%v", aWins, bWins)
	}
}

func TestSchemaResolverOpposedSumsTheOpponentStat(t *testing.T) {
	actor := &entity.Entity{ID: "hero"}
	opponent := &entity.Entity{ID: "ogre", State: state.NewState(map[string]interface{}{"might": 4})}
	bridge := fakeBridge{entities: map[string]*entity.Entity{"ogre": opponent}}
	r := SchemaResolver{bridge: bridge, conventions: core.CheckConventions{
		Notation: "1d1",
		Profiles: map[string]core.ResolutionProfile{"grapple": {DC: 1}},
	}}
	actorTotal := 2
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Profile: "grapple", Target: "ogre", Opposed: "might", ForcedTotal: &actorTotal,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if res.OpposedTotal != 5 || res.OpposedActor != "ogre" || res.OpposedRoll == nil {
		t.Fatalf("opposed result = %+v", res)
	}
	// The opponent's 1 + 4 beats the actor's 2, so the profile's worse outcome.
	if res.Outcome != "fail" {
		t.Fatalf("outcome = %q, want fail", res.Outcome)
	}
}

func TestSchemaResolverOpposedTieFollowsTheProfile(t *testing.T) {
	actor := &entity.Entity{ID: "hero"}
	opponent := &entity.Entity{ID: "ogre", State: state.NewState(map[string]interface{}{"might": 0})}
	bridge := fakeBridge{entities: map[string]*entity.Entity{"ogre": opponent}}
	r := SchemaResolver{bridge: bridge, conventions: core.CheckConventions{
		Notation: "1d1",
		Profiles: map[string]core.ResolutionProfile{
			"grapple": {DC: 1, Opposed: "might", Ties: core.TieOpponent},
		},
	}}
	actorTotal := 1
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Profile: "grapple", Target: "ogre", ForcedTotal: &actorTotal,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if res.OpposedTotal != 1 || res.Outcome != "fail" {
		t.Fatalf("res = %+v, want a tie lost through the profile", res)
	}
}

func TestSchemaResolverOpposedUnknownOpponentIsFlat(t *testing.T) {
	actor := &entity.Entity{ID: "hero"}
	r := SchemaResolver{conventions: core.CheckConventions{
		Notation: "1d1",
		Profiles: map[string]core.ResolutionProfile{"grapple": {DC: 1}},
	}}
	actorTotal := 5
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Profile: "grapple", Target: "nobody", Opposed: "might", ForcedTotal: &actorTotal,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	if res.OpposedTotal != 1 || res.Outcome != "success" {
		t.Fatalf("res = %+v, want a flat opponent beaten", res)
	}
}

func TestSchemaResolverOpposedMapsALadder(t *testing.T) {
	actor := &entity.Entity{ID: "hero"}
	r := SchemaResolver{conventions: core.CheckConventions{
		Notation: "1d1",
		Profiles: map[string]core.ResolutionProfile{
			"pbta": {Opposed: "might", Ladder: []core.LadderStep{
				{Min: 10, Outcome: "strong"}, {Min: 7, Outcome: "weak"}, {Min: 0, Outcome: "miss"},
			}},
		},
	}}
	actorTotal := 5
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Profile: "pbta", Target: "ogre", ForcedTotal: &actorTotal,
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	// A win takes the ladder's best outcome, a loss its worst.
	if res.Outcome != "strong" {
		t.Fatalf("outcome = %q, want the ladder's best on a win", res.Outcome)
	}
}

// TestSchemaResolverNonOpposedUnchanged guards the fixed path: a request that
// sets a target but no opposed stat resolves as it did before opposed checks.
func TestSchemaResolverNonOpposedUnchanged(t *testing.T) {
	r := SchemaResolver{conventions: core.CheckConventions{
		Notation:   "1d1",
		Outcome:    []string{"pass", "fail"},
		Difficulty: []core.DifficultySpec{{ID: "trivial", Target: 1}},
	}}
	res, err := r.Resolve(context.Background(), harness.CheckRequest{
		Target: "ogre", Difficulty: "trivial",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != "pass" || res.OpposedRoll != nil || res.OpposedTotal != 0 {
		t.Fatalf("res = %+v, want the fixed path", res)
	}
}
