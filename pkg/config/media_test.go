package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTTSForResolution(t *testing.T) {
	m := MediaConfig{
		TTS: TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"},
		TTSProviders: map[string]TTSConfig{
			"npc": {Type: "builtin", BuiltinName: "sherpa-onnx"},
		},
	}
	if m.TTSFor("").BuiltinName != "elevenlabs" {
		t.Fatal("empty name should resolve to the default")
	}
	if m.TTSFor("default").BuiltinName != "elevenlabs" {
		t.Fatal("the reserved name should resolve to the default")
	}
	if m.TTSFor("npc").BuiltinName != "sherpa-onnx" {
		t.Fatal("a named entry should resolve to itself")
	}
	if m.TTSFor("gone").BuiltinName != "elevenlabs" {
		t.Fatal("an unknown name should fall back to the default")
	}
	names := m.TTSNames()
	if len(names) != 2 || names[0] != "default" {
		t.Fatalf("names = %v, want default first", names)
	}
}

func TestMediaProvidersRoundTrip(t *testing.T) {
	in := []byte("media:\n  tts:\n    type: builtin\n  tts_providers:\n    npc:\n      type: builtin\n")
	var cfg Config
	if err := yaml.Unmarshal(in, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Media.TTSFor("npc").Type != "builtin" {
		t.Fatal("named entry did not survive the round trip")
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "tts_providers") {
		t.Fatal("tts_providers was dropped on save")
	}
}

func TestValidateRejectsBadProviderNames(t *testing.T) {
	bad := DefaultConfig()
	bad.Media.TTSProviders = map[string]TTSConfig{"default": {}}
	if len(bad.Validate()) == 0 {
		t.Fatal("the reserved name should be rejected")
	}
	bad = DefaultConfig()
	bad.Media.TTSProviders = map[string]TTSConfig{"Bad Name": {}}
	if len(bad.Validate()) == 0 {
		t.Fatal("a malformed name should be rejected")
	}
	ok := DefaultConfig()
	ok.Media.TTSProviders = map[string]TTSConfig{"npc": {Type: "builtin"}}
	if problems := ok.Validate(); len(problems) != 0 {
		t.Fatalf("a valid name should validate cleanly: %v", problems)
	}
}

func TestConfigWithoutProvidersIsUnchanged(t *testing.T) {
	cfg := DefaultConfig()
	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "_providers") {
		t.Fatalf("an empty map should be omitted, got:\n%s", out)
	}
}
