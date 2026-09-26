package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestLoadPromptsBuildsMechanicsInstruction(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	sysDir := paths.SystemDir("sys")
	if err := os.MkdirAll(filepath.Join(sysDir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "mechanics.js"), []byte(`onAction("do", function(ctx) { return {}; });`), 0644); err != nil {
		t.Fatal(err)
	}
	systemYAML := "id: sys\nname: Sys\nmechanics:\n  checks:\n    notation: 2d6\n    outcome: [pass, fail]\n"
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte(systemYAML), 0644); err != nil {
		t.Fatal(err)
	}

	o := &TurnOrchestrator{}
	o.LoadPrompts(paths, "sys", "")

	if !strings.Contains(o.mechanicsPrompt, "## RESOLVING UNCERTAINTY") {
		t.Fatalf("mechanics prompt not built: %q", o.mechanicsPrompt)
	}
	if !strings.Contains(o.mechanicsPrompt, "2d6") {
		t.Errorf("mechanics prompt missing declared notation: %q", o.mechanicsPrompt)
	}
}

func TestLoadPromptsOmitsMechanicsWhenNoneShipped(t *testing.T) {
	paths := core.NewPathResolver(t.TempDir())
	sysDir := paths.SystemDir("plain")
	if err := os.MkdirAll(filepath.Join(sysDir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "system.yaml"), []byte("id: plain\nname: Plain\n"), 0644); err != nil {
		t.Fatal(err)
	}

	o := &TurnOrchestrator{}
	o.LoadPrompts(paths, "plain", "")

	if o.mechanicsPrompt != "" {
		t.Errorf("mechanics prompt = %q, want empty", o.mechanicsPrompt)
	}
}
