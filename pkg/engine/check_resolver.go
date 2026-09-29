package engine

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
)

// defaultCheckResolver is used until the mechanics schema provides a resolver.
// It rolls the request's notation (or 2d6) and passes at total 8 or more.
type defaultCheckResolver struct{}

func (defaultCheckResolver) Resolve(_ context.Context, req harness.CheckRequest, _ *entity.Entity) (*harness.CheckResult, error) {
	notation := req.Notation
	if notation == "" {
		notation = "2d6"
	}
	roll, err := rules.EvaluateRoll(notation)
	if err != nil {
		return nil, fmt.Errorf("resolve check: %w", err)
	}
	outcome := "fail"
	if roll.Total >= 8 {
		outcome = "pass"
	}
	return &harness.CheckResult{
		CheckID: newCheckID(),
		Roll:    roll.Summary(roll.Total),
		Outcome: outcome,
	}, nil
}

func newCheckID() string { return "chk_" + strconv.FormatInt(time.Now().UnixNano(), 36) }
