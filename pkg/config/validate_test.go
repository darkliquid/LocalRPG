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

func TestDefaultConfigIsCurrentVersion(t *testing.T) {
	if got := config.DefaultConfig().Version; got != config.CurrentVersion {
		t.Fatalf("DefaultConfig version = %q, want %q", got, config.CurrentVersion)
	}
}

func TestValidateReportsRoleProblems(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "bogus"}
	cfg.Agents.Roles["narrator"] = config.AgentRoleConfig{Type: "http"}
	cfg.Agents.Roles["extractor"] = config.AgentRoleConfig{Type: "cli"}
	cfg.Agents.Roles["completion"] = config.AgentRoleConfig{Type: "builtin", BuiltinName: "nope"}

	problems := strings.Join(cfg.Validate(), "\n")
	for _, want := range []string{
		`agents.roles["gm"]: unknown type "bogus"`,
		`agents.roles["narrator"]: endpoint is required`,
		`agents.roles["extractor"]: command is required`,
		`agents.roles["completion"]: unknown builtin "nope"`,
	} {
		if !strings.Contains(problems, want) {
			t.Errorf("missing %q in:\n%s", want, problems)
		}
	}
}

func TestValidateDefaultsAreClean(t *testing.T) {
	if problems := config.DefaultConfig().Validate(); len(problems) != 0 {
		t.Errorf("default config should validate cleanly, got: %v", problems)
	}
}

func TestValidateRejectsBadInstances(t *testing.T) {
	bad := config.DefaultConfig()
	bad.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "http", Endpoint: "http://x", Instance: "Bad Id"}
	if len(bad.Validate()) == 0 {
		t.Fatal("a malformed instance should be rejected")
	}

	dup := config.DefaultConfig()
	dup.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "http", Endpoint: "http://x", Instance: "same"}
	dup.Agents.Roles["narrator"] = config.AgentRoleConfig{Type: "http", Endpoint: "http://y", Instance: "same"}
	if len(dup.Validate()) == 0 {
		t.Fatal("a duplicate instance should be rejected")
	}
}

func TestValidateAcceptsUniqueInstance(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "http", Endpoint: "http://x", Instance: "good"}
	cfg.Agents.Roles["narrator"] = config.AgentRoleConfig{Type: "http", Endpoint: "http://y", Instance: "cheap"}
	if problems := cfg.Validate(); len(problems) != 0 {
		t.Errorf("unique valid instances should validate cleanly, got: %v", problems)
	}
}
