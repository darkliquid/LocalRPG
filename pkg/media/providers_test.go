package media_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestMediaProviders_Disabled(t *testing.T) {
	ttsClient, err := media.NewTTSClient(config.TTSConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = ttsClient.Synthesize(context.Background(), "Hello", nil)
	if !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected media.ErrProviderDisabled, got %v", err)
	}

	sttClient, err := media.NewSTTClient(config.STTConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = sttClient.Transcribe(context.Background(), []byte("audio"))
	if !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected media.ErrProviderDisabled, got %v", err)
	}

	imgClient, err := media.NewImageClient(config.ImageConfig{Type: "disabled"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = imgClient.GenerateImage(context.Background(), "A dark tower")
	if !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected media.ErrProviderDisabled, got %v", err)
	}
}

func TestMediaProviders_BuiltinEcho(t *testing.T) {
	ttsClient, err := media.NewTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bytes, err := ttsClient.Synthesize(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("synthesize failed: %v", err)
	}
	if len(bytes) == 0 {
		t.Errorf("expected non-empty audio bytes from builtin echo")
	}

	sttClient, err := media.NewSTTClient(config.STTConfig{Type: "builtin", BuiltinName: "echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text, err := sttClient.Transcribe(context.Background(), []byte("test audio"))
	if err != nil {
		t.Fatalf("transcribe failed: %v", err)
	}
	if text == "" {
		t.Errorf("expected non-empty transcript from builtin echo")
	}

	imgClient, err := media.NewImageClient(config.ImageConfig{Type: "builtin", BuiltinName: "echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	imgBytes, err := imgClient.GenerateImage(context.Background(), "a dragon")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if len(imgBytes) == 0 {
		t.Errorf("expected non-empty image bytes from builtin echo")
	}
}

func TestSceneImageClientAlwaysProducesArtByDefault(t *testing.T) {
	// No provider configured, fallback on: the built-in generator stands in.
	client, err := media.NewSceneImageClient(config.ImageConfig{Type: "disabled", BuiltinFallback: true})
	if err != nil {
		t.Fatalf("NewSceneImageClient failed: %v", err)
	}

	data, err := client.GenerateImage(context.Background(), "moonlit harbour")
	if err != nil {
		t.Fatalf("expected the built-in generator to stand in: %v", err)
	}
	if !bytes.Contains(bytes.ToLower(data), []byte("<svg")) {
		t.Errorf("expected SVG art from the built-in generator")
	}
}

func TestSceneImageClientRespectsAFailedProvider(t *testing.T) {
	client, err := media.NewSceneImageClient(config.ImageConfig{
		Type:            "http",
		Endpoint:        "http://127.0.0.1:1/unreachable",
		BuiltinFallback: true,
	})
	if err != nil {
		t.Fatalf("NewSceneImageClient failed: %v", err)
	}

	data, err := client.GenerateImage(context.Background(), "moonlit harbour")
	if err != nil {
		t.Fatalf("expected the built-in generator to cover a provider failure: %v", err)
	}
	if !bytes.Contains(bytes.ToLower(data), []byte("<svg")) {
		t.Errorf("expected fallback art")
	}
}

func TestSceneImageClientWithoutFallbackStaysDisabled(t *testing.T) {
	client, err := media.NewSceneImageClient(config.ImageConfig{Type: "disabled", BuiltinFallback: false})
	if err != nil {
		t.Fatalf("NewSceneImageClient failed: %v", err)
	}

	if _, err := client.GenerateImage(context.Background(), "moonlit harbour"); !errors.Is(err, media.ErrProviderDisabled) {
		t.Errorf("expected media.ErrProviderDisabled, got %v", err)
	}
}
