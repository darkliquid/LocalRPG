package harness

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestNewModelProvider(t *testing.T) {
	// Disabled
	disabled, err := NewModelProvider("p1", ProviderConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disabled.ID() != "p1" {
		t.Errorf("expected id p1, got %q", disabled.ID())
	}
	_, err = disabled.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err == nil {
		t.Errorf("expected error from disabled provider")
	}

	// CLI
	cli, err := NewModelProvider("p2", ProviderConfig{
		Type:    "cli",
		Command: "echo",
		Args:    []string{"hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, err := cli.Generate(context.Background(), GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if resp.Text == "" {
		t.Errorf("expected output from echo cli provider")
	}

	// Builtin
	builtin, err := NewModelProvider("p3", ProviderConfig{
		Type:        "builtin",
		BuiltinName: "echo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp3, err := builtin.Generate(context.Background(), GenerateRequest{Prompt: "ping"})
	if err != nil {
		t.Fatalf("builtin generate failed: %v", err)
	}
	if resp3 == nil {
		t.Errorf("expected response from builtin echo")
	}
}

func TestRouterFromConfigBuildsEachRole(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["narrator"] = config.AgentRoleConfig{Type: "cli", Command: "echo"}
	cfg.Agents.Fallbacks = map[string]string{"gm": "narrator"}

	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig failed: %v", err)
	}

	if _, err := router.GetProviderForRole("gm"); err != nil {
		t.Errorf("expected the gm role to resolve: %v", err)
	}
	if _, err := router.GetProviderForRole("narrator"); err != nil {
		t.Errorf("expected the narrator role to resolve: %v", err)
	}
}

func TestRouterFromConfigSkipsInheritedRoles(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{
		Type:        "inherit",
		InheritFrom: config.RoleGM,
	}

	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig failed: %v", err)
	}

	// An inherited role is resolved through another role, not registered itself.
	if _, err := router.GetProviderForRole(config.RoleExtractor); err == nil {
		t.Errorf("expected the inherited role to have no provider of its own")
	}
	if ExtractorFromConfig(cfg, router) == nil {
		t.Errorf("expected extraction to resolve through the inherited role")
	}
}

func TestRouterFromConfigFallsBackToEchoForGM(t *testing.T) {
	cfg := config.DefaultConfig()
	delete(cfg.Agents.Roles, "gm")

	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig failed: %v", err)
	}

	provider, err := router.GetProviderForRole("gm")
	if err != nil {
		t.Fatalf("expected an echo fallback for gm: %v", err)
	}
	if provider.ID() != "default-echo" {
		t.Errorf("provider = %q, want default-echo", provider.ID())
	}
}
