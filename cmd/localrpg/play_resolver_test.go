package main

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
)

func routerWithGM(t *testing.T) *harness.Router {
	t.Helper()

	router := harness.NewRouter()
	router.RegisterProvider(harness.NewCLIProvider("gm", "echo", []string{}))
	router.AssignRole(config.RoleGM, "gm")
	return router
}

func TestResolveExtractorInheritsTheNamedRole(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{
		Type:        "inherit",
		InheritFrom: config.RoleGM,
	}

	if resolveExtractor(cfg, routerWithGM(t)) == nil {
		t.Fatalf("expected an extractor inheriting gm")
	}
}

func TestResolveExtractorDefaultsToInheritingGMWhenUnset(t *testing.T) {
	cfg := config.DefaultConfig()
	delete(cfg.Agents.Roles, config.RoleExtractor)

	if resolveExtractor(cfg, routerWithGM(t)) == nil {
		t.Fatalf("an unset extractor role must keep working as it did before")
	}
}

func TestResolveExtractorDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{Type: "disabled"}

	if resolveExtractor(cfg, routerWithGM(t)) != nil {
		t.Errorf("expected a disabled role to produce no extractor")
	}
}

func TestResolveExtractorWithAMissingInheritTarget(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{
		Type:        "inherit",
		InheritFrom: "narrator",
	}
	cfg.Agents.Roles["narrator"] = config.AgentRoleConfig{Type: "disabled"}

	if resolveExtractor(cfg, routerWithGM(t)) != nil {
		t.Errorf("expected a missing inherit target to disable extraction")
	}
}

func TestResolveExtractorUsesAConcreteProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{Type: "builtin"}

	if resolveExtractor(cfg, routerWithGM(t)) == nil {
		t.Errorf("expected a configured builtin extractor")
	}
}
