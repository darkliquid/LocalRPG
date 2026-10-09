package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestChainRoundTrips(t *testing.T) {
	in := []byte("agents:\n  roles:\n    gm:\n      chain: [good, cheap]\n      select: cheapest\n")
	var cfg Config
	if err := yaml.Unmarshal(in, &cfg); err != nil {
		t.Fatal(err)
	}
	role := cfg.Agents.Roles["gm"]
	if len(role.Chain) != 2 || role.Select != SelectCheapest {
		t.Fatalf("role = %+v", role)
	}
	if role.ChainConfig().Rule() != SelectCheapest {
		t.Fatalf("rule = %q", role.ChainConfig().Rule())
	}
}

func TestPurposeChainRoundTrips(t *testing.T) {
	in := []byte("media:\n  purpose_chains:\n    scene:\n      chain: [hero, default]\n      select: local-first\n")
	var cfg Config
	if err := yaml.Unmarshal(in, &cfg); err != nil {
		t.Fatal(err)
	}
	chain := cfg.Media.PurposeChain(PurposeScene)
	if len(chain.Chain) != 2 || chain.Rule() != SelectLocalFirst {
		t.Fatalf("purpose chain = %+v", chain)
	}
	if got := cfg.Media.PurposeChain(PurposeNPC); len(got.Chain) != 0 || got.Rule() != SelectFirst {
		t.Fatalf("an undeclared purpose must be empty and default to first, got %+v", got)
	}
}

func TestValidSelectRule(t *testing.T) {
	for _, rule := range []string{SelectFirst, SelectCheapest, SelectLocalFirst, SelectByTag} {
		if !ValidSelectRule(rule) {
			t.Errorf("%q should be valid", rule)
		}
	}
	if ValidSelectRule("bogus") {
		t.Error("bogus should not be a valid rule")
	}
}

func TestValidateRejectsBadChain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.Roles["gm"] = AgentRoleConfig{Type: "http", Endpoint: "http://x", Chain: []string{"gone"}, Select: SelectCheapest}
	if len(cfg.Validate()) == 0 {
		t.Fatal("an unknown chain member should be rejected")
	}
}

func TestValidateRejectsUnknownRule(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.Roles["gm"] = AgentRoleConfig{Type: "http", Endpoint: "http://x", Chain: []string{"gm"}, Select: "cheapestest"}
	if !hasProblem(cfg.Validate(), "unknown rule") {
		t.Fatalf("an unknown rule should be rejected, got %v", cfg.Validate())
	}
}

func TestValidateRejectsByTagWithoutTag(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.Roles["gm"] = AgentRoleConfig{Type: "http", Endpoint: "http://x", Chain: []string{"gm"}, Select: SelectByTag}
	if !hasProblem(cfg.Validate(), "by-tag requires a tag") {
		t.Fatalf("by-tag without a tag should be rejected, got %v", cfg.Validate())
	}
}

func TestValidateAcceptsAConfiguredChain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.Roles["gm"] = AgentRoleConfig{Type: "http", Endpoint: "http://x", Chain: []string{"gm", "cheap"}, Select: SelectCheapest}
	cfg.Agents.Roles["cheap"] = AgentRoleConfig{Type: "http", Endpoint: "http://y"}
	if problems := cfg.Validate(); len(problems) != 0 {
		t.Fatalf("a configured chain should validate, got %v", problems)
	}
}

func TestValidateChainDefaultsToFirst(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.Roles["gm"] = AgentRoleConfig{Type: "http", Endpoint: "http://x", Chain: []string{"gm"}}
	if problems := cfg.Validate(); len(problems) != 0 {
		t.Fatalf("an omitted select must default to first, got %v", problems)
	}
}

func TestValidateRejectsBadPurposeChain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Media.PurposeChains = map[string]ChainConfig{
		"scene": {Chain: []string{"nope"}, Select: SelectFirst},
		"bogus": {Chain: []string{"default"}},
	}
	problems := cfg.Validate()
	if !hasProblem(problems, "not a configured provider") {
		t.Fatalf("an unknown image provider should be rejected, got %v", problems)
	}
	if !hasProblem(problems, "unknown purpose") {
		t.Fatalf("an unknown purpose key should be rejected, got %v", problems)
	}
}

func hasProblem(problems []string, want string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, want) {
			return true
		}
	}
	return false
}
