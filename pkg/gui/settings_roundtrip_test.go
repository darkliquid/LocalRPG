package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestSettingsRoundTripKeepsProviders(t *testing.T) {
	// A provider setup is the thing a user is most annoyed to lose, and the
	// settings API replaces the whole document, so a round trip has to keep every
	// provider, its key reference, and the prices beside it.
	svc := NewService(t.TempDir())
	cfg := svc.configMgr.Get()
	cfg.Providers.Gemini.APIKey = "env:GEMINI_API_KEY"
	cfg.Providers.Currency = "GBP"
	cfg.Providers.Prices = []config.PriceConfig{
		{Provider: "llm:gemini", Model: "gemini-3.8-flash", PerMillionInput: 750_000},
	}
	cfg.Agents.Roles[config.RoleGM] = config.AgentRoleConfig{
		Type: "http", Endpoint: "http://127.0.0.1:11434/v1", Model: "llama3.1:8b",
	}
	cfg.Agents.Roles["generator"] = config.AgentRoleConfig{
		Type: "inherit", InheritFrom: config.RoleGM,
	}

	if _, err := svc.SaveSettings(context.Background(), *cfg); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	// Read it back the way the app does, from disk rather than from memory.
	fresh := NewService(svc.rootDir)
	reloaded := fresh.configMgr.Get()
	if got := reloaded.Agents.Roles[config.RoleGM].Endpoint; got != "http://127.0.0.1:11434/v1" {
		t.Fatalf("the provider did not survive the round trip: %+v", reloaded.Agents.Roles)
	}
	if got := reloaded.Providers.Gemini.APIKey; got != "env:GEMINI_API_KEY" {
		t.Fatalf("the shared key did not survive: %q", got)
	}
	if got := reloaded.Providers.Currency; got != "GBP" {
		t.Fatalf("the currency did not survive: %q", got)
	}
	if len(reloaded.Providers.Prices) != 1 {
		t.Fatalf("prices did not survive: %+v", reloaded.Providers.Prices)
	}
	if reloaded.Agents.Roles["generator"].InheritFrom != config.RoleGM {
		t.Fatalf("the generator role did not survive: %+v", reloaded.Agents.Roles["generator"])
	}
	if reloaded.Agents.Roles[config.RoleGM].Type != "http" {
		t.Fatalf("the gm role did not survive: %+v", reloaded.Agents.Roles[config.RoleGM])
	}
	if resolveGeneratorRole(reloaded) != config.RoleGM {
		t.Fatalf("the reloaded config should resolve a real generator, got %q",
			resolveGeneratorRole(reloaded))
	}
}
