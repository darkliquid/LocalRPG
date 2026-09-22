package config_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestPresetsCatalog(t *testing.T) {
	// 1. LLM Presets
	ollamaPreset, ok := config.GetAgentPreset("ollama")
	if !ok || ollamaPreset.Type != "http" || ollamaPreset.Endpoint != "http://localhost:11434/v1" {
		t.Errorf("expected valid ollama preset, got %+v", ollamaPreset)
	}

	// 2. TTS Presets
	kokoroPreset, ok := config.GetTTSPreset("kokoro-fastapi")
	if !ok || kokoroPreset.Type != "http" || kokoroPreset.DefaultVoice != "af_bella" {
		t.Errorf("expected valid kokoro preset, got %+v", kokoroPreset)
	}

	nativeOSPreset, ok := config.GetTTSPreset("native-os")
	if !ok || nativeOSPreset.Type != "builtin" || nativeOSPreset.BuiltinName != "native-os" {
		t.Errorf("expected valid native-os preset, got %+v", nativeOSPreset)
	}

	sherpaPreset, ok := config.GetTTSPreset("sherpa-onnx")
	if !ok || sherpaPreset.Type != "builtin" || sherpaPreset.BuiltinName != "sherpa-onnx" {
		t.Errorf("expected valid sherpa-onnx preset, got %+v", sherpaPreset)
	}

	// 3. Image Presets
	artPreset, ok := config.GetImagePreset("procedural-art")
	if !ok || artPreset.Type != "builtin" || artPreset.BuiltinName != "procedural-art" {
		t.Errorf("expected valid procedural-art preset, got %+v", artPreset)
	}
}

func TestDefaultVoiceProfiles(t *testing.T) {
	cfg := config.DefaultConfig()
	if len(cfg.Media.TTS.VoiceProfiles) == 0 {
		t.Fatalf("expected default voice profiles in default config")
	}

	foundElder := false
	for _, p := range cfg.Media.TTS.VoiceProfiles {
		if p.ID == "elder_sage" {
			foundElder = true
			if p.Pitch >= 1.0 {
				t.Errorf("expected elder_sage to have deeper pitch < 1.0, got %f", p.Pitch)
			}
		}
	}
	if !foundElder {
		t.Errorf("expected elder_sage archetype in default voice profiles")
	}
}
