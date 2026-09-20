package tui

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	input := "# The Tavern\n\nA quiet room with **Lady Evelyn** in the corner."
	rendered, err := RenderMarkdown(input, 80)
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	if !strings.Contains(rendered, "The Tavern") || !strings.Contains(rendered, "Lady Evelyn") {
		t.Errorf("expected rendered markdown, got: %s", rendered)
	}
}
