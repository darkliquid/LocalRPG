package ttspiper_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/provider/ttspiper"
)

func TestPiperRegistrationAndDescriptor(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeyTTSPiper))
	if !ok {
		t.Fatalf("provider %q not registered", provider.KeyTTSPiper)
	}
	desc := reg.Descriptor
	if desc.Family != provider.FamilyTTS {
		t.Errorf("Family = %v, want %v", desc.Family, provider.FamilyTTS)
	}
	if desc.Tier != provider.TierLocalServer {
		t.Errorf("Tier = %v, want %v", desc.Tier, provider.TierLocalServer)
	}
	if len(desc.Presets) == 0 || desc.Presets[0].ID != "piper" {
		t.Errorf("unexpected presets: %+v", desc.Presets)
	}
}

func TestPiperBuild(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeyTTSPiper))
	if !ok {
		t.Fatalf("provider %q not registered", provider.KeyTTSPiper)
	}

	built, err := reg.Build(context.Background(), nil)
	if err != nil {
		t.Fatalf("Build with nil failed: %v", err)
	}
	if _, ok := built.(media.TTSClient); !ok {
		t.Fatalf("expected media.TTSClient, got %T", built)
	}

	payload := media.TTSBuildPayload{
		Config: config.TTSConfig{
			Command: "piper",
			Args:    []string{"--test"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	built, err = reg.Build(context.Background(), raw)
	if err != nil {
		t.Fatalf("Build with valid payload failed: %v", err)
	}
	if _, ok := built.(media.TTSClient); !ok {
		t.Fatalf("expected media.TTSClient, got %T", built)
	}

	_, err = reg.Build(context.Background(), []byte("not-json"))
	if err == nil {
		t.Fatal("expected error on invalid JSON, got nil")
	}
}

func TestCLITTSClientSynthesizeSuccess(t *testing.T) {
	client := ttspiper.NewCLITTSClient("sh", []string{"-c", "cat", "--"})
	audio, err := client.Synthesize(context.Background(), "spoken dialog", nil)
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	want := "spoken dialog"
	if string(audio) != want {
		t.Errorf("Synthesize = %q, want %q", string(audio), want)
	}
}

func TestCLITTSClientSynthesizeError(t *testing.T) {
	client := ttspiper.NewCLITTSClient("sh", []string{"-c", "echo 'failed to load onnx model' >&2; exit 1", "--"})
	_, err := client.Synthesize(context.Background(), "hello", nil)
	if err == nil {
		t.Fatal("expected Synthesize to fail, got nil")
	}
	if !strings.Contains(err.Error(), "cli tts error") || !strings.Contains(err.Error(), "failed to load onnx model") {
		t.Errorf("unexpected error format: %v", err)
	}
}
