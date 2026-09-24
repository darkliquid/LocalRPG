package harness_test

import (
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestNewModelProviderUsesTheRegistry(t *testing.T) {
	cases := []harness.ProviderConfig{
		{Type: "http", Endpoint: "http://localhost:11434/v1", Model: "llama3.2"},
		{Type: "cli", Command: "echo"},
		{Type: "builtin", BuiltinName: "narrative-oracle"},
		{Type: "gemini", APIKey: "test-key", Model: "gemini-3.8-flash"},
	}
	for _, cfg := range cases {
		provider, err := harness.NewModelProvider("gm", cfg)
		if err != nil {
			t.Errorf("NewModelProvider(%q): %v", cfg.Type, err)
			continue
		}
		if provider == nil {
			t.Errorf("NewModelProvider(%q) returned nil", cfg.Type)
		}
	}

	if _, err := harness.NewModelProvider("gm", harness.ProviderConfig{Type: "nonsense"}); err == nil {
		t.Error("expected an unknown provider type to error")
	}
}

func TestBuildModelRejectsUnknownID(t *testing.T) {
	if _, err := harness.BuildModel("does-not-exist", harness.ProviderConfig{}); err == nil {
		t.Error("expected BuildModel to reject an unknown ID")
	}
}
