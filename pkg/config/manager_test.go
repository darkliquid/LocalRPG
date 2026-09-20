package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestConfigManager_Defaults(t *testing.T) {
	mgr := config.NewConfigManagerWithPaths("/non/existent/user/config.yaml", "/non/existent/local/localrpg.yaml")
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatalf("unexpected error loading defaults: %v", err)
	}

	if cfg.Paths.Systems != "./systems" {
		t.Errorf("expected default systems path ./systems, got %q", cfg.Paths.Systems)
	}
	if cfg.Agents.DefaultRole != "gm" {
		t.Errorf("expected default role gm, got %q", cfg.Agents.DefaultRole)
	}
	if cfg.Media.TTS.Type != "disabled" {
		t.Errorf("expected default tts type disabled, got %q", cfg.Media.TTS.Type)
	}
	if cfg.Preferences.FontScale != "medium" {
		t.Errorf("expected default font_scale medium, got %q", cfg.Preferences.FontScale)
	}
}

func TestConfigManager_HierarchicalSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	userConfigPath := filepath.Join(tmpDir, "user", "config.yaml")
	localConfigPath := filepath.Join(tmpDir, "local", "localrpg.yaml")

	mgr := config.NewConfigManagerWithPaths(userConfigPath, localConfigPath)

	// Save when neither exists -> should write to user config
	cfg, _ := mgr.Load()
	cfg.Paths.Systems = "/custom/systems"
	if err := mgr.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	if _, err := os.Stat(userConfigPath); os.IsNotExist(err) {
		t.Fatalf("expected user config to be created at %s", userConfigPath)
	}

	// Now create a local override
	_ = os.MkdirAll(filepath.Dir(localConfigPath), 0755)
	_ = os.WriteFile(localConfigPath, []byte("paths:\n  systems: /local/override\n"), 0644)

	reloaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if reloaded.Paths.Systems != "/local/override" {
		t.Errorf("expected local override /local/override, got %q", reloaded.Paths.Systems)
	}
	if !mgr.IsLocalOverride() {
		t.Errorf("expected IsLocalOverride to be true")
	}
	if mgr.ActiveFilePath() != localConfigPath {
		t.Errorf("expected ActiveFilePath to be %s, got %s", localConfigPath, mgr.ActiveFilePath())
	}
}
