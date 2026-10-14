package rules

import (
	"context"
	"fmt"
	"strconv"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// SchemaResolver resolves a check from a system's declared conventions: it rolls
// the notation, adds the actor's governing stat, compares against the named
// difficulty, and maps to a declared outcome.
type SchemaResolver struct {
	bridge      GameHostAPI
	conventions core.CheckConventions
}

// Resolve implements harness.CheckResolver.
func (r SchemaResolver) Resolve(_ context.Context, req harness.CheckRequest, actor *entity.Entity) (*harness.CheckResult, error) {
	profile := req.Profile
	if profile == "" {
		profile = "default"
	}
	p, hasProfile := r.conventions.Profiles[profile]

	notation := r.resolveNotation(req, p, hasProfile)
	roll, manual, err := r.resolveRoll(notation, req)
	if err != nil {
		return nil, err
	}

	bonus, applied := SumBonuses(func(name string) (int, bool) {
		return stateValue(r.bridge, actor, name)
	}, req)
	total := roll.Total + bonus

	res := &harness.CheckResult{
		CheckID: harness.NewCheckID(),
		Actor:   req.Actor,
		Target:  req.Target,
		Roll:    roll.Summary(total),
		Applied: applied,
	}
	if manual {
		res.Source = "manual"
	}

	actorWon, opposed, err := r.checkOpposed(req, p, hasProfile, notation, total, res)
	if err != nil {
		return nil, err
	}

	r.resolveOutcome(res, req, p, hasProfile, profile, total, roll.Successes, actorWon, opposed)
	return res, nil
}

func (r SchemaResolver) resolveNotation(req harness.CheckRequest, p core.ResolutionProfile, hasProfile bool) string {
	notation := req.Notation
	if notation == "" && hasProfile {
		notation = p.Notation
	}
	if notation == "" {
		notation = r.conventions.Notation
	}
	if notation == "" {
		notation = "2d6"
	}
	if hasProfile {
		notation = ProfileNotation(notation, p)
	}
	return notation
}

func (r SchemaResolver) resolveRoll(notation string, req harness.CheckRequest) (*RollResult, bool, error) {
	roll, err := EvaluateRoll(notation)
	if err != nil {
		return nil, false, fmt.Errorf("resolve check: %w", err)
	}

	// A manually entered roll replaces the dice and keeps the bonuses: a player
	// enters the dice, and the system still adds what it would have added.
	manual := len(req.ForcedDice) > 0 || req.ForcedTotal != nil
	switch {
	case len(req.ForcedDice) > 0:
		roll = ManualRoll(notation, req.ForcedDice)
	case req.ForcedTotal != nil:
		roll = ManualRoll(notation, []int{*req.ForcedTotal})
	}
	return roll, manual, nil
}

func (r SchemaResolver) checkOpposed(req harness.CheckRequest, p core.ResolutionProfile, hasProfile bool, notation string, total int, res *harness.CheckResult) (bool, bool, error) {
	// An opposed check rolls the opponent with the same notation and compares the
	// totals; everything after this maps a win or a loss like any other outcome.
	if stat := OpposedStat(req, p, hasProfile); stat != "" && req.Target != "" {
		opponentBonus := r.opponentBonus(req.Target, stat)
		oppRoll, oppTotal, won, err := ResolveOpposed(notation, total, opponentBonus, ProfileTies(p, hasProfile))
		if err != nil {
			return false, false, err
		}
		res.OpposedRoll, res.OpposedTotal, res.OpposedActor = oppRoll, oppTotal, req.Target
		return won, true, nil
	}
	return true, false, nil
}

func (r SchemaResolver) resolveOutcome(res *harness.CheckResult, req harness.CheckRequest, p core.ResolutionProfile, hasProfile bool, profile string, total, successes int, actorWon, opposed bool) {
	if hasProfile {
		outcome, decided := "", false
		if opposed {
			outcome, decided = ProfileOpposedOutcome(p, actorWon)
		} else {
			outcome, decided = ResolveProfile(p, total, successes)
		}
		if decided {
			res.Outcome = outcome
			res.Profile = profile
		}
		res.Position = ClampTo(p.Position, req.Position)
		res.Effect = ClampTo(p.Effect, req.Effect)
		res.Successes = successes
	}

	if res.Outcome == "" {
		if opposed {
			res.Outcome = OutcomeFor(r.conventions.Outcome, actorWon)
			return
		}
		target := 8
		for _, difficulty := range r.conventions.Difficulty {
			if difficulty.ID == req.Difficulty {
				target = difficulty.Target
				break
			}
		}
		res.Outcome = OutcomeFor(r.conventions.Outcome, total >= target)
	}
}

// opponentBonus reads the opponent's governing stat through the bridge. An
// unknown entity or stat contributes nothing, so a mistyped target degrades to a
// flat opponent roll rather than failing the check.
func (r SchemaResolver) opponentBonus(target, stat string) int {
	if r.bridge == nil || target == "" || stat == "" {
		return 0
	}
	opponent, err := r.bridge.GetEntity(target)
	if err != nil || opponent == nil {
		return 0
	}
	bonus, _ := stateValue(r.bridge, opponent, stat)
	return bonus
}

// ProfileTies returns the profile's tie rule, or empty when there is no profile.
func ProfileTies(p core.ResolutionProfile, hasProfile bool) string {
	if !hasProfile {
		return ""
	}
	return p.Ties
}

// stateValue reads a numeric stat or skill from the actor or, failing that, the
// bridge. Skills are stored on entity state like stats, so one reader serves
// both.
func stateValue(bridge GameHostAPI, actor *entity.Entity, stat string) (int, bool) {
	if actor != nil && actor.State != nil {
		if raw, ok := actor.State.Get(stat); ok {
			if n, ok := toInt(raw); ok {
				return n, true
			}
		}
	}
	if bridge != nil && actor != nil {
		if raw, err := bridge.GetStat(actor.ID, stat); err == nil {
			if n, ok := toInt(raw); ok {
				return n, true
			}
		}
	}
	return 0, false
}

func toInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		if n, err := strconv.Atoi(typed); err == nil {
			return n, true
		}
	}
	return 0, false
}

// OutcomeFor picks the pass or fail key from a declared vocabulary, tolerating a
// system that names them differently or declares none.
func OutcomeFor(vocabulary []string, pass bool) string {
	if len(vocabulary) == 0 {
		if pass {
			return "pass"
		}
		return "fail"
	}
	if pass {
		for _, key := range vocabulary {
			if key == "pass" || key == "success" || key == "critical" {
				return key
			}
		}
		return vocabulary[0]
	}
	for _, key := range vocabulary {
		if key == "fail" || key == "failure" {
			return key
		}
	}
	return vocabulary[len(vocabulary)-1]
}
