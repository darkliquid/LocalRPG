package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// runLoopForTest drives the generation loop with a minimal assembled context.
func runLoopForTest(o *TurnOrchestrator) (streamResult, error) {
	o.logger = trace.Nop()
	return o.runGenerationLoop(context.Background(), &harness.AssembleResult{Prompt: "context"}, "", nil)
}

func TestLoopResolvesCheckThenSubmits(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "request_check", Arguments: `{"actor":"player","check_kind":"skill","stakes":"jump","outcomes":{"pass":"clear","fail":"fall"}}`}}},
		{tools: []harness.ToolCall{{ID: "2", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"uncertain","reason":"a gap"},"segments":[{"kind":"narration","text":"You leap.","check_ref":"1"}]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	o.SetCheckResolver(defaultCheckResolver{})

	result, err := runLoopForTest(o)
	if err != nil {
		t.Fatalf("loop: %v", err)
	}
	if result.Submission == nil {
		t.Fatal("no submission")
	}
	if len(result.Checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(result.Checks))
	}
	if result.Checks[0].CheckID == "" {
		t.Fatal("check has no id")
	}
}
