package engine

import (
	"context"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
)

// defaultCheckResolver is used until the mechanics schema provides a resolver.
// It rolls the request's notation (or 2d6) and passes at total 8 or more. When
// the system declares resolution profiles, it resolves through the named profile
// instead, so the default path and the schema resolver cannot drift.
type defaultCheckResolver struct {
	mechanics *core.MechanicsSpec
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
	bonus, applied := rules.SumBonuses(func(name string) (int, bool) {
		return engineStateValue(actor, name)
	}, req)
	total := roll.Total + bonus
	if req.ForcedTotal != nil {
		total = *req.ForcedTotal
	}

	res := &harness.CheckResult{
		CheckID: harness.NewCheckID(),
		Roll:    roll.Summary(total),
		Applied: applied,
	}
	if req.ForcedTotal != nil {
		res.Source = "manual"
	}
	if hasProfile {
		if outcome, decided := rules.ResolveProfile(chosen, total, roll.Successes); decided {
			res.Outcome = outcome
			res.Profile = profile
		}
		res.Position = rules.ClampTo(chosen.Position, req.Position)
		res.Effect = rules.ClampTo(chosen.Effect, req.Effect)
		res.Successes = roll.Successes
	}
	if res.Outcome == "" {
		if total >= 8 {
			res.Outcome = "pass"
		} else {
			res.Outcome = "fail"
		}
	}
	return res, nil
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
