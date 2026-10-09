package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// TestRouterWithChainsTriesTheChainInOrder is the end-to-end guard for a role
// chain: the config declares one, the GUI wires it, and the router tries each
// member and traces the selection. Disabled roles fail immediately, so the test
// needs no provider and no network.
func TestRouterWithChainsTriesTheChainInOrder(t *testing.T) {
	mem := trace.NewMemory(trace.LevelSummary)
	cfg := &config.Config{}
	cfg.Agents.Roles = map[string]config.AgentRoleConfig{
		"gm":     {Type: "disabled", Chain: []string{"gm", "backup"}, Select: config.SelectFirst},
		"backup": {Type: "disabled"},
	}

	router, err := routerWithChains(cfg, mem)
	if err != nil {
		t.Fatalf("build router: %v", err)
	}

	_, err = router.GenerateForRole(context.Background(), "gm", harness.GenerateRequest{Prompt: "hello"})
	failure, ok := harness.FailureFrom(err)
	if !ok {
		t.Fatalf("expected a GenerationFailure, got %v", err)
	}
	if len(failure.Attempts) != 2 {
		t.Fatalf("expected one attempt per chain member, got %d (%+v)", len(failure.Attempts), failure.Attempts)
	}
	if _, ok := mem.Find("router.select"); !ok {
		t.Fatalf("router.select was not traced: %v", mem.Names())
	}
}
