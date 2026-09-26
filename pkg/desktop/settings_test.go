package desktop

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestSettingsSnapshot(t *testing.T) {
	cfg := config.DefaultConfig()
	appState = &State{
		Loaded:      true,
		Screen:      ScreenSettings,
		SettingsTab: "paths",
		Config:      cfg,
		ConfigPath:  "/home/user/.config/localrpg/config.yaml",
	}
	ui.Snapshot(t, "settings", 1000, 700, RootView)
}

func TestSettingsTabDefaultsToPaths(t *testing.T) {
	appState = &State{Loaded: true}
	if got := settingsTab(); got != "paths" {
		t.Fatalf("settingsTab() = %q, want paths", got)
	}
	appState.SettingsTab = "media"
	if got := settingsTab(); got != "media" {
		t.Fatalf("settingsTab() = %q, want media", got)
	}
}

func TestLoadSettingsCopiesConfig(t *testing.T) {
	appState = &State{Loaded: true}
	svc := gui.NewService(t.TempDir())
	loadSettings(context.Background(), svc)
	if appState.Config == nil {
		t.Fatal("loadSettings must populate Config")
	}
	if appState.ConfigPath == "" {
		t.Error("expected a config file path")
	}
}
