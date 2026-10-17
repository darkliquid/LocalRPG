package sttwebspeech_test

import (
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	_ "github.com/darkliquid/localrpg/pkg/provider/sttwebspeech"
)

func TestSTTWebSpeechRegistrationAndDescriptor(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeySTTWebSpeech))
	if !ok {
		t.Fatalf("provider %q not registered", provider.KeySTTWebSpeech)
	}
	desc := reg.Descriptor
	if desc.Family != provider.FamilySTT {
		t.Errorf("Family = %v, want %v", desc.Family, provider.FamilySTT)
	}
	if desc.Tier != provider.TierOfflineBasic {
		t.Errorf("Tier = %v, want %v", desc.Tier, provider.TierOfflineBasic)
	}
	if len(desc.Features) == 0 || desc.Features[0] != provider.FeatureOffline {
		t.Errorf("expected FeatureOffline, got %v", desc.Features)
	}
	if len(desc.Presets) == 0 || desc.Presets[0].ID != "web-speech" {
		t.Errorf("unexpected presets: %+v", desc.Presets)
	}
}

func TestSTTWebSpeechBuildAndTranscribe(t *testing.T) {
	reg, ok := provider.Lookup(string(provider.KeySTTWebSpeech))
	if !ok {
		t.Fatalf("provider %q not registered", provider.KeySTTWebSpeech)
	}

	built, err := reg.Build(context.Background(), nil)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	client, ok := built.(media.STTClient)
	if !ok {
		t.Fatalf("expected media.STTClient, got %T", built)
	}

	_, err = client.Transcribe(context.Background(), []byte("audio-bytes"))
	if !errors.Is(err, media.ErrWebSpeechIsClientSide) {
		t.Fatalf("expected ErrWebSpeechIsClientSide, got: %v", err)
	}
}
