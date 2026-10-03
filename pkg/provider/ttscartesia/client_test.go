package ttscartesia

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func newTestClient(t *testing.T, baseURL, apiKey string) *CartesiaTTSClient {
	t.Helper()
	cfg := config.TTSConfig{
		Type:        "builtin",
		BuiltinName: "cartesia",
		APIKey:      apiKey,
	}
	client, err := NewCartesiaTTSClient(cfg, "")
	if err != nil {
		t.Fatalf("NewCartesiaTTSClient failed: %v", err)
	}
	client.baseURL = baseURL
	return client
}

func TestSynthesize(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cartesia-Version") != cartesiaAPIVersion {
			t.Errorf("missing or wrong Cartesia-Version: %s", r.Header.Get("Cartesia-Version"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("wrong auth: %s", r.Header.Get("Authorization"))
		}
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if req["model_id"] != "sonic-3.6" {
			t.Errorf("model_id = %v, want sonic-3.6", req["model_id"])
		}
		if req["transcript"] != "Hello there!" {
			t.Errorf("transcript = %v, want 'Hello there!'", req["transcript"])
		}
		voiceMap, ok := req["voice"].(map[string]interface{})
		if !ok || voiceMap["id"] != "db6b0ed5-d5d3-463d-ae85-518a07d3c2b4" {
			t.Errorf("voice.id = %v, want default voice id", req["voice"])
		}
		genCfg, ok := req["generation_config"].(map[string]interface{})
		if !ok || genCfg["speed"] != 1.2 {
			t.Errorf("generation_config.speed = %v, want 1.2", req["generation_config"])
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Write([]byte("RIFF1234WAVEfmt "))
	}))
	defer ts.Close()

	client := newTestClient(t, ts.URL, "test-key")
	voice := &entity.VoiceConfig{VoiceID: "db6b0ed5-d5d3-463d-ae85-518a07d3c2b4", SpeechRate: 1.2}
	audio, err := client.Synthesize(context.Background(), "Hello there!", voice)
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if string(audio) != "RIFF1234WAVEfmt " {
		t.Errorf("unexpected audio bytes: %q", string(audio))
	}
	usage := client.LastUsage()
	if usage.Characters != len([]rune("Hello there!")) || usage.Requests != 1 {
		t.Errorf("unexpected usage: %+v", usage)
	}
}

func TestVoiceCatalog(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"data": [
					{
						"id": "v1",
						"name": "Skylar",
						"tagline": "Friendly",
						"description": "A guide",
						"gender": "feminine",
						"accents": [{"locale": "en-US", "is_native": true}],
						"preview_file_url": "https://example.com/v1.mp3"
					}
				],
				"has_more": true,
				"next_page": "cursor-2"
			}`))
		} else {
			if r.URL.Query().Get("starting_after") != "cursor-2" {
				t.Errorf("expected starting_after=cursor-2, got %s", r.URL.Query().Get("starting_after"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"data": [
					{
						"id": "v2",
						"name": "Daniel",
						"gender": "masculine",
						"accents": [{"locale": "en-US", "is_native": true}]
					}
				],
				"has_more": false
			}`))
		}
	}))
	defer ts.Close()

	client := newTestClient(t, ts.URL, "test-key")
	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices failed: %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(voices))
	}
	if voices[0].ID != "v1" || voices[0].Gender != "female" || voices[0].PreviewURL != "https://example.com/v1.mp3" {
		t.Errorf("voice 0 mapped incorrectly: %+v", voices[0])
	}
	if voices[1].ID != "v2" || voices[1].Gender != "male" {
		t.Errorf("voice 1 mapped incorrectly: %+v", voices[1])
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
		w.Header().Set("Content-Type", "audio/wav")
		w.Write([]byte("RIFFWAV"))
	}))
	defer ts.Close()

	client := newTestClient(t, ts.URL, "test-key")
	_, err := client.Synthesize(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestStructuredError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error_code":"invalid_credentials","title":"Unauthorized","message":"Invalid API key provided"}`))
	}))
	defer ts.Close()

	client := newTestClient(t, ts.URL, "bad-key")
	_, err := client.Synthesize(context.Background(), "test", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	expected := "cartesia: invalid_credentials - Invalid API key provided"
	if err.Error() != expected {
		t.Errorf("got error %q, want %q", err.Error(), expected)
	}
}

func TestSpeechCueCapabilities(t *testing.T) {
	client := newTestClient(t, "https://api.cartesia.ai", "test-key")
	caps := client.SpeechCueCapabilities()
	if !caps.AudioTags {
		t.Error("expected AudioTags = true")
	}
	if caps.MarkdownEmphasis {
		t.Error("expected MarkdownEmphasis = false")
	}
}
