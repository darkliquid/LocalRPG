package config_test

import (
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestValidateRejectsLegacyPriceKey(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.Prices = []config.PriceConfig{{Provider: "openaichat", PerMillionInput: 1}}
	problems := cfg.Validate()
	if len(problems) == 0 {
		t.Fatal("expected a problem for the legacy key")
	}
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, "llm:openaichat") {
		t.Errorf("message should show the canonical key, got: %s", joined)
	}
}

func TestValidateAcceptsCanonicalKeys(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.Prices = []config.PriceConfig{
		{Provider: "llm:gemini", PerMillionInput: 1},
		{Provider: "tts:http@localhost:8880", PerRequest: 2},
	}
	if problems := cfg.Validate(); len(problems) != 0 {
		t.Errorf("unexpected problems: %v", problems)
	}
}

func TestValidateWarnsOnAnOldConfigVersion(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Version = "1"
	cfg.Providers.Prices = nil
	problems := cfg.Validate()
	if len(problems) != 1 || !strings.Contains(problems[0], "canonical provider keys") {
		t.Fatalf("problems = %v, want the version warning", problems)
	}
}

func TestDefaultConfigIsCurrentVersion(t *testing.T) {
	if got := config.DefaultConfig().Version; got != config.CurrentVersion {
		t.Fatalf("DefaultConfig version = %q, want %q", got, config.CurrentVersion)
	}
}
