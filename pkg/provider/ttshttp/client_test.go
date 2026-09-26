package ttshttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider/ttshttp"
)

func TestResolveHTTPEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSpeech string
		wantVoices string
	}{
		{
			name:       "bare base url",
			input:      "http://localhost:8880",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "base url with trailing slash",
			input:      "http://localhost:8880/",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "full speech endpoint",
			input:      "http://localhost:8880/v1/audio/speech",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "v1 endpoint",
			input:      "http://localhost:8880/v1",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "alltalk endpoint preserved",
			input:      "http://localhost:7851/api/tts-generate",
			wantSpeech: "http://localhost:7851/api/tts-generate",
			wantVoices: "",
		},
		{
			name:       "empty endpoint",
			input:      "",
			wantSpeech: "",
			wantVoices: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpeech, gotVoices := ttshttp.ResolveHTTPEndpoints(tt.input)
			if gotSpeech != tt.wantSpeech {
				t.Errorf("speechURL = %q, want %q", gotSpeech, tt.wantSpeech)
			}
			if gotVoices != tt.wantVoices {
				t.Errorf("voicesURL = %q, want %q", gotVoices, tt.wantVoices)
			}
		})
	}
}

func TestLiveKokoroFastAPI(t *testing.T) {
	resp, err := http.Get("http://localhost:8880/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skip("skipping live Kokoro-FastAPI test: server not reachable on http://localhost:8880")
	}
	resp.Body.Close()

	// Test 1: Base URL
	client := ttshttp.NewHTTPTTSClient(config.TTSConfig{
		Type:     "http",
		Endpoint: "http://localhost:8880",
		Model:    "kokoro",
	})

	catalog, ok := client.(media.VoiceCatalog)
	if !ok {
		t.Fatalf("client does not implement media.VoiceCatalog")
	}

	voices, err := catalog.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices failed: %v", err)
	}
	if len(voices) < 10 {
		t.Fatalf("expected at least 10 voices from live Kokoro-FastAPI, got %d", len(voices))
	}

	// Verify af_heart is present and parsed with metadata
	var foundHeart bool
	for _, v := range voices {
		if v.ID == "af_heart" {
			foundHeart = true
			if v.Gender != "female" {
				t.Errorf("af_heart gender = %q, want female", v.Gender)
			}
			if v.Language != "en-US" {
				t.Errorf("af_heart language = %q, want en-US", v.Language)
			}
			if v.Accent != "American" {
				t.Errorf("af_heart accent = %q, want American", v.Accent)
			}
			break
		}
	}
	if !foundHeart {
		t.Errorf("af_heart voice not found in live voices list")
	}

	// Test 2: Synthesis with base URL
	audio, err := client.Synthesize(context.Background(), "Live Kokoro test.", &entity.VoiceConfig{
		VoiceID: "af_heart",
	})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if len(audio) < 100 {
		t.Fatalf("expected audio bytes, got %d bytes", len(audio))
	}

	// Test 3: Legacy URL with /v1/audio/speech
	clientLegacy := ttshttp.NewHTTPTTSClient(config.TTSConfig{
		Type:     "http",
		Endpoint: "http://localhost:8880/v1/audio/speech",
		Model:    "kokoro",
	})
	catalogLegacy, ok := clientLegacy.(media.VoiceCatalog)
	if !ok {
		t.Fatalf("legacy client does not implement media.VoiceCatalog")
	}
	legacyVoices, err := catalogLegacy.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices (legacy) failed: %v", err)
	}
	if len(legacyVoices) != len(voices) {
		t.Errorf("legacy voice count = %d, want %d", len(legacyVoices), len(voices))
	}
}

func TestHTTPTTSClient_AdaptsAllTalkPayload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["text_input"] != "Greetings, traveler." {
			t.Errorf("expected text_input, got %v", req["text_input"])
		}
		if req["character_voice_gen"] != "elder_sage" {
			t.Errorf("expected character_voice_gen elder_sage, got %v", req["character_voice_gen"])
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFF1234WAVEfmt audio-clip"))
	}))
	defer ts.Close()

	client := ttshttp.NewHTTPTTSClient(config.TTSConfig{
		Type:     "http",
		Endpoint: ts.URL + "/api/tts-generate",
	})

	data, err := client.Synthesize(context.Background(), "Greetings, traveler.", &entity.VoiceConfig{VoiceID: "elder_sage"})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if !strings.HasPrefix(string(data), "RIFF") {
		t.Errorf("unexpected audio data: %s", string(data))
	}
}

