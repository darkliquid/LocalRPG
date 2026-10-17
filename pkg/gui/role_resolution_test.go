package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// TestRouterWithChainsResolvesCLIAndHTTPRoles verifies that CLI and HTTP agent
// roles declared in config resolve through the GUI service router without build
// errors (LocalRPG-gsq).
func TestRouterWithChainsResolvesCLIAndHTTPRoles(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{
		Type:    "cli",
		Command: "/bin/sh",
		Args:    []string{"-c", "exit 0"},
	}
	cfg.Agents.Roles["extractor"] = config.AgentRoleConfig{
		Type:     "http",
		Endpoint: "http://127.0.0.1:8080/v1",
		Model:    "test-model",
	}

	router, err := routerWithChains(cfg, trace.Nop())
	if err != nil {
		t.Fatalf("routerWithChains: %v", err)
	}

	for _, buildErr := range router.BuildErrors() {
		t.Errorf("unexpected build error for role %q (%s): %v", buildErr.Role, buildErr.Type, buildErr.Err)
	}

	gmProvider, err := router.GetProviderForRole("gm")
	if err != nil {
		t.Fatalf("GetProviderForRole(gm): %v", err)
	}
	if gmProvider == nil {
		t.Fatal("expected non-nil gm provider")
	}

	extractorProvider, err := router.GetProviderForRole("extractor")
	if err != nil {
		t.Fatalf("GetProviderForRole(extractor): %v", err)
	}
	if extractorProvider == nil {
		t.Fatal("expected non-nil extractor provider")
	}
}
