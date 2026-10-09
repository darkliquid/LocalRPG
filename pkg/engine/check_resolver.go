package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// defaultCheckResolver is used until the mechanics schema provides a resolver.
// It rolls the request's notation (or 2d6) and passes at total 8 or more. When
// the system declares resolution profiles, it resolves through the named profile
// instead, so the default path and the schema resolver cannot drift. A store, when
// present, lets an opposed check read the opponent's stat.
type defaultCheckResolver struct {
	mechanics *core.MechanicsSpec
	store     *storage.Store
}

func (r defaultCheckResolver) Resolve(_ context.Context, req harness.CheckRequest, actor *entity.Entity) (*harness.CheckResult, error) {
	profile := req.Profile
	if profile == "" {
		profile = "default"
	}
	var chosen core.ResolutionProfile
	hasProfile := false
	if r.mechanics != nil {
		chosen, hasProfile = r.mechanics.Checks.Profiles[profile]
	}

	notation := req.Notation
	if notation == "" && hasProfile {
		notation = chosen.Notation
	}
	if notation == "" {
		notation = "2d6"
	}
	if hasProfile {
		notation = rules.ProfileNotation(notation, chosen)
	}
	roll, err := rules.EvaluateRoll(notation)
	if err != nil {
		return nil, fmt.Errorf("resolve check: %w", err)
	}

	// A manually entered roll replaces the dice and keeps the bonuses: a player
	// enters the dice, and the system still adds what it would have added.
	manual := len(req.ForcedDice) > 0 || req.ForcedTotal != nil
	switch {
	case len(req.ForcedDice) > 0:
		roll = rules.ManualRoll(notation, req.ForcedDice)
	case req.ForcedTotal != nil:
		roll = rules.ManualRoll(notation, []int{*req.ForcedTotal})
	}

	bonus, applied := rules.SumBonuses(func(name string) (int, bool) {
		return engineStateValue(actor, name)
	}, req)
	total := roll.Total + bonus

	res := &harness.CheckResult{
		CheckID: harness.NewCheckID(),
		Roll:    roll.Summary(total),
		Applied: applied,
	}
	if manual {
		res.Source = "manual"
	}

	// An opposed check rolls the opponent with the same notation and compares the
	// totals; everything after this maps a win or a loss like any other outcome.
	actorWon := true
	opposed := false
	if stat := rules.OpposedStat(req, chosen, hasProfile); stat != "" && req.Target != "" {
		oppRoll, oppTotal, won, err := rules.ResolveOpposed(
			notation, total, r.opponentBonus(req.Target, stat), rules.ProfileTies(chosen, hasProfile))
		if err != nil {
			return nil, err
		}
		res.OpposedRoll, res.OpposedTotal, res.OpposedActor = oppRoll, oppTotal, req.Target
		actorWon, opposed = won, true
	}

	if hasProfile {
		outcome, decided := "", false
		if opposed {
			outcome, decided = rules.ProfileOpposedOutcome(chosen, actorWon)
		} else {
			outcome, decided = rules.ResolveProfile(chosen, total, roll.Successes)
		}
		if decided {
			res.Outcome = outcome
			res.Profile = profile
		}
		res.Position = rules.ClampTo(chosen.Position, req.Position)
		res.Effect = rules.ClampTo(chosen.Effect, req.Effect)
		res.Successes = roll.Successes
	}
	if res.Outcome == "" {
		if opposed {
			res.Outcome = rules.OutcomeFor(nil, actorWon)
			return res, nil
		}
		if total >= 8 {
			res.Outcome = "pass"
		} else {
			res.Outcome = "fail"
		}
	}
	return res, nil
}

// opponentBonus reads the opponent's governing stat from the campaign index. An
// unknown entity or stat contributes nothing, so a mistyped target degrades to a
// flat opponent roll rather than failing the check.
func (r defaultCheckResolver) opponentBonus(target, stat string) int {
	if r.store == nil || stat == "" {
		return 0
	}
	opponent := findEntityByRef(r.store, target)
	if opponent == nil {
		return 0
	}
	bonus, _ := engineStateValue(opponent, stat)
	return bonus
}

// findEntityByRef resolves an entity reference: an id, a slugified name, or a
// wikilink, so a check's target can be named the way the fiction names it.
func findEntityByRef(store *storage.Store, ref string) *entity.Entity {
	if store == nil || strings.TrimSpace(ref) == "" {
		return nil
	}
	for _, candidate := range normalizeRefCandidates(ref) {
		if ent, err := store.GetEntity(candidate); err == nil && ent != nil {
			return ent
		}
	}
	summaries, err := store.ListEntities()
	if err != nil {
		return nil
	}
	for _, summary := range summaries {
		nameKey := entity.Slugify(summary.Name)
		for _, candidate := range normalizeRefCandidates(ref) {
			if candidate != nameKey {
				continue
			}
			if ent, err := store.GetEntity(summary.ID); err == nil && ent != nil {
				return ent
			}
		}
	}
	return nil
}

// engineStateValue reads a numeric stat or skill from the actor's state, so a
// system with no onCheck still honours a named value.
func engineStateValue(actor *entity.Entity, name string) (int, bool) {
	if actor == nil || actor.State == nil {
		return 0, false
	}
	raw, ok := actor.State.Get(name)
	if !ok {
		return 0, false
	}
	switch typed := raw.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	}
	return 0, false
}
