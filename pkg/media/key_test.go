package media_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestTTSKeyFor(t *testing.T) {
	tests := []struct {
		cfg  config.TTSConfig
		want provider.Key
		ok   bool
	}{
		{config.TTSConfig{Type: "gemini"}, provider.KeyTTSGemini, true},
		{config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"}, provider.KeyTTSElevenLabs, true},
		{config.TTSConfig{Type: "builtin", BuiltinName: "sherpa-onnx"}, provider.KeyTTSSherpaONNX, true},
		{config.TTSConfig{Type: "builtin", BuiltinName: "kokoro"}, provider.KeyTTSSherpaONNX, true},
		{config.TTSConfig{Type: "builtin", BuiltinName: "native-os"}, provider.KeyTTSNativeOS, true},
		{config.TTSConfig{Type: "disabled"}, "", false},
	}
	for _, tc := range tests {
		got, ok := media.TTSKeyFor(tc.cfg)
		if ok != tc.ok {
			t.Errorf("TTSKeyFor(%+v) ok = %v, want %v", tc.cfg, ok, tc.ok)
			continue
		}
		if tc.want != "" && got != tc.want {
			t.Errorf("TTSKeyFor(%+v) = %q, want %q", tc.cfg, got, tc.want)
		}
	}
}

func TestTTSKeyForHTTPIsAnInstance(t *testing.T) {
	got, ok := media.TTSKeyFor(config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880"})
	if !ok {
		t.Fatal("expected a key")
	}
	if got != "tts:http@localhost:8880" {
		t.Errorf("got %q, want tts:http@localhost:8880", got)
	}
	if got.Parent() != provider.KeyTTSHTTP {
		t.Errorf("parent = %q, want %q", got.Parent(), provider.KeyTTSHTTP)
	}
}

func TestTTSKeyForCLIIsAnInstance(t *testing.T) {
	got, ok := media.TTSKeyFor(config.TTSConfig{Type: "cli", Command: "/usr/bin/piper"})
	if !ok || got != "tts:piper@piper" {
		t.Errorf("TTSKeyFor(cli) = %q/%v, want tts:piper@piper", got, ok)
	}
}

func TestSTTKeyForSkipsBrowserOnly(t *testing.T) {
	if _, ok := media.STTKeyFor(config.STTConfig{Type: "web-speech"}); ok {
		t.Error("web-speech must have no key")
	}
	if _, ok := media.STTKeyFor(config.STTConfig{Type: "builtin"}); ok {
		t.Error("the server-side builtin echo STT must have no key")
	}
	got, ok := media.STTKeyFor(config.STTConfig{Type: "http", Endpoint: "http://localhost:8000"})
	if !ok || got != "stt:whisper-http@localhost:8000" {
		t.Errorf("STTKeyFor(http) = %q/%v, want stt:whisper-http@localhost:8000", got, ok)
	}
	if got, ok := media.STTKeyFor(config.STTConfig{Type: "cli", Command: "whisper-cli"}); !ok || got != "stt:whisper-cli@whisper-cli" {
		t.Errorf("STTKeyFor(cli) = %q/%v", got, ok)
	}
}

func TestImageKeyFor(t *testing.T) {
	tests := []struct {
		cfg  config.ImageConfig
		want provider.Key
		ok   bool
	}{
		{config.ImageConfig{Type: "gemini"}, provider.KeyImageGemini, true},
		{config.ImageConfig{Type: "builtin", BuiltinName: "procedural-art"}, provider.KeyImageProceduralArt, true},
		{config.ImageConfig{Type: "http", Endpoint: "http://127.0.0.1:8188"}, "image:http@127.0.0.1:8188", true},
		{config.ImageConfig{Type: "comfyui", Endpoint: "http://127.0.0.1:8188"}, "image:http@127.0.0.1:8188", true},
		{config.ImageConfig{Type: "disabled"}, "", false},
	}
	for _, tc := range tests {
		got, ok := media.ImageKeyFor(tc.cfg)
		if ok != tc.ok || (tc.want != "" && got != tc.want) {
			t.Errorf("ImageKeyFor(%+v) = %q/%v, want %q/%v", tc.cfg, got, ok, tc.want, tc.ok)
		}
	}
}