func TestHTTPTTSClientSynthesizeKokoro(t *testing.T) {
	var receivedPath string
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("fake-mp3-audio"))
	}))
	defer server.Close()

	// Given a base URL (without /v1/audio/speech)
	client := ttshttp.NewHTTPTTSClient(config.TTSConfig{
		Type:     "http",
		Endpoint: server.URL,
		Model:    "kokoro",
	})

	audio, err := client.Synthesize(context.Background(), "Hello test", &entity.VoiceConfig{
		VoiceID:    "af_bella",
		SpeechRate: 1.25,
	})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if string(audio) != "fake-mp3-audio" {
		t.Errorf("unexpected audio: %q", string(audio))
	}
	if receivedPath != "/v1/audio/speech" {
		t.Errorf("receivedPath = %q, want /v1/audio/speech", receivedPath)
	}
	if receivedBody["model"] != "kokoro" {
		t.Errorf("model = %v, want kokoro", receivedBody["model"])
	}
	if receivedBody["voice"] != "af_bella" {
		t.Errorf("voice = %v, want af_bella", receivedBody["voice"])
	}
	if receivedBody["allow_voice_tags"] != true {
		t.Errorf("allow_voice_tags = %v, want true", receivedBody["allow_voice_tags"])
	}
	if receivedBody["response_format"] != "mp3" {
		t.Errorf("response_format = %v, want mp3", receivedBody["response_format"])
	}
	if speed, ok := receivedBody["speed"].(float64); !ok || speed != 1.25 {
		t.Errorf("speed = %v, want 1.25", receivedBody["speed"])
	}
}

func TestHTTPTTSClientListVoices(t *testing.T) {
	t.Run("kokoro fastapi voices response object", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/audio/voices" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"voices": [
					{
						"id": "af_heart",
						"name": "af_heart",
						"target_quality": "A",
						"overall_grade": "A"
					},
					{
						"id": "bm_george",
						"name": "bm_george",
						"target_quality": "B",
						"overall_grade": "C"
					},
					{
						"id": "zf_xiaobei",
						"name": "zf_xiaobei"
					}
				],
				"default_voice": "af_heart"
			}`))
		}))
		defer server.Close()

		client := ttshttp.NewHTTPTTSClient(config.TTSConfig{
			Type:     "http",
			Endpoint: server.URL,
			Model:    "kokoro",
		})

		catalog, ok := client.(media.VoiceCatalog)
		if !ok {
			t.Fatalf("client does not implement media.VoiceCatalog")
		}

		voices, err := catalog.ListVoices(context.Background())
		if err != nil {
			t.Fatalf("ListVoices failed: %v", err)
		}

		if len(voices) != 3 {
			t.Fatalf("got %d voices, want 3", len(voices))
		}

		// Check af_heart
		heart := voices[0]
		if heart.ID != "af_heart" {
			t.Errorf("ID = %q, want af_heart", heart.ID)
		}
		if heart.Name != "Heart (American Female)" {
			t.Errorf("Name = %q, want Heart (American Female)", heart.Name)
		}
		if heart.Gender != "female" {
			t.Errorf("Gender = %q, want female", heart.Gender)
		}
		if heart.Language != "en-US" {
			t.Errorf("Language = %q, want en-US", heart.Language)
		}
		if heart.Accent != "American" {
			t.Errorf("Accent = %q, want American", heart.Accent)
		}

		// Check bm_george
		george := voices[1]
		if george.ID != "bm_george" {
			t.Errorf("ID = %q, want bm_george", george.ID)
		}
		if george.Name != "George (British Male)" {
			t.Errorf("Name = %q, want George (British Male)", george.Name)
		}
		if george.Gender != "male" {
			t.Errorf("Gender = %q, want male", george.Gender)
		}
		if george.Language != "en-GB" {
			t.Errorf("Language = %q, want en-GB", george.Language)
		}
		if george.Accent != "British" {
			t.Errorf("Accent = %q, want British", george.Accent)
		}

		// Check zf_xiaobei
		xiaobei := voices[2]
		if xiaobei.ID != "zf_xiaobei" {
			t.Errorf("ID = %q, want zf_xiaobei", xiaobei.ID)
		}
		if xiaobei.Name != "Xiaobei (Chinese Female)" {
			t.Errorf("Name = %q, want Xiaobei (Chinese Female)", xiaobei.Name)
		}
		if xiaobei.Language != "zh" {
			t.Errorf("Language = %q, want zh", xiaobei.Language)
		}
	})

	t.Run("bare array response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"id": "af_bella", "name": "af_bella"},
				{"id": "am_adam", "name": "am_adam"}
			]`))
		}))
		defer server.Close()

		client := ttshttp.NewHTTPTTSClient(config.TTSConfig{
			Type:     "http",
			Endpoint: server.URL,
		})

		catalog, ok := client.(media.VoiceCatalog)
		if !ok {
			t.Fatalf("client does not implement media.VoiceCatalog")
		}

		voices, err := catalog.ListVoices(context.Background())
		if err != nil {
			t.Fatalf("ListVoices failed: %v", err)
		}

		if len(voices) != 2 {
			t.Fatalf("got %d voices, want 2", len(voices))
		}
		if voices[0].ID != "af_bella" || voices[0].Name != "Bella (American Female)" {
			t.Errorf("unexpected voice 0: %+v", voices[0])
		}
	})

	t.Run("unsupported endpoint returns empty with no error", func(t *testing.T) {
		client := ttshttp.NewHTTPTTSClient(config.TTSConfig{
			Type:     "http",
			Endpoint: "http://localhost:7851/api/tts-generate",
		})

		catalog, ok := client.(media.VoiceCatalog)
		if !ok {
			t.Fatalf("client does not implement media.VoiceCatalog")
		}

		voices, err := catalog.ListVoices(context.Background())
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if len(voices) != 0 {
			t.Errorf("expected 0 voices, got %d", len(voices))
		}
	})
}
