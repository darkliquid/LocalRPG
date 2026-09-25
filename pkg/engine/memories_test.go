package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestTurnWritesDeclaredAndMechanicalMemories(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "request_check", Arguments: `{"actor":"Kae","check_kind":"skill","stakes":"cross the bridge","outcomes":{"pass":"clear","fail":"fall"}}`}}},
		{tools: []harness.ToolCall{{ID: "2", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"uncertain","reason":"a swaying bridge"},"segments":[{"kind":"narration","text":"Kae crosses.","check_ref":"1"}],"personae":[{"name":"Kae","type":"character","new":true}],"memories":[{"kind":"event","entity_refs":["Kae"],"text":"Kae crossed the rope bridge.","importance":3}]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")

	if _, err := o.ProcessActionStream(context.Background(), "Do", "cross the bridge", nil); err != nil {
		t.Fatalf("turn: %v", err)
	}

	memories, err := o.store.ListMemoriesForEntity("kae", 10)
	if err != nil {
		t.Fatalf("ListMemoriesForEntity: %v", err)
	}
	if len(memories) != 2 {
		t.Fatalf("memories = %d, want the declared and the mechanical memory", len(memories))
	}
	kinds := map[string]bool{}
	for _, m := range memories {
		kinds[m.Kind] = true
	}
	if !kinds[entity.MemoryEvent] || !kinds[entity.MemoryMechanical] {
		t.Fatalf("memory kinds = %v, want event and mechanical", kinds)
	}
}
