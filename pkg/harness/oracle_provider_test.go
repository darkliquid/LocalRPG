package harness_test

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestNarrativeOracle_GeneratesResponsiveProse(t *testing.T) {
	provider, err := harness.NewModelProvider("gm", harness.ProviderConfig{
		Type:        "builtin",
		BuiltinName: "narrative-oracle",
	})
	if err != nil {
		t.Fatalf("failed to create narrative oracle: %v", err)
	}

	prompt := `
## CURRENT SCENE & IMMEDIATE CONTEXT
Location: [[The Sunken Outpost]]
Present Entities: [[Elena Nightshade]], [[Warden Craig]]

[MECHANICS RESULT: Mode=attack Roll=11 Tier=Success]
Player Action: I strike at the shadow beast with my silver blade!
`

	resp, err := provider.Generate(context.Background(), harness.GenerateRequest{Prompt: prompt})
	if err != nil {
		t.Fatalf("oracle generate failed: %v", err)
	}

	if resp.Text == "" {
		t.Errorf("expected non-empty narrative response")
	}

	if strings.HasPrefix(resp.Text, "Echo:") {
		t.Errorf("expected narrative prose, got echo: %s", resp.Text)
	}

	// Should weave in wikilinks and acknowledge the action/mechanics
	if !strings.Contains(resp.Text, "Elena") && !strings.Contains(resp.Text, "blade") {
		t.Errorf("expected prose to acknowledge player action or entity: %s", resp.Text)
	}
}
