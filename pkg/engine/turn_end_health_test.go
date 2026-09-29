package engine

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/rules"
)

// turnEndHealthOrchestrator builds a playable orchestrator whose onTurnEnd hook
// runs script, with a declared health schema.
func turnEndHealthOrchestrator(t *testing.T, script string, startHealth int) *TurnOrchestrator {
	t.Helper()

	provider := &scriptedStreamProvider{chunks: []string{"The poison spreads."}}
	orchestrator, _, store := streamingOrchestrator(t, provider)

	jsEngine := rules.NewJSEngine(rules.NewHostBridge(store, nil, "player"))
	if err := jsEngine.LoadScript(script); err != nil {
		t.Fatalf("load script: %v", err)
	}
	if err := jsEngine.HostAPI().SetStat("player", "hp", startHealth); err != nil {
		t.Fatal(err)
	}
	orchestrator.rulesEngine = jsEngine
	orchestrator.SetHealthSpec(&core.HealthSpec{Stat: "hp", ZeroEffect: "You collapse."})
	return orchestrator
}

func TestTurnEndHookHealthChangeIsRecordedOnTheSameTurn(t *testing.T) {
	orchestrator := turnEndHealthOrchestrator(t,
		`onTurnEnd(function (ctx) { setStat("player", "hp", 0); });`, 3)

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I press on")
	if err != nil {
		t.Fatalf("ProcessAction: %v", err)
	}
	if len(turn.HealthEffects) != 1 || turn.HealthEffects[0].Effect != "You collapse." {
		t.Fatalf("HealthEffects = %#v, want the zero effect recorded on this turn", turn.HealthEffects)
	}
}

func TestTurnEndHookHealingKeepsTheEarlierEffect(t *testing.T) {
	// The player is already down when the turn ends; the hook heals them, but the
	// effect still happened and must be recorded exactly once.
	orchestrator := turnEndHealthOrchestrator(t,
		`onTurnEnd(function (ctx) { setStat("player", "hp", 5); });`, 0)

	turn, err := orchestrator.ProcessAction(context.Background(), "Do", "I steady myself")
	if err != nil {
		t.Fatalf("ProcessAction: %v", err)
	}
	if len(turn.HealthEffects) != 1 || turn.HealthEffects[0].Effect != "You collapse." {
		t.Fatalf("HealthEffects = %#v, want exactly one effect", turn.HealthEffects)
	}
}

func TestTurnEndHookSeesPendingHealthEffect(t *testing.T) {
	// The hook records how many health effects it was handed, which is the
	// effect resolved from the turn's own state changes before the hook ran.
	orchestrator := turnEndHealthOrchestrator(t,
		`onTurnEnd(function (ctx) { setStat("player", "seen", ctx.health_effects ? ctx.health_effects.length : -1); });`, 0)

	if _, err := orchestrator.ProcessAction(context.Background(), "Do", "I wait"); err != nil {
		t.Fatalf("ProcessAction: %v", err)
	}

	seen, err := orchestrator.rulesEngine.HostAPI().GetStat("player", "seen")
	if err != nil {
		t.Fatalf("GetStat: %v", err)
	}
	if intValue(seen) != 1 {
		t.Fatalf("hook saw %d health effects, want 1", intValue(seen))
	}
}
