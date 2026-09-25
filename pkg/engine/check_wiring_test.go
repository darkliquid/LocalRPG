package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
	"github.com/darkliquid/localrpg/pkg/storage"
)

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
		{tools: []harness.ToolCall{{ID: "2", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"uncertain","reason":"luck"},"segments":[{"kind":"narration","text":"Luck turns.","check_ref":"1"}]}`}}},
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
