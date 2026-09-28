package harness

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestRouterRecordsRoleBuildFailures(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "bogus"}

	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig: %v", err)
	}

	buildErrs := router.BuildErrors()
	if len(buildErrs) != 1 {
		t.Fatalf("BuildErrors = %d, want 1", len(buildErrs))
	}
	if buildErrs[0].Role != "gm" {
		t.Errorf("role = %q, want gm", buildErrs[0].Role)
	}
	if buildErrs[0].Err == nil {
		t.Errorf("expected the build error to be recorded")
	}

	// A configured-but-broken gm must not be silently replaced by the echo.
	if _, err := router.GetProviderForRole("gm"); err == nil {
		t.Errorf("a broken gm must not resolve to a fallback provider")
	}
}

func TestRouterEchoOnlyWhenGMUnconfigured(t *testing.T) {
	cfg := config.DefaultConfig()
	delete(cfg.Agents.Roles, "gm")

	router, err := RouterFromConfig(cfg)
	if err != nil {
		t.Fatalf("RouterFromConfig: %v", err)
	}
	if len(router.BuildErrors()) != 0 {
		t.Errorf("unexpected build errors: %v", router.BuildErrors())
	}
	provider, err := router.GetProviderForRole("gm")
	if err != nil {
		t.Fatalf("expected the echo default: %v", err)
	}
	if provider.ID() != "default-echo" {
		t.Errorf("provider = %q, want default-echo", provider.ID())
	}
}
