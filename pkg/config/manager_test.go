package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestConfigManager_Defaults(t *testing.T) {
	mgr := config.NewConfigManagerWithPaths("/non/existent/user/config.yaml", "/non/existent/local/localrpg.yaml")
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatalf("unexpected error loading defaults: %v", err)
	}

	if cfg.Paths.Systems != "" {
		t.Errorf("expected an empty default systems path (filled by resolution), got %q", cfg.Paths.Systems)
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

func TestDetectConfigFileUsesConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CONFIG_DIRS", "")
	xdg.Reload()
	t.Cleanup(xdg.Reload)

	read, write := config.DetectConfigFile()
	want := filepath.Join(dir, "localrpg", "config.yaml")
	if read != want || write != want {
		t.Fatalf("DetectConfigFile = (%q, %q), want %q", read, write, want)
	}
}

func TestDetectConfigFileHonoursConfigDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALRPG_CONFIG_DIR", dir)

	read, _ := config.DetectConfigFile()
	if read != filepath.Join(dir, "config.yaml") {
		t.Fatalf("DetectConfigFile = %q, want the override path", read)
	}
}
