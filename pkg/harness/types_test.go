package harness

import (
	"testing"
)

func TestProviderConfigValidation(t *testing.T) {
	cfg := ProviderConfig{
		Type:     "cli",
		Command:  "claude",
		Args:     []string{"--print"},
		Endpoint: "",
	}

	if cfg.Type != "cli" || cfg.Command != "claude" {
		t.Errorf("unexpected config: %+v", cfg)
	}

	roleCfg := RoleRoutingConfig{
		Roles: map[string]ProviderConfig{
			"gm":        {Type: "cli", Command: "claude"},
			"extractor": {Type: "http", Endpoint: "http://localhost:11434", Model: "qwen2.5:7b"},
		},
	}

	if len(roleCfg.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roleCfg.Roles))
	}
}
