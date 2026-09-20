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

	bridge := NewHostBridge(store)
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
