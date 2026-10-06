package config

import "testing"

func TestPurposeFamilyAndKnown(t *testing.T) {
	if PurposeFamily(PurposeNarrator) != "tts" || PurposeFamily(PurposeNPC) != "tts" {
		t.Fatal("narrator and npc are tts purposes")
	}
	if PurposeFamily(PurposeScene) != "image" || PurposeFamily(PurposePortrait) != "image" {
		t.Fatal("scene and portrait are image purposes")
	}
	if PurposeFamily(PurposePlaceholder) != "image" {
		t.Fatal("placeholder is an image purpose")
	}
	if KnownPurpose("bogus") {
		t.Fatal("bogus is not a known purpose")
	}
}

func TestPurposeResolution(t *testing.T) {
	m := MediaConfig{
		TTS:            TTSConfig{BuiltinName: "elevenlabs"},
		TTSProviders:   map[string]TTSConfig{"npc": {BuiltinName: "sherpa-onnx"}},
		Image:          ImageConfig{BuiltinName: "procedural-art"},
		ImageProviders: map[string]ImageConfig{"hero": {BuiltinName: "gemini"}},
		Purposes:       map[string]string{"npc": "npc", "portrait": "hero"},
	}
	if m.TTSForPurpose(PurposeNarrator).BuiltinName != "elevenlabs" {
		t.Fatal("unset narrator should be the default")
	}
	if m.TTSForPurpose(PurposeNPC).BuiltinName != "sherpa-onnx" {
		t.Fatal("npc should resolve to the npc entry")
	}
	if m.ImageForPurpose(PurposePortrait).BuiltinName != "gemini" {
		t.Fatal("portrait should resolve to hero")
	}
	if m.ImageForPurpose(PurposeScene).BuiltinName != "procedural-art" {
		t.Fatal("unset scene should be the default")
	}
}

func TestPurposeResolutionWithNoMap(t *testing.T) {
	m := MediaConfig{TTS: TTSConfig{BuiltinName: "x"}, Image: ImageConfig{BuiltinName: "y"}}
	if m.TTSForPurpose(PurposeNPC).BuiltinName != "x" || m.ImageForPurpose(PurposeScene).BuiltinName != "y" {
		t.Fatal("an empty purposes map must resolve everything to the default")
	}
}

func TestValidateRejectsBadPurposes(t *testing.T) {
	bad := DefaultConfig()
	bad.Media.Purposes = map[string]string{"bogus": "default"}
	if len(bad.Validate()) == 0 {
		t.Fatal("an unknown purpose should be rejected")
	}
	bad = DefaultConfig()
	bad.Media.Purposes = map[string]string{"npc": "gone"}
	if len(bad.Validate()) == 0 {
		t.Fatal("a missing provider should be rejected")
	}
	bad = DefaultConfig()
	bad.Media.ImageProviders = map[string]ImageConfig{"hero": {}}
	bad.Media.Purposes = map[string]string{"narrator": "hero"}
	if len(bad.Validate()) == 0 {
		t.Fatal("a cross-family provider should be rejected")
	}
	ok := DefaultConfig()
	ok.Media.TTSProviders = map[string]TTSConfig{"npc": {Type: "builtin"}}
	ok.Media.Purposes = map[string]string{"npc": "npc"}
	if problems := ok.Validate(); len(problems) != 0 {
		t.Fatalf("a valid purpose should validate cleanly: %v", problems)
	}
}
