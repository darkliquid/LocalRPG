package desktop

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/models"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/ui"
)

func settingsStateWithTab(tab string) *State {
	cfg := config.DefaultConfig()
	cfg.Media.TTS.Type = "builtin"
	cfg.Media.TTS.BuiltinName = "kokoro"
	cfg.Agents.Roles = map[string]config.AgentRoleConfig{
		"gm": {Type: "builtin", BuiltinName: "narrative-oracle", Model: "oracle-1", Temperature: 0.8, MaxTokens: 2048},
	}
	return &State{
		Loaded:      true,
		Screen:      ScreenSettings,
		SettingsTab: tab,
		Config:      cfg,
		ConfigPath:  "/home/user/.config/localrpg/config.yaml",
		Providers: []provider.Descriptor{
			{ID: "narrative-oracle", Label: "Narrative Oracle", Family: provider.FamilyLLM, Source: "builtin"},
			{ID: "native-os", Label: "Native OS Voice", Family: provider.FamilyTTS, Source: "builtin"},
		},
		Models: []models.ModelStatus{
			{ID: "kokoro-tts", Name: "Kokoro Voice Pack", TotalBytes: 90177536},
		},
		Inspect: &gui.TTSInspectResponseDTO{
			ProviderKey: "builtin:kokoro",
			Catalog: gui.VoiceCatalogDTO{Available: true, Voices: []media.ProviderVoice{
				{ID: "af_bella", Name: "Bella (American Female)", Gender: "female", Accent: "american"},
				{ID: "am_adam", Name: "Adam (American Male)", Gender: "male", Accent: "american"},
				{ID: "bf_emma", Name: "Emma (British Female)", Gender: "female", Accent: "british"},
			}},
			Options: []media.VoiceOption{{Key: "speed", Label: "Speed", Kind: "float", Min: 0.5, Max: 2, Default: 1.0}},
			SpeechCues: media.SpeechCueCapabilities{
				AudioTags: true, SupportedTags: []string{"whispers", "sighs"},
			},
		},
	}
}

func TestSettingsAgentsTabSnapshot(t *testing.T) {
	appState = settingsStateWithTab("agents")
	ui.Snapshot(t, "settings_agents", 1000, 700, RootView)
}

func TestSettingsProvidersTabSnapshot(t *testing.T) {
	appState = settingsStateWithTab("providers")
	ui.Snapshot(t, "settings_providers", 1000, 700, RootView)
}

func TestSettingsPreferencesTabSnapshot(t *testing.T) {
	appState = settingsStateWithTab("preferences")
	ui.Snapshot(t, "settings_preferences", 1000, 700, RootView)
}

func TestSettingsMediaTabSnapshot(t *testing.T) {
	appState = settingsStateWithTab("media")
	ui.Snapshot(t, "settings_media", 1000, 2400, RootView)
}
