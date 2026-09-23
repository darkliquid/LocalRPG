package gui

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
)

// inspectingClient implements every optional capability, so one fake covers the
// whole inspect response.
type inspectingClient struct{}

func (c *inspectingClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return []byte("audio"), nil
}

func (c *inspectingClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	return []media.ProviderVoice{{ID: "v1", Name: "Voice One", Tags: []string{"male"}}}, nil
}

func (c *inspectingClient) VoiceOptions() []media.VoiceOption {
	return []media.VoiceOption{{Key: "stability", Label: "Stability", Kind: "float", Min: 0, Max: 1}}
}

func (c *inspectingClient) Metered() bool { return true }

// bareClient only synthesises, so it has no options and no catalog.
type bareClient struct{}

func (c *bareClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return []byte("audio"), nil
}

func TestInspectTTSReportsCapabilities(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &inspectingClient{}, nil
	}

	res, err := svc.InspectTTS(context.Background(), TTSInspectRequestDTO{
		Config: config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880/v1/audio/speech", APIKey: "secret"},
	})
	if err != nil {
		t.Fatalf("InspectTTS: %v", err)
	}
	if res.ProviderKey != "http:localhost:8880" {
		t.Errorf("ProviderKey = %q", res.ProviderKey)
	}
	if !res.Metered {
		t.Errorf("expected the provider's Metered declaration to surface")
	}
	if len(res.Options) != 1 || res.Options[0].Key != "stability" {
		t.Errorf("options = %+v", res.Options)
	}
	if !res.Catalog.Available || len(res.Catalog.Voices) != 1 {
		t.Errorf("catalog = %+v", res.Catalog)
	}

	encoded, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "api_key") {
		t.Errorf("inspect response leaked a secret: %s", encoded)
	}
}

func TestInspectTTSWithoutCapabilities(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &bareClient{}, nil
	}

	res, err := svc.InspectTTS(context.Background(), TTSInspectRequestDTO{Config: config.TTSConfig{Type: "builtin", BuiltinName: "native-os"}})
	if err != nil {
		t.Fatalf("InspectTTS: %v", err)
	}
	if len(res.Options) != 0 {
		t.Errorf("expected no options, got %+v", res.Options)
	}
	if res.Catalog.Available {
		t.Errorf("expected no catalog")
	}
	if res.Metered {
		t.Errorf("a provider that is not metered must report false")
	}
}

func TestInspectTTSConfigOverridesMetered(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &bareClient{}, nil
	}

	on := true
	res, err := svc.InspectTTS(context.Background(), TTSInspectRequestDTO{Config: config.TTSConfig{Type: "cli", Command: "piper", Metered: &on}})
	if err != nil {
		t.Fatalf("InspectTTS: %v", err)
	}
	if !res.Metered {
		t.Errorf("a configured metered flag must override the provider")
	}
}

func TestInspectTTSRoute(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &inspectingClient{}, nil
	}
	server := NewServer(svc, AssetHandler())

	body := `{"config":{"type":"http","endpoint":"http://localhost:8880/v1/audio/speech"}}`
	req := httptest.NewRequest("POST", "/api/tts/inspect", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "provider_key") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestInspectTTSReportsKeyPresenceWithoutTheKey(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "")
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &bareClient{}, nil
	}

	without, err := svc.InspectTTS(context.Background(), TTSInspectRequestDTO{
		Config: config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"},
	})
	if err != nil {
		t.Fatalf("InspectTTS: %v", err)
	}
	if without.KeyPresent {
		t.Errorf("expected key_present false with no key configured")
	}
	if !without.KeyRequired {
		t.Errorf("expected key_required true for a keyed provider")
	}

	with, err := svc.InspectTTS(context.Background(), TTSInspectRequestDTO{
		Config: config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs", APIKey: "secret"},
	})
	if err != nil {
		t.Fatalf("InspectTTS: %v", err)
	}
	if !with.KeyPresent {
		t.Errorf("expected key_present true with a configured key")
	}

	encoded, err := json.Marshal(with)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), "secret") {
		t.Errorf("inspect response leaked the key: %s", encoded)
	}
}

func TestInspectTTSHTTPDoesNotRequireKey(t *testing.T) {
	svc := NewService(t.TempDir())
	svc.newTTSClient = func(config.TTSConfig) (media.TTSClient, error) {
		return &bareClient{}, nil
	}

	res, err := svc.InspectTTS(context.Background(), TTSInspectRequestDTO{
		Config: config.TTSConfig{
			Type:        "http",
			Endpoint:    "http://localhost:8880/v1/audio/speech",
			Model:       "kokoro",
			BuiltinName: "elevenlabs",
		},
	})
	if err != nil {
		t.Fatalf("InspectTTS: %v", err)
	}
	if res.KeyRequired {
		t.Errorf("expected KeyRequired false for http tts provider, got true")
	}
}

func TestInspectTTSKokoroHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/audio/voices" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"voices":[{"id":"af_bella","name":"af_bella"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := NewService(t.TempDir())
	res, err := svc.InspectTTS(context.Background(), TTSInspectRequestDTO{
		Config: config.TTSConfig{
			Type:     "http",
			Endpoint: server.URL,
			Model:    "kokoro",
		},
	})
	if err != nil {
		t.Fatalf("InspectTTS: %v", err)
	}
	if !res.Catalog.Available {
		t.Errorf("Catalog.Available = false, want true")
	}
	if len(res.Catalog.Voices) != 1 || res.Catalog.Voices[0].ID != "af_bella" {
		t.Errorf("unexpected voices: %+v", res.Catalog.Voices)
	}
}

