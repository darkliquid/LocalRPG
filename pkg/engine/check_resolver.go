package engine

import (
	"context"
	"fmt"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
)

// defaultCheckResolver is used until the mechanics schema provides a resolver.
// It rolls the request's notation (or 2d6) and passes at total 8 or more.
type defaultCheckResolver struct{}

func (defaultCheckResolver) Resolve(_ context.Context, req harness.CheckRequest, actor *entity.Entity) (*harness.CheckResult, error) {
	notation := req.Notation
	if notation == "" {
		notation = "2d6"
	}
	roll, err := rules.EvaluateRoll(notation)
	if err != nil {
		return nil, fmt.Errorf("resolve check: %w", err)
	}
	bonus, applied := rules.SumBonuses(func(name string) (int, bool) {
		return engineStateValue(actor, name)
	}, req)
	total := roll.Total + bonus
	outcome := "fail"
	if total >= 8 {
		outcome = "pass"
	}
	return &harness.CheckResult{
		CheckID: harness.NewCheckID(),
		Roll:    roll.Summary(total),
		Outcome: outcome,
		Applied: applied,
	}, nil
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
