package config

import (
	"reflect"
	"testing"
)

func TestApplyOfflinePreset(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Paths.Games = "/custom"
	changes := ApplyOfflinePreset(cfg, "native-os")
	if cfg.Agents.Roles["gm"].BuiltinName != "narrative-oracle" {
		t.Fatalf("gm = %+v", cfg.Agents.Roles["gm"])
	}
	if cfg.Media.TTS.BuiltinName != "native-os" || cfg.Media.Image.BuiltinName != "procedural-art" {
		t.Fatalf("media = %+v", cfg.Media)
	}
	if cfg.Paths.Games != "/custom" {
		t.Fatal("the preset must not touch paths")
	}
	if len(changes) == 0 {
		t.Fatal("the preset should report its changes")
	}
}

func TestApplyOfflinePresetIdempotent(t *testing.T) {
	cfg1 := DefaultConfig()
	ApplyOfflinePreset(cfg1, "native-os")
	cfg2 := DefaultConfig()
	ApplyOfflinePreset(cfg2, "native-os")
	changes2 := ApplyOfflinePreset(cfg2, "native-os")
	if !reflect.DeepEqual(cfg1, cfg2) {
		t.Fatal("applying offline preset twice should yield the same config")
	}
	if len(changes2) != 0 {
		t.Fatalf("applying offline preset when already applied should report no changes, got %v", changes2)
	}
}

func TestApplyOfflinePresetSherpa(t *testing.T) {
	cfg := DefaultConfig()
	ApplyOfflinePreset(cfg, "sherpa-onnx")
	if cfg.Media.TTS.BuiltinName != "sherpa-onnx" {
		t.Fatalf("tts = %+v", cfg.Media.TTS)
	}
}
