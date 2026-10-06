package harness

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/storage"
)

func TestFormatMechanicsInstructions(t *testing.T) {
	generic := FormatMechanicsInstructions(nil, "auto", nil)
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
	}, "auto", nil)
	for _, want := range []string{"2d6", "strong, weak, miss", "Standard 8"} {
		if !strings.Contains(declared, want) {
			t.Errorf("declared instruction missing %q: %q", want, declared)
		}
	}

	withStats := FormatMechanicsInstructions(nil, "auto", []StatValue{
		{ID: "body", Label: "Body", Value: 3},
	})
	if !strings.Contains(withStats, "Player stats: Body 3") {
		t.Errorf("stats line missing: %q", withStats)
	}
}

func TestMechanicsInstructionListsSkills(t *testing.T) {
	spec := &core.MechanicsSpec{Skills: []core.SkillSpec{{ID: "stealth", Label: "Stealth"}}}
	got := FormatMechanicsInstructions(spec, "auto", nil)
	if !strings.Contains(got, "Stealth") {
		t.Fatalf("instruction did not list the skill: %s", got)
	}
}

func TestFormatMechanicsInstructionsPerPolicy(t *testing.T) {
	spec := &core.MechanicsSpec{Checks: core.CheckConventions{Notation: "2d6", Outcome: []string{"strong", "weak", "miss"}}}

	off := FormatMechanicsInstructions(spec, "off", nil)
	if !strings.Contains(off, "disabled") || strings.Contains(off, "Resolve with request_check") {
		t.Errorf("off text = %q", off)
	}
	auto := FormatMechanicsInstructions(spec, "auto", nil)
	if !strings.Contains(auto, "request_check") || !strings.Contains(auto, "2d6") {
		t.Errorf("auto text = %q", auto)
	}
	ask := FormatMechanicsInstructions(spec, "ask", nil)
	if !strings.Contains(ask, "propose_check") || strings.Contains(ask, "Resolve with request_check") {
		t.Errorf("ask text = %q", ask)
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
