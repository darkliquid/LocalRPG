package rules

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestJSEngineExecution(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewStore(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed player
	player := &entity.Entity{
		ID:   "player",
		Name: "Sean",
		Type: "character",
	}
	player.InitState(map[string]any{"hp": 20})
	store.SaveEntity(player)

	bridge := NewHostBridge(store, nil, "")
	engine := NewJSEngine(bridge)

	script := `
onAction("attack", function(ctx) {
    var rollRes = roll("1d20+2");
    var currentHP = getStat("player", "hp");
    setStat("player", "hp", currentHP - 5);
    injectGMDirection("The player attacked with roll " + rollRes.total);
    return {
        success: rollRes.total >= 10,
        message: "Attack resolved",
        roll: rollRes
    };
});
`
	if err := engine.LoadScript(script); err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	result, err := engine.ExecuteAction("attack", map[string]any{"target": "goblin"})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if result == nil || result.Message != "Attack resolved" {
		t.Errorf("unexpected action result: %+v", result)
	}

	// Verify state mutation
	hp, err := bridge.GetStat("player", "hp")
	if err != nil || (hp != int64(15) && hp != 15 && hp != float64(15)) {
		t.Errorf("expected player hp=15, got %v (%T)", hp, hp)
	}

	// Verify GM directive injection
	directives := bridge.GetDirectives()
	if len(directives) != 1 {
		t.Errorf("expected 1 directive, got: %v", directives)
	}
}

func TestExecuteActionReadsTheOutcomeLabel(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	engine := NewJSEngine(NewHostBridge(store, nil, ""))
	if err := engine.LoadScript(`
		onAction("attack", (ctx) => ({ success: false, outcome: "glancing_blow", message: "A glancing blow." }));
		onAction("parley", (ctx) => ({ outcome: "uneasy_truce" }));
	`); err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	res, err := engine.ExecuteAction("attack", map[string]any{"action": "swing"})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}
	if res.Outcome != "glancing_blow" {
		t.Errorf("Outcome = %q, want glancing_blow", res.Outcome)
	}
	if res.Success {
		t.Errorf("expected success to stay false")
	}
	if _, ok := res.Data["outcome"]; ok {
		t.Errorf("expected the reserved key to be kept out of Data, got %+v", res.Data)
	}

	// A label alone does not imply success: the engine never invents semantics.
	truce, err := engine.ExecuteAction("parley", map[string]any{"action": "talk"})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}
	if truce.Outcome != "uneasy_truce" || truce.Success {
		t.Errorf("expected the label with success false, got %+v", truce)
	}
}

func TestGrantXPAwardsTheCurrency(t *testing.T) {
	store := newRulesTestStore(t)
	bridge := NewHostBridge(store, nil, "player")
	engine := NewJSEngine(bridge)
	engine.SetManifest(&core.SystemManifest{
		ID: "sys",
		Mechanics: &core.MechanicsSpec{
			Advancement: &core.AdvancementSpec{Currency: core.CurrencySpec{Stat: "xp"}},
		},
	})
	if err := engine.LoadScript(`grantXP(3);`); err != nil {
		t.Fatalf("LoadScript: %v", err)
	}
	if got, _ := bridge.GetStat("player", "xp"); mustInt(t, got) != 3 {
		t.Fatalf("xp = %v, want 3", got)
	}
}

func TestJSEngineHostExceptionPropagation(t *testing.T) {
	store := newRulesTestStore(t)
	bridge := NewHostBridge(store, nil, "player")
	engine := NewJSEngine(bridge)

	// 1. Host error caught within JS try/catch
	scriptWithCatch := `
var caughtError = "";
try {
    roll("not-a-valid-dice-notation");
} catch (e) {
    caughtError = "" + e;
}
`
	if err := engine.LoadScript(scriptWithCatch); err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	caughtVal := engine.vm.Get("caughtError")
	if caughtVal == nil || !strings.Contains(caughtVal.String(), "roll error") {
		t.Errorf("expected caughtError to contain 'roll error', got %v", caughtVal)
	}

	// 2. Uncaught host error surfaces as Go error from ExecuteAction
	scriptUncaughtAction := `
onAction("bad_roll", function(ctx) {
    roll("invalid_dice");
    return { success: true };
});
`
	if err := engine.LoadScript(scriptUncaughtAction); err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}
	_, err := engine.ExecuteAction("bad_roll", map[string]any{})
	if err == nil {
		t.Fatal("expected ExecuteAction to fail on uncaught host error, got nil")
	}
	if !strings.Contains(err.Error(), "roll error") {
		t.Errorf("expected error to contain 'roll error', got %v", err)
	}

	// 3. Registering non-function hook throws and surfaces to Go
	badHookScript := `onAction("test", "not-a-function");`
	err = engine.LoadScript(badHookScript)
	if err == nil {
		t.Fatal("expected LoadScript to fail when registering non-function hook")
	}
	if !strings.Contains(err.Error(), "onAction handler must be a function") {
		t.Errorf("expected error to contain 'onAction handler must be a function', got %v", err)
	}
}
