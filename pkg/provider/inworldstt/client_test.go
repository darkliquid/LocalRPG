package inworldstt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestInworldSTTTranscribesWAVAndOpus(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"transcription": map[string]interface{}{"transcript": "Hello from Inworld STT", "isFinal": true},
		})
	}))
	defer server.Close()

	client, err := NewInworldSTTClient(config.STTConfig{Endpoint: server.URL, APIKey: "custom-stt-key"}, "shared-key")
	if err != nil {
		t.Fatalf("NewInworldSTTClient: %v", err)
	}

	transcript, err := client.Transcribe(context.Background(), media.GenerateToneWAV(440, 0.1))
	if err != nil {
		t.Fatalf("Transcribe WAV: %v", err)
	}
	if transcript != "Hello from Inworld STT" {
		t.Errorf("transcript = %q, want Hello from Inworld STT", transcript)
	}
	if gotAuth != "Basic custom-stt-key" {
		t.Errorf("Authorization = %q, want Basic custom-stt-key", gotAuth)
	}
	cfg, _ := gotBody["transcribe_config"].(map[string]interface{})
	if cfg["audio_encoding"] != "LINEAR16" {
		t.Errorf("audio_encoding = %v, want LINEAR16 for a WAV", cfg["audio_encoding"])
	}
	if cfg["model_id"] != "inworld/inworld-stt-1" {
		t.Errorf("model_id = %v, want inworld/inworld-stt-1", cfg["model_id"])
	}
	if client.LastUsage().Requests != 1 {
		t.Errorf("LastUsage().Requests = %d, want 1", client.LastUsage().Requests)
	}

	if _, err := client.Transcribe(context.Background(), []byte("OggSfake-opus-data")); err != nil {
		t.Fatalf("Transcribe Opus: %v", err)
	}
	cfg, _ = gotBody["transcribe_config"].(map[string]interface{})
	if cfg["audio_encoding"] != "OGG_OPUS" {
		t.Errorf("audio_encoding = %v, want OGG_OPUS for an Ogg stream", cfg["audio_encoding"])
	}
}

func TestInworldSTTUsesSharedKey(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"transcription": map[string]interface{}{"transcript": "ok"},
		})
	}))
	defer server.Close()

	client, err := NewInworldSTTClient(config.STTConfig{Endpoint: server.URL}, "shared-key")
	if err != nil {
		t.Fatalf("NewInworldSTTClient: %v", err)
	}
	if _, err := client.Transcribe(context.Background(), []byte("OggSaudio")); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if gotAuth != "Basic shared-key" {
		t.Errorf("Authorization = %q, want Basic shared-key", gotAuth)
	}
}

func TestInworldSTTEncodesBase64Content(t *testing.T) {
	var gotBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"transcription": map[string]interface{}{"transcript": "ok"},
		})
	}))
	defer server.Close()

	client, err := NewInworldSTTClient(config.STTConfig{Endpoint: server.URL, APIKey: "k"}, "")
	if err != nil {
		t.Fatalf("NewInworldSTTClient: %v", err)
	}
	if _, err := client.Transcribe(context.Background(), []byte("ID3mp3-audio")); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	audio, _ := gotBody["audio_data"].(map[string]interface{})
	content, _ := audio["content"].(string)
	if decoded, err := base64.StdEncoding.DecodeString(content); err != nil || string(decoded) != "ID3mp3-audio" {
		t.Errorf("audio_data.content did not round-trip the MP3 bytes (err %v)", err)
	}
}

func TestInworldSTTMissingKey(t *testing.T) {
	os.Unsetenv("INWORLD_API_KEY")
	_, err := NewInworldSTTClient(config.STTConfig{}, "")
	if err == nil || !strings.Contains(err.Error(), "inworld: an API key is required") {
		t.Fatalf("expected missing key error, got: %v", err)
	}
}
