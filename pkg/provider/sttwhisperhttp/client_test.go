package sttwhisperhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

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
	client := NewHTTPSTTClient(cfg)

	result, err := client.Transcribe(context.Background(), []byte("audio-sample-bytes"))
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	if result != "I enter the dark dungeon." {
		t.Errorf("unexpected transcription: %s", result)
	}
}
