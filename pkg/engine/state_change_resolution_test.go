package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/rules"
)

// A state change that names an entity the way the prose does must land, and one
// that names nothing at all must be skipped: the GM has already narrated the
// consequence, so a reference the index cannot resolve must not cost the player
// the turn they just played.
func TestStateChangesResolveNamesWithoutLosingTheTurn(t *testing.T) {
	provider := &toolScriptProvider{replies: []toolReply{
		{tools: []harness.ToolCall{{ID: "1", Name: "submit_turn", Arguments: `{"action_verdict":{"feasibility":"automatic","reason":"nothing in the way"},"segments":[{"kind":"narration","text":"You shoulder the door open."}],"state_changes":[{"entity":"Nobody At All","path":"hp","op":"sub","value":1},{"entity":"Alden Tavern","path":"reputation","op":"set","value":3}]}`}}},
	}}
	o, _ := toolLoopOrchestrator(t, provider)
	o.SetTools(&fakeExecutor{}, "yes")
	o.rulesEngine = rules.NewJSEngine(rules.NewHostBridge(o.store, nil, "player"))

	turn, err := o.ProcessActionStream(context.Background(), "Do", "shoulder the door open", nil)
	if err != nil {
		t.Fatalf("an unresolvable state change must not lose the turn: %v", err)
	}
	if turn.Narration != "You shoulder the door open." {
		t.Errorf("narration = %q, want the submitted segment", turn.Narration)
	}

	// The display name resolved to the indexed location, which is what the GM was
	// told it could write.
	reputation, err := o.rulesEngine.HostAPI().GetStat("alden-tavern", "reputation")
	if err != nil {
		t.Fatalf("GetStat: %v", err)
	}
	if value, ok := reputation.(float64); !ok || value != 3 {
		t.Errorf("tavern reputation = %v, want 3", reputation)
	}
}
