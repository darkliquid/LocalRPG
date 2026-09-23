package media

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
)

// elevenLabsServer records what the client sent and replies with fixed bytes.
type elevenLabsServer struct {
	lastPath   string
	lastQuery  string
	lastHeader string
	lastBody   map[string]interface{}
	status     int
	response   []byte
}

func (s *elevenLabsServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.lastPath = r.URL.Path
		s.lastQuery = r.URL.RawQuery
		s.lastHeader = r.Header.Get("xi-api-key")
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&s.lastBody)
		}
		if s.status != 0 {
			w.WriteHeader(s.status)
			return
		}
		_, _ = w.Write(s.response)
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestElevenLabsClient(t *testing.T, server *httptest.Server) *ElevenLabsTTSClient {
	t.Helper()
	client, err := NewElevenLabsTTSClient(config.TTSConfig{
		Type:        "builtin",
		BuiltinName: "elevenlabs",
		APIKey:      "test-key",
	})
	if err != nil {
		t.Fatalf("NewElevenLabsTTSClient: %v", err)
	}
	client.baseURL = server.URL
	client.client = server.Client()
	return client
}

func TestNewElevenLabsTTSClientRequiresAPIKey(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "")
	_, err := NewElevenLabsTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"})
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("err = %v, want ErrMissingAPIKey", err)
	}

	t.Setenv("ELEVENLABS_API_KEY", "from-env")
	client, err := NewElevenLabsTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"})
	if err != nil {
		t.Fatalf("expected the environment key to be accepted: %v", err)
	}
	if client.apiKey != "from-env" {
		t.Errorf("apiKey = %q, want the environment value", client.apiKey)
	}
}

func TestElevenLabsSynthesizeMapsTheRequest(t *testing.T) {
	server := &elevenLabsServer{response: []byte("ID3audio")}
	httpServer := server.start(t)
	client := newTestElevenLabsClient(t, httpServer)

	voice := &entity.VoiceConfig{
		VoiceID:    "EXAVITQu4vr4xnSDxMaL",
		SpeechRate: 1.2,
		Options: map[string]interface{}{
			"stability":         0.35,
			"similarity_boost":  0.8,
			"style":             0.2,
			"use_speaker_boost": false,
			"format":            "pcm_24000",
		},
	}
	audio, err := client.Synthesize(context.Background(), "The gate opens.", voice)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(audio) != "ID3audio" {
		t.Errorf("audio = %q", audio)
	}
	if server.lastPath != "/v1/text-to-speech/EXAVITQu4vr4xnSDxMaL" {
		t.Errorf("path = %q", server.lastPath)
	}
	if server.lastHeader != "test-key" {
		t.Errorf("xi-api-key = %q", server.lastHeader)
	}
	if server.lastQuery != "output_format=pcm_24000" {
		t.Errorf("query = %q", server.lastQuery)
	}
	if server.lastBody["model_id"] != "eleven_multilingual_v2" {
		t.Errorf("model_id = %v", server.lastBody["model_id"])
	}
	settings, ok := server.lastBody["voice_settings"].(map[string]interface{})
	if !ok {
		t.Fatalf("voice_settings missing: %v", server.lastBody)
	}
	if settings["stability"] != 0.35 || settings["similarity_boost"] != 0.8 || settings["style"] != 0.2 {
		t.Errorf("voice_settings = %v", settings)
	}
	if settings["use_speaker_boost"] != false {
		t.Errorf("use_speaker_boost = %v, want false", settings["use_speaker_boost"])
	}
	if settings["speed"] != 1.2 {
		t.Errorf("speed = %v, want the voice's speech rate", settings["speed"])
	}
}

func TestElevenLabsOmitsVoiceSettingsWithoutOptions(t *testing.T) {
	server := &elevenLabsServer{response: []byte("ID3audio")}
	httpServer := server.start(t)
	client := newTestElevenLabsClient(t, httpServer)

	voice := &entity.VoiceConfig{VoiceID: "EXAVITQu4vr4xnSDxMaL", SpeechRate: 1}
	if _, err := client.Synthesize(context.Background(), "Hello.", voice); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if _, ok := server.lastBody["voice_settings"]; ok {
		t.Errorf("voice_settings must be omitted when the profile declares nothing, got %v", server.lastBody["voice_settings"])
	}
}

func TestElevenLabsClampsOutOfRangeOptions(t *testing.T) {
	server := &elevenLabsServer{response: []byte("ID3audio")}
	httpServer := server.start(t)
	client := newTestElevenLabsClient(t, httpServer)

	voice := &entity.VoiceConfig{VoiceID: "v", Options: map[string]interface{}{"stability": 4.0}}
	if _, err := client.Synthesize(context.Background(), "Hello.", voice); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	settings, _ := server.lastBody["voice_settings"].(map[string]interface{})
	if settings["stability"] != 1.0 {
		t.Errorf("stability = %v, want the clamped 1.0", settings["stability"])
	}
}

