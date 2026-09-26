package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestRollModeBecomesProposedCheck(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"automatic","reason":"no uncertainty"},"segments":[{"kind":"narration","text":"No need to roll."}],"dismissed_checks":[{"check_ref":"player-roll","reason":"no uncertainty"}]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	if _, err := o.ProcessActionStream(context.Background(), "Roll", "stealth 2d6", nil); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if len(provider.requests) == 0 {
		t.Fatal("the provider received no request")
	}
	carried := false
	for _, request := range provider.requests {
		for _, message := range request.Messages {
			if strings.Contains(message.Content, "[PROPOSED CHECK: stealth 2d6") {
				carried = true
			}
		}
	}
	if !carried {
		t.Fatal("the player's proposed check was not passed to the model")
	}
}
