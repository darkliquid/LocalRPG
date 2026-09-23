package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestSaveSettingsClampsProfileOptions(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &inspectingClient{}, nil
	}

	cfg := config.DefaultConfig()
	cfg.Media.TTS = config.TTSConfig{
		Type:         "http",
		Endpoint:     "http://localhost:8880/v1/audio/speech",
		AutoPlay:     true,
		MasterVolume: 1,
		VoiceProfiles: []config.VoiceProfile{
			{ID: "hushed", Name: "Hushed", VoiceID: "bf_emma", Options: map[string]interface{}{"stability": 2.5, "banana": 1}},
		},
		Options: map[string]interface{}{"stability": -1.0},
	}

	if err := svc.validateVoiceOptionsInConfig(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}

	profile := cfg.Media.TTS.VoiceProfiles[0]
	if profile.Options["stability"] != 1.0 {
		t.Errorf("profile stability = %v, want the clamped 1.0", profile.Options["stability"])
	}
	if _, ok := profile.Options["banana"]; ok {
		t.Errorf("an unknown option must be dropped, got %v", profile.Options)
	}
	if cfg.Media.TTS.Options["stability"] != 0.0 {
		t.Errorf("config stability = %v, want the clamped 0.0", cfg.Media.TTS.Options["stability"])
	}
}

func TestSaveSettingsKeepsOptionsWhenProviderIsDisabled(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &bareClient{}, nil
	}

	cfg := config.DefaultConfig()
	cfg.Media.TTS = config.TTSConfig{
		Type:     "builtin",
		AutoPlay: false,
		Options:  map[string]interface{}{"stability": 0.5},
	}

	// A provider with no declaration cannot validate, so nothing is dropped.
	if err := svc.validateVoiceOptionsInConfig(cfg); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cfg.Media.TTS.Options["stability"] != 0.5 {
		t.Errorf("options = %v, want the value kept", cfg.Media.TTS.Options)
	}
}
