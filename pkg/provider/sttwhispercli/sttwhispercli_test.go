package sttwhispercli_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/provider/sttwhispercli"
)

func TestWhisperCLIRegistrationAndDescriptor(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeySTTWhisperCLI))
	if !ok {
		t.Fatalf("provider %q not registered", provider.KeySTTWhisperCLI)
	}
	desc := reg.Descriptor
	if desc.Family != provider.FamilySTT {
		t.Errorf("Family = %v, want %v", desc.Family, provider.FamilySTT)
	}
	if desc.Tier != provider.TierLocalServer {
		t.Errorf("Tier = %v, want %v", desc.Tier, provider.TierLocalServer)
	}
	if len(desc.Presets) == 0 || desc.Presets[0].ID != "whisper-cli" {
		t.Errorf("unexpected presets: %+v", desc.Presets)
	}
}

func TestWhisperCLIBuild(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeySTTWhisperCLI))
	if !ok {
		t.Fatalf("provider %q not registered", provider.KeySTTWhisperCLI)
	}

	built, err := reg.Build(context.Background(), nil)
	if err != nil {
		t.Fatalf("Build with nil failed: %v", err)
	}
	if _, ok := built.(media.STTClient); !ok {
		t.Fatalf("expected media.STTClient, got %T", built)
	}

	cfg := config.STTConfig{
		Command: "cat",
		Args:    []string{"-u"},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	built, err = reg.Build(context.Background(), raw)
	if err != nil {
		t.Fatalf("Build with valid config failed: %v", err)
	}
	if _, ok := built.(media.STTClient); !ok {
		t.Fatalf("expected media.STTClient, got %T", built)
	}

	_, err = reg.Build(context.Background(), []byte("not-json"))
	if err == nil {
		t.Fatal("expected error on invalid JSON, got nil")
	}
}

func TestCLISTTClientTranscribeSuccess(t *testing.T) {
	client := sttwhispercli.NewCLISTTClient("sh", []string{"-c", "cat", "--"})
	text, err := client.Transcribe(context.Background(), []byte("recognized text from audio"))
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	want := "recognized text from audio"
	if text != want {
		t.Errorf("Transcribe = %q, want %q", text, want)
	}
}

func TestCLISTTClientTranscribeError(t *testing.T) {
	client := sttwhispercli.NewCLISTTClient("sh", []string{"-c", "echo 'whisper fatal error' >&2; exit 1", "--"})
	_, err := client.Transcribe(context.Background(), []byte("audio"))
	if err == nil {
		t.Fatal("expected Transcribe to fail, got nil")
	}
	if !strings.Contains(err.Error(), "cli stt error") || !strings.Contains(err.Error(), "whisper fatal error") {
		t.Errorf("unexpected error format: %v", err)
	}
}
