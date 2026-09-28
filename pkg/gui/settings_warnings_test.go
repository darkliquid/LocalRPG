package gui

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestSaveSettingsReturnsWarnings(t *testing.T) {
	svc := NewService(t.TempDir())

	cfg := *config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "bogus"}

	res, err := svc.SaveSettings(context.Background(), cfg)
	if err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected validation warnings in the save response")
	}
}

func TestGetSettingsReturnsWarningsForABrokenConfig(t *testing.T) {
	root := t.TempDir()
	svc := NewService(root)

	cfg := *config.DefaultConfig()
	cfg.Agents.Roles["gm"] = config.AgentRoleConfig{Type: "bogus"}
	if _, err := svc.SaveSettings(context.Background(), cfg); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	res, err := svc.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected warnings for a saved broken config")
	}
}
