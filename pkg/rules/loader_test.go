package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestLoaderComposition(t *testing.T) {
	tempDir := t.TempDir()
	paths := core.NewPathResolver(tempDir)

	// Setup system rules
	sysDir := paths.SystemDir("d20")
	os.MkdirAll(sysDir, 0755)
	baseScript := `
onAction("inspect", function(ctx) {
    return { success: true, message: "Inspected from base system" };
});
`
	os.WriteFile(filepath.Join(sysDir, "mechanics.js"), []byte(baseScript), 0644)

	// Setup world overrides
	worldOverrideDir := filepath.Join(paths.WorldDir("fantasy"), "system_overrides", "d20")
	os.MkdirAll(worldOverrideDir, 0755)
	overrideScript := `
onAction("inspect", function(ctx) {
    return { success: true, message: "Inspected from world override" };
});
`
	os.WriteFile(filepath.Join(worldOverrideDir, "hooks.js"), []byte(overrideScript), 0644)

	store, err := storage.NewStore(filepath.Join(tempDir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	bridge := NewHostBridge(store, nil, "")
	engine := NewJSEngine(bridge)

	loader := NewRuleLoader(paths, engine)
	if err := loader.LoadRules("d20", "fantasy"); err != nil {
		t.Fatalf("LoadRules failed: %v", err)
	}

	res, err := engine.ExecuteAction("inspect", nil)
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}
	if res.Message != "Inspected from world override" {
		t.Errorf("expected override message, got %q", res.Message)
	}
}
