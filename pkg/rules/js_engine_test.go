package rules

import (
	"path/filepath"
	"testing"

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
	player.InitState(map[string]interface{}{"hp": 20})
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

	result, err := engine.ExecuteAction("attack", map[string]interface{}{"target": "goblin"})
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

	res, err := engine.ExecuteAction("attack", map[string]interface{}{"action": "swing"})
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
	truce, err := engine.ExecuteAction("parley", map[string]interface{}{"action": "talk"})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}
	if truce.Outcome != "uneasy_truce" || truce.Success {
		t.Errorf("expected the label with success false, got %+v", truce)
	}
}
