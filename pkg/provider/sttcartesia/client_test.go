package sttcartesia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func newTestClient(t *testing.T, baseURL, apiKey string) *CartesiaSTTClient {
	t.Helper()
	cfg := config.STTConfig{
		Type:        "builtin",
		BuiltinName: "cartesia",
		APIKey:      apiKey,
	}
	client, err := NewCartesiaSTTClient(cfg, "")
	if err != nil {
		t.Fatalf("NewCartesiaSTTClient failed: %v", err)
	}
	client.baseURL = baseURL
	return client
}

func TestTranscribe(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cartesia-Version") != cartesiaAPIVersion {
			t.Errorf("wrong version: %s", r.Header.Get("Cartesia-Version"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("wrong auth: %s", r.Header.Get("Authorization"))
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if r.FormValue("model") != "ink-whisper" {
			t.Errorf("model = %s, want ink-whisper", r.FormValue("model"))
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("missing file part: %v", err)
		}
		file.Close()

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"type":"transcript","text":"Cast magic missile","duration":3.2}`))
	}))
	defer ts.Close()

	client := newTestClient(t, ts.URL, "test-key")
	text, err := client.Transcribe(context.Background(), []byte("fake-audio-bytes"))
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	if text != "Cast magic missile" {
		t.Errorf("got %q, want 'Cast magic missile'", text)
	}
	usage := client.LastUsage()
	if usage.Requests != 1 {
		t.Errorf("expected 1 request, got %d", usage.Requests)
	}
}

func TestRateLimitRetry(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error_code":"concurrency_limited","message":"slow down"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"type":"transcript","text":"Retried successfully"}`))
	}))
	defer ts.Close()

	client := newTestClient(t, ts.URL, "test-key")
	text, err := client.Transcribe(context.Background(), []byte("audio"))
	if err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if text != "Retried successfully" {
		t.Errorf("got %q, want 'Retried successfully'", text)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestStructuredError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error_code":"invalid_audio","title":"Bad Request","message":"Could not decode audio stream"}`))
	}))
	defer ts.Close()

	client := newTestClient(t, ts.URL, "test-key")
	_, err := client.Transcribe(context.Background(), []byte("garbage"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	expected := "cartesia: invalid_audio - Could not decode audio stream"
	if err.Error() != expected {
		t.Errorf("got error %q, want %q", err.Error(), expected)
	}
}
