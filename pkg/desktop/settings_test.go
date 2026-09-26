package desktop

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func TestApplyModelStatusReplacesByID(t *testing.T) {
	appState = &State{Models: []models.ModelStatus{{ID: "kokoro-tts", Name: "Kokoro", Progress: 0}}}
	applyModelStatus(models.ModelStatus{ID: "kokoro-tts", Name: "Kokoro", Downloading: true, Progress: 0.5})
	if len(appState.Models) != 1 || appState.Models[0].Progress != 0.5 {
		t.Fatalf("models = %+v", appState.Models)
	}
}

func TestProvidersForFamilyFilters(t *testing.T) {
	appState = &State{Providers: []provider.Descriptor{
		{ID: "a", Family: provider.FamilyLLM},
		{ID: "b", Family: provider.FamilyTTS},
		{ID: "c", Family: provider.FamilyLLM},
	}}
	got := providersForFamily(provider.FamilyLLM)
	if len(got) != 2 {
		t.Fatalf("llm providers = %d, want 2", len(got))
	}
	if got := providersForFamily(provider.FamilySTT); len(got) != 0 {
		t.Fatalf("stt providers = %d, want 0", len(got))
	}
}

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
