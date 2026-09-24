package harness

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func routerWithGM(t *testing.T) *Router {
	t.Helper()

	router := NewRouter()
	router.RegisterProvider(&builtinEchoModelProvider{id: "gm"})
	router.AssignRole(config.RoleGM, "gm")
	return router
}

func TestExtractorFromConfigInheritsTheNamedRole(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{
		Type:        "inherit",
		InheritFrom: config.RoleGM,
	}

	if ExtractorFromConfig(cfg, routerWithGM(t)) == nil {
		t.Fatalf("expected an extractor inheriting gm")
	}
}

func TestExtractorFromConfigDefaultsToInheritingGMWhenUnset(t *testing.T) {
	cfg := config.DefaultConfig()
	delete(cfg.Agents.Roles, config.RoleExtractor)

	if ExtractorFromConfig(cfg, routerWithGM(t)) == nil {
		t.Fatalf("an unset extractor role must keep working as it did before")
	}
}

func TestExtractorFromConfigDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{Type: "disabled"}

	if ExtractorFromConfig(cfg, routerWithGM(t)) != nil {
		t.Errorf("expected a disabled role to produce no extractor")
	}
}

func TestExtractorFromConfigWithAMissingInheritTarget(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{
		Type:        "inherit",
		InheritFrom: "narrator",
	}
	cfg.Agents.Roles["narrator"] = config.AgentRoleConfig{Type: "disabled"}

	if ExtractorFromConfig(cfg, routerWithGM(t)) != nil {
		t.Errorf("expected a missing inherit target to disable extraction")
	}
}

func TestExtractorFromConfigUsesAConcreteProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Roles[config.RoleExtractor] = config.AgentRoleConfig{Type: "builtin"}

	if ExtractorFromConfig(cfg, routerWithGM(t)) == nil {
		t.Errorf("expected a configured builtin extractor")
	}
}
