package media_test

import (
	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider/ttselevenlabs"
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

func TestNewTTSClientBuildsElevenLabs(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "")
	_, err := media.NewTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"})
	if !errors.Is(err, ttselevenlabs.ErrMissingAPIKey) {
		t.Fatalf("err = %v, want ErrMissingAPIKey when no key is set", err)
	}

	client, err := media.NewTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs", APIKey: "abc"})
	if err != nil {
		t.Fatalf("NewTTSClient: %v", err)
	}
	if _, ok := client.(*ttselevenlabs.ElevenLabsTTSClient); !ok {
		t.Errorf("client = %T, want *ttselevenlabs.ElevenLabsTTSClient", client)
	}
}

func TestNewImageClientBuildsGemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	client, err := media.NewImageClient(config.ImageConfig{
		Type:  "gemini",
		Model: "imagen-3.0-generate-002",
	})
	if err != nil {
		t.Fatalf("NewImageClient failed for gemini: %v", err)
	}
	if client == nil {
		t.Fatalf("expected non-nil image client")
	}

	builtinClient, err := media.NewImageClient(config.ImageConfig{
		Type:        "builtin",
		BuiltinName: "gemini",
		Model:       "gemini-3.1-flash-image",
	})
	if err != nil {
		t.Fatalf("NewImageClient failed for builtin gemini: %v", err)
	}
	if builtinClient == nil {
		t.Fatalf("expected non-nil builtin gemini image client")
	}
}

func TestNewTTSClientBuildsGemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "env-key")

	// 1. type: "gemini"
	client, err := media.NewTTSClientWithSharedKey(config.TTSConfig{
		Type: "gemini",
	}, "")
	if err != nil {
		t.Fatalf("expected gemini client to build, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	// 2. type: "builtin", builtin_name: "gemini"
	client, err = media.NewTTSClientWithSharedKey(config.TTSConfig{
		Type:        "builtin",
		BuiltinName: "gemini",
	}, "")
	if err != nil {
		t.Fatalf("expected builtin gemini client to build, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
