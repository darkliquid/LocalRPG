package harness

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestFormatMechanicsInstructions(t *testing.T) {
	generic := FormatMechanicsInstructions(nil)
	if !strings.Contains(generic, "## RESOLVING UNCERTAINTY") {
		t.Fatalf("missing header in %q", generic)
	}
	if !strings.Contains(generic, "request_check") {
		t.Fatalf("missing request_check guidance in %q", generic)
	}

	declared := FormatMechanicsInstructions(&core.MechanicsSpec{
		Checks: core.CheckConventions{
			Notation:   "2d6",
			Outcome:    []string{"strong", "weak", "miss"},
			Difficulty: []core.DifficultySpec{{ID: "standard", Label: "Standard", Target: 8}},
		},
	})
	for _, want := range []string{"2d6", "strong, weak, miss", "Standard 8"} {
		if !strings.Contains(declared, want) {
			t.Errorf("declared instruction missing %q: %q", want, declared)
		}
	}
}

func TestContextRendersMechanicsPrompt(t *testing.T) {
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assembler := NewContextAssembler(store)

	withPrompt, err := assembler.Assemble(ContextRequest{
		LocationID:      "loc1",
		PlayerID:        "p1",
		Action:          "I leap the gap",
		MechanicsPrompt: "## RESOLVING UNCERTAINTY\nRoll when it matters.\n",
	})
	if err != nil {
		t.Fatalf("assemble failed: %v", err)
	}
	if !strings.Contains(withPrompt.Prompt, "Roll when it matters.") {
		t.Fatalf("mechanics prompt not rendered:\n%s", withPrompt.Prompt)
	}

	withoutPrompt, err := assembler.Assemble(ContextRequest{LocationID: "loc1", PlayerID: "p1", Action: "I wait"})
	if err != nil {
		t.Fatalf("assemble failed: %v", err)
	}
	if strings.Contains(withoutPrompt.Prompt, "RESOLVING UNCERTAINTY") {
		t.Fatalf("mechanics prompt rendered when empty")
	}
}
