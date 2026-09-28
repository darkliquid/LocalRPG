package harness_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestKeyFor(t *testing.T) {
	tests := []struct {
		cfg  harness.ProviderConfig
		want provider.Key
		ok   bool
	}{
		{harness.ProviderConfig{Type: "http", Endpoint: "http://localhost:11434/v1"}, "llm:openaichat@localhost:11434", true},
		{harness.ProviderConfig{Type: "gemini"}, provider.KeyLLMGemini, true},
		{harness.ProviderConfig{Type: "builtin", BuiltinName: "narrative-oracle"}, provider.KeyLLMNarrativeOracle, true},
		{harness.ProviderConfig{Type: "cli", Command: "claude"}, "llm:cli@claude", true},
		{harness.ProviderConfig{Type: "disabled"}, "", false},
	}
	for _, tc := range tests {
		got, ok := harness.KeyFor(tc.cfg)
		if ok != tc.ok || (tc.want != "" && got != tc.want) {
			t.Errorf("KeyFor(%+v) = %q/%v, want %q/%v", tc.cfg, got, ok, tc.want, tc.ok)
		}
	}
}
