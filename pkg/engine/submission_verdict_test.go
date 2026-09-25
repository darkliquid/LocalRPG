package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestImpossibleVerdictRejectsAction(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"impossible","reason":"no wings"},"segments":[{"kind":"narration","text":"You cannot fly."}]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	turn, err := o.ProcessActionStream(context.Background(), "Do", "I fly over the wall", nil)
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if !turn.Rejected || turn.Verdict == nil || turn.Verdict.Feasibility != harness.FeasibilityImpossible {
		t.Fatalf("turn = %+v", turn)
	}
	if turn.Narration != "You cannot fly." {
		t.Fatalf("narration = %q", turn.Narration)
	}
}
