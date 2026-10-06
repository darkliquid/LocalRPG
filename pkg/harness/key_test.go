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
		{harness.ProviderConfig{Type: "inworld"}, provider.KeyLLMInworld, true},
		{harness.ProviderConfig{Type: "builtin", BuiltinName: "inworld"}, provider.KeyLLMInworld, true},
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

func TestKeyForPrefersInstance(t *testing.T) {
	base := harness.ProviderConfig{Type: "http", Endpoint: "https://api.openai.com"}
	a, _ := harness.KeyFor(base)
	base.Instance = "good"
	b, _ := harness.KeyFor(base)
	if a == b {
		t.Fatalf("instance did not change the key: %s", a)
	}
	if b != "llm:openaichat@good" {
		t.Fatalf("key = %s", b)
	}

	builtin := harness.ProviderConfig{Type: "builtin", BuiltinName: "gemini", Instance: "second"}
	got, _ := harness.KeyFor(builtin)
	if got != "llm:gemini@second" {
		t.Fatalf("builtin instance key = %s", got)
	}
}

// TestKeyForUnsetInstanceIsUnchanged guards the backward-compatible case: a
// configuration with no instance produces exactly the key it produced before.
func TestKeyForUnsetInstanceIsUnchanged(t *testing.T) {
	got, ok := harness.KeyFor(harness.ProviderConfig{Type: "http", Endpoint: "https://api.openai.com"})
	if !ok || got != "llm:openaichat@api.openai.com" {
		t.Fatalf("unset instance key = %q/%v, want llm:openaichat@api.openai.com", got, ok)
	}
}
