package harness

import (
	"context"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// CheckResolver resolves a CheckRequest into a CheckResult. It lives in harness
// so pkg/rules can implement it without importing pkg/engine, which imports
// pkg/rules.
type CheckResolver interface {
	Resolve(ctx context.Context, req CheckRequest, actor *entity.Entity) (*CheckResult, error)
}