func TestElevenLabsHidesPitch(t *testing.T) {
	server := &elevenLabsServer{response: []byte("ID3audio")}
	httpServer := server.start(t)
	client := newTestElevenLabsClient(t, httpServer)

	// Pitch is unsupported and must not become a request field; speech rate is.
	voice := &entity.VoiceConfig{VoiceID: "v", Pitch: 0.7, SpeechRate: 1}
	if _, err := client.Synthesize(context.Background(), "Hello.", voice); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if _, ok := server.lastBody["pitch"]; ok {
		t.Errorf("pitch must not be sent, got %v", server.lastBody["pitch"])
	}
}

func TestElevenLabsMapsErrors(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{http.StatusUnauthorized, "rejected the API key"},
		{http.StatusPaymentRequired, "quota or rate limit"},
		{http.StatusUnprocessableEntity, "not available on this account"},
	}
	for _, tc := range cases {
		server := &elevenLabsServer{status: tc.status}
		httpServer := server.start(t)
		client := newTestElevenLabsClient(t, httpServer)

		_, err := client.Synthesize(context.Background(), "Hello.", &entity.VoiceConfig{VoiceID: "v"})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: err = %v, want it to mention %q", tc.status, err, tc.want)
		}
	}
}

func TestElevenLabsVoiceOptionsSchema(t *testing.T) {
	client := &ElevenLabsTTSClient{}
	options := client.VoiceOptions()
	keys := make(map[string]bool, len(options))
	for _, option := range options {
		keys[option.Key] = true
	}
	for _, want := range []string{"stability", "similarity_boost", "style", "use_speaker_boost", "model", "format"} {
		if !keys[want] {
			t.Errorf("schema is missing %q", want)
		}
	}
	if keys["pitch"] {
		t.Errorf("pitch is unsupported and must not be declared")
	}
	if !client.Metered() {
		t.Errorf("ElevenLabs charges per request and must report metered")
	}
}

func TestElevenLabsListVoicesPagesAndMaps(t *testing.T) {
	page := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/voices" {
			t.Errorf("path = %q, want /v2/voices", r.URL.Path)
		}
		if got := r.URL.Query().Get("page_size"); got != "100" {
			t.Errorf("page_size = %q, want 100", got)
		}
		page++
		switch page {
		case 1:
			if r.URL.Query().Get("next_page_token") != "" {
				t.Errorf("first page should not carry a token")
			}
			_, _ = w.Write([]byte(`{
				"voices": [{
					"voice_id": "v1",
					"name": "Sarah",
					"category": "premade",
					"description": "A calm narrator.",
					"preview_url": "https://example.test/v1.mp3",
					"labels": {"gender": "Female", "accent": "American", "age": "middle-aged", "use_case": "narrative"},
					"settings": {"stability": 0.5, "similarity_boost": 0.75},
					"available_for_tiers": ["free"],
					"verified_languages": [{"language": "en"}]
				}],
				"has_more": true,
				"total_count": 2,
				"next_page_token": "page-2"
			}`))
		default:
			if r.URL.Query().Get("next_page_token") != "page-2" {
				t.Errorf("second page token = %q, want page-2", r.URL.Query().Get("next_page_token"))
			}
			_, _ = w.Write([]byte(`{"voices": [{"voice_id": "v2", "name": "Brian", "labels": {}}], "has_more": false, "total_count": 2}`))
		}
	}))
	t.Cleanup(server.Close)

	client := newTestElevenLabsClient(t, server)

	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("voices = %d, want 2 after paging", len(voices))
	}

	first := voices[0]
	if first.ID != "v1" || first.Name != "Sarah" {
		t.Errorf("first voice = %+v", first)
	}
	if first.Gender != "female" {
		t.Errorf("gender = %q, want lowercased", first.Gender)
	}
	if first.Accent != "American" {
		t.Errorf("accent = %q", first.Accent)
	}
	if first.Language != "en" {
		t.Errorf("language = %q", first.Language)
	}
	if first.PreviewURL != "https://example.test/v1.mp3" {
		t.Errorf("preview = %q", first.PreviewURL)
	}
	if first.Defaults["stability"] != 0.5 {
		t.Errorf("defaults = %v", first.Defaults)
	}
	if !containsString(first.Tags, "middle-aged") || !containsString(first.Tags, "narrative") {
		t.Errorf("tags = %v, want the normalised labels", first.Tags)
	}
	if !containsString(first.Categories, "premade") {
		t.Errorf("categories = %v", first.Categories)
	}
}
