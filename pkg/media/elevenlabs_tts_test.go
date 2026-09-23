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
