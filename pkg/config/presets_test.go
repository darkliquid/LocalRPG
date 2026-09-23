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

func TestKokoroVoiceProfilesPreset(t *testing.T) {
	if len(config.KokoroVoiceProfiles) != 11 {
		t.Fatalf("expected 11 Kokoro voice profiles, got %d", len(config.KokoroVoiceProfiles))
	}
	for _, p := range config.KokoroVoiceProfiles {
		if p.ID == "" || p.VoiceID == "" || len(p.Tags) == 0 {
			t.Errorf("invalid profile: %+v", p)
		}
	}

	preset, ok := config.GetTTSPreset("sherpa-onnx")
	if !ok {
		t.Fatal("missing sherpa-onnx preset")
	}
	if len(preset.VoiceProfiles) != 11 {
		t.Errorf("expected sherpa-onnx preset to have 11 voice profiles, got %d", len(preset.VoiceProfiles))
	}
}

func TestGetGeminiAgentPresets(t *testing.T) {
	preset, ok := config.GetAgentPreset("gemini-2.5-flash")
	if !ok {
		t.Fatal("expected gemini-2.5-flash preset to exist")
	}
	if preset.BuiltinName != "gemini" || preset.Model != "gemini-2.5-flash" {
		t.Errorf("unexpected preset: %+v", preset)
	}
	if preset.ThinkingBudget == nil || *preset.ThinkingBudget != 0 {
		t.Errorf("expected thinking_budget 0 for flash, got %v", preset.ThinkingBudget)
	}

	proPreset, ok := config.GetAgentPreset("gemini-2.5-pro")
	if !ok {
		t.Fatal("expected gemini-2.5-pro preset to exist")
	}
	if proPreset.ThinkingBudget == nil || *proPreset.ThinkingBudget != -1 {
		t.Errorf("expected thinking_budget -1 for pro, got %v", proPreset.ThinkingBudget)
	}
}

func TestGetGeminiImagePresets(t *testing.T) {
	presetIDs := []string{
		"imagen-3",
		"imagen-3-fast",
		"nano-banana-2",
		"nano-banana-2-lite",
		"nano-banana-pro",
		"nano-banana",
	}

	for _, id := range presetIDs {
		preset, ok := config.GetImagePreset(id)
		if !ok {
			t.Fatalf("expected preset %q to exist", id)
		}
		if preset.Type != "gemini" {
			t.Errorf("preset %q expected type gemini, got %q", id, preset.Type)
		}
		if preset.AspectRatio != "16:9" {
			t.Errorf("preset %q expected aspect_ratio 16:9, got %q", id, preset.AspectRatio)
		}
		if preset.PersonGeneration != "ALLOW_ADULT" {
			t.Errorf("preset %q expected person_generation ALLOW_ADULT, got %q", id, preset.PersonGeneration)
		}
	}
}

