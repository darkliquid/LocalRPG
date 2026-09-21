package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestHTTPSTTClient_TranscribesMultipartAudio(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Authorization Bearer test-key, got %s", auth)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected file field: %v", err)
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		if string(content) != "audio-sample-bytes" {
			t.Errorf("unexpected file content: %s", string(content))
		}
		if model := r.FormValue("model"); model != "whisper-1" {
			t.Errorf("expected model whisper-1, got %s", model)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "I enter the dark dungeon."})
	}))
	defer ts.Close()

	cfg := config.STTConfig{
		Type:     "http",
		Endpoint: ts.URL,
		Model:    "whisper-1",
		APIKey:   "test-key",
	}
	client, err := media.NewSTTClient(cfg)
	if err != nil {
		t.Fatalf("NewSTTClient failed: %v", err)
	}

	result, err := client.Transcribe(context.Background(), []byte("audio-sample-bytes"))
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	if result != "I enter the dark dungeon." {
		t.Errorf("unexpected transcription: %s", result)
	}
}
