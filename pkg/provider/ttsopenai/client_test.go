package ttsopenai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestOpenAITTSSynthesizeShape(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "audio/opus")
		_, _ = w.Write([]byte("fake-opus-audio"))
	}))
	t.Cleanup(server.Close)

	client := NewClient(config.TTSConfig{
		Type: "openai", Endpoint: server.URL, Model: "gpt-4o-mini-tts", APIKey: "k",
	}, "")
	audio, err := client.Synthesize(context.Background(), "Hello", &entity.VoiceConfig{
		VoiceID: "marin", SpeechRate: 1.1,
		Options: map[string]any{"instructions": "breathy, elderly"},
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(audio) != "fake-opus-audio" {
		t.Errorf("audio = %q", audio)
	}
	if body["model"] != "gpt-4o-mini-tts" || body["voice"] != "marin" || body["response_format"] != "opus" {
		t.Errorf("body = %v", body)
	}
	if body["instructions"] != "breathy, elderly" {
		t.Errorf("instructions = %v", body["instructions"])
	}
	if speed, ok := body["speed"].(float64); !ok || speed != 1.1 {
		t.Errorf("speed = %v", body["speed"])
	}
}

func TestOpenAITTSVoiceCatalog(t *testing.T) {
	client := NewClient(config.TTSConfig{Type: "openai"}, "")
	catalog, ok := client.(media.VoiceCatalog)
	if !ok {
		t.Fatal("client does not implement media.VoiceCatalog")
	}
	voices, err := catalog.ListVoices(context.Background())
	if err != nil || len(voices) != 13 {
		t.Fatalf("voices = %d, %v; want 13", len(voices), err)
	}
}

func TestOpenAITTSSharedKeyAndDefaults(t *testing.T) {
	var body map[string]any
	var auth, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		auth = r.Header.Get("Authorization")
		path = r.URL.Path
		_, _ = w.Write([]byte("x"))
	}))
	t.Cleanup(server.Close)

	client := NewClient(config.TTSConfig{Type: "openai", Endpoint: server.URL + "/v1/audio/speech"}, "shared")
	if _, err := client.Synthesize(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if auth != "Bearer shared" {
		t.Errorf("shared key not used: %q", auth)
	}
	if body["voice"] != "alloy" || body["model"] != "gpt-4o-mini-tts" {
		t.Errorf("defaults: %v", body)
	}
	if _, hasSpeed := body["speed"]; hasSpeed {
		t.Errorf("speed sent with zero rate: %v", body)
	}
	if path != "/v1/audio/speech" {
		t.Errorf("path = %q", path)
	}
}

func TestOpenAITTSConfigOptionsFallback(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte("x"))
	}))
	t.Cleanup(server.Close)

	client := NewClient(config.TTSConfig{
		Type: "openai", Endpoint: server.URL, Options: map[string]any{"instructions": "stern"},
	}, "shared")
	if _, err := client.Synthesize(context.Background(), "hi", &entity.VoiceConfig{VoiceID: "nova"}); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if body["instructions"] != "stern" {
		t.Errorf("instructions fallback = %v", body["instructions"])
	}
}

func TestOpenAITTSErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(config.TTSConfig{Type: "openai", Endpoint: server.URL}, "")
	if _, err := client.Synthesize(context.Background(), "hi", nil); err == nil || err.Error() == "" {
		t.Fatal("expected an error")
	}
}
