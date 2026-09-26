package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/gui"
)

func TestContextTokenSummary(t *testing.T) {
	appState = &State{TurnContext: nil}
	if got := contextTokenSummary(); got != "" {
		t.Fatalf("nil context summary = %q, want empty", got)
	}
	appState.TurnContext = &gui.TurnContextDTO{Budget: 8000, EstimatedTokens: 2000}
	if got := contextTokenSummary(); got != "2000/8000 tokens" {
		t.Fatalf("summary = %q", got)
	}
}

func TestShortHash(t *testing.T) {
	if got := shortHash(""); got != "-" {
		t.Errorf("empty = %q", got)
	}
	if got := shortHash("abcdef1234567890"); got != "abcdef12" {
		t.Errorf("long = %q", got)
	}
}
