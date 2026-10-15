package config_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestRevisionIncrementsOnSave(t *testing.T) {
	m := config.NewConfigManagerWithPaths(
		filepath.Join(t.TempDir(), "user.yaml"),
		filepath.Join(t.TempDir(), "local.yaml"),
	)
	start := m.Revision()
	if err := m.Save(config.DefaultConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if m.Revision() <= start {
		t.Fatalf("Revision did not advance: start %d, now %d", start, m.Revision())
	}
}

func TestConfigManagerReportsAMalformedUserConfig(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "config.yaml")
	// A tab cannot start a YAML token, so the document does not parse at all.
	if err := os.WriteFile(userPath, []byte("agents:\n\tdefault_role: gm\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	mgr := config.NewConfigManagerWithPaths(userPath, filepath.Join(dir, "localrpg.yaml"))
	cfg, err := mgr.Load()
	if err == nil {
		t.Fatal("expected an error for a config file that does not parse")
	}
	if !strings.Contains(err.Error(), userPath) {
		t.Errorf("error %q does not name the file %q", err, userPath)
	}
	if cfg == nil {
		t.Fatal("expected a usable configuration alongside the error")
	}

	var carried bool
	for _, warning := range mgr.Warnings() {
		if strings.Contains(warning, userPath) {
			carried = true
		}
	}
	if !carried {
		t.Errorf("Warnings %v do not carry the parse failure", mgr.Warnings())
	}
}

func TestConfigManagerReportsAMalformedLocalOverride(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "config.yaml")
	localPath := filepath.Join(dir, "localrpg.yaml")
	if err := os.WriteFile(userPath, []byte("agents:\n  default_role: custom\n"), 0644); err != nil {
		t.Fatalf("write user config: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("paths:\n\tloading\n"), 0644); err != nil {
		t.Fatalf("write local override: %v", err)
	}

	mgr := config.NewConfigManagerWithPaths(userPath, localPath)
	cfg, err := mgr.Load()
	if err == nil {
		t.Fatal("expected an error for a local override that does not parse")
	}
	if !strings.Contains(err.Error(), localPath) {
		t.Errorf("error %q does not name the override %q", err, localPath)
	}
	if mgr.IsLocalOverride() {
		t.Error("an override that did not parse must not mark itself as the active file")
	}
	if cfg.Agents.DefaultRole != "custom" {
		t.Errorf("expected the user config to still apply, got default_role %q", cfg.Agents.DefaultRole)
	}
}

func TestConfigManagerDoesNotHalfApplyAMalformedFile(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "config.yaml")
	// The first field decodes and the second is the wrong type, so a file read
	// field by field leaves a configuration that is neither the file nor the
	// defaults.
	if err := os.WriteFile(userPath, []byte("agents:\n  default_role: custom\npaths: 42\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	mgr := config.NewConfigManagerWithPaths(userPath, filepath.Join(dir, "localrpg.yaml"))
	cfg, err := mgr.Load()
	if err == nil {
		t.Fatal("expected an error for a field of the wrong type")
	}
	if cfg.Agents.DefaultRole != "gm" {
		t.Errorf("a file that did not parse was half applied: default_role is %q", cfg.Agents.DefaultRole)
	}
}
