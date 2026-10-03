package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// runLoopForTest runs one generation loop against a bare assembled context, so a
// test can assert what the loop resolved without a full turn.
func runLoopForTest(o *TurnOrchestrator) (streamResult, error) {
	o.logger = trace.Nop()
	return o.runGenerationLoop(context.Background(), &harness.AssembleResult{Prompt: "context"}, "", nil, nil, "auto", nil)
}

// rulesResolverEngine builds a JSEngine whose onCheck resolver hardcodes a
// critical outcome, so a check resolved through it is distinguishable from the
// deterministic default.
func rulesResolverEngine(t *testing.T, store *storage.Store) *rules.JSEngine {
	t.Helper()
	bridge := rules.NewHostBridge(store, nil, "player")
	bridge.SetManifest(&core.SystemManifest{Mechanics: &core.MechanicsSpec{
		Checks: core.CheckConventions{Notation: "1d6"},
	}})
	engine := rules.NewJSEngine(bridge)
	if err := engine.LoadScript(`onCheck("luck", function(req){ return {outcome:"critical"}; });`); err != nil {
		t.Fatalf("LoadScript: %v", err)
	}
	return engine
}

func TestOrchestratorPrefersRulesResolver(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "request_check", Arguments: `{"actor":"player","check_kind":"luck","stakes":"fate","outcomes":{"critical":"great","fail":"bad"}}`}}},
		{text: "Luck turns."},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	o.SetCheckResolver(rulesResolverEngine(t, o.store))

	result, err := runLoopForTest(o)
	if err != nil {
		t.Fatalf("loop: %v", err)
	}
	if len(result.Checks) != 1 || result.Checks[0].Outcome != "critical" {
		t.Fatalf("checks = %+v, want the js resolver's critical outcome", result.Checks)
	}
}
