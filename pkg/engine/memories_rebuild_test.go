package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestEnsureTurnMemoriesIsIdempotent(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"automatic","reason":"greeting"},"segments":[{"kind":"narration","text":"Kae waves."}],"personae":[{"name":"Kae","type":"character","new":true}],"memories":[{"kind":"event","entity_refs":["Kae"],"text":"Kae waved.","importance":2}]}`}}},
	}}
	o, timeline := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")

	turn, err := o.ProcessActionStream(context.Background(), "Do", "wave", nil)
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	before, err := o.store.ListMemoriesForEntity("kae", 10)
	if err != nil {
		t.Fatalf("ListMemoriesForEntity: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("memories before rebuild = %d, want 1", len(before))
	}

	if err := timeline.ensureTurnMemories(turn); err != nil {
		t.Fatalf("ensureTurnMemories: %v", err)
	}
	after, err := o.store.ListMemoriesForEntity("kae", 10)
	if err != nil {
		t.Fatalf("ListMemoriesForEntity: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("memories after rebuild = %d, want %d (idempotent)", len(after), len(before))
	}
}
