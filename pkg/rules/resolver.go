package rules

import (
	"context"
	"fmt"
	"strconv"
	"time"

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
	notation := req.Notation
	if notation == "" {
		notation = r.conventions.Notation
	}
	if notation == "" {
		notation = "2d6"
	}
	roll, err := EvaluateRoll(notation)
	if err != nil {
		return nil, fmt.Errorf("resolve check: %w", err)
	}

	total := roll.Total
	if req.Stat != "" {
		if bonus, ok := statValue(r.bridge, actor, req.Stat); ok {
			total += bonus
		}
	}

	target := 8
	for _, difficulty := range r.conventions.Difficulty {
		if difficulty.ID == req.Difficulty {
			target = difficulty.Target
			break
		}
	}

	outcome := outcomeFor(r.conventions.Outcome, total >= target)
	return &harness.CheckResult{
		CheckID: newCheckID(),
		Actor:   req.Actor,
		Target:  req.Target,
		Roll: &harness.RollSummary{
			Notation:  roll.Notation,
			Total:     total,
			Successes: roll.Successes,
			RollCount: roll.RollCount,
		},
		Outcome: outcome,
	}, nil
}

// statValue reads a numeric stat from the actor or, failing that, the bridge.
func statValue(bridge GameHostAPI, actor *entity.Entity, stat string) (int, bool) {
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

func toInt(value interface{}) (int, bool) {
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

// outcomeFor picks the pass or fail key from a declared vocabulary, tolerating a
// system that names them differently or declares none.
func outcomeFor(vocabulary []string, pass bool) string {
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

func newCheckID() string { return "chk_" + strconv.FormatInt(time.Now().UnixNano(), 36) }
