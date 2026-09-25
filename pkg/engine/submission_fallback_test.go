package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestMalformedSubmissionFallsBackToProse(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "submit_turn", Arguments: `{"segments":[]}`}}},
		{text: "The room is quiet.", tools: []harness.ToolCall{{ID: "2", Name: "submit_turn", Arguments: `{"segments":[]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	turn, err := o.ProcessActionStream(context.Background(), "Do", "look around", nil)
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if turn.Verdict != nil {
		t.Fatalf("fallback turn should have no verdict, got %+v", turn.Verdict)
	}
	if !strings.Contains(turn.Narration, "quiet") {
		t.Fatalf("narration = %q, want the provisional prose", turn.Narration)
	}
}
