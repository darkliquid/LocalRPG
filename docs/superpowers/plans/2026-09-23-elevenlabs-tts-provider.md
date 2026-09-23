# ElevenLabs TTS Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a built-in ElevenLabs TTS provider whose voices and tunables are described by the capability interfaces, so it needs no provider-specific frontend code.

**Architecture:** One file implements `TTSClient`, `VoiceCatalog`, `VoiceOptions`, and `MeteredProvider` against ElevenLabs' REST API using only the standard library. The existing pipeline, catalog cache, inspect endpoint, and settings UI already do the rest, because they were built provider-agnostic.

**Tech Stack:** Go 1.27.1 (standard library only, `net/http` and `net/http/httptest`), existing `pkg/media` provider factory, `pkg/config` presets, `pkg/trace`, React/TypeScript presets and Settings.

**Spec:** `docs/superpowers/specs/2026-09-22-elevenlabs-tts-provider-design.md`
**Depends on:** `docs/superpowers/plans/2026-09-23-tts-provider-capabilities.md` and `...-frontend.md`, both implemented.

## Scope

The capability platform is in place: `VoiceCatalog`, `VoiceOptions`, `MeteredProvider`, `ProviderKey`, `ValidateVoiceOptions`, `CachedVoiceCatalog`, `NormaliseVoiceTags`, `ComputeAudioCacheKeyForVoice`, `POST /api/tts/inspect`, `useTTSInspect`, `VoiceOptionsControl`, and `VoiceCatalogPicker` all exist. This plan is the provider itself, its preset, credential handling, and secret redaction.

Acceptance criterion 7 ("bulk synthesis warns first") stays partial for the same reason recorded in the frontend plan: no GUI bulk-synthesis control exists to warn in front of. The provider reports `Metered() == true`, which is what the badge and the count endpoint consume.

## Global Constraints

- Standard library only for the client and its tests; no new dependency.
- Tests use `testing`, `t.TempDir()`, and `net/http/httptest`. No testify.
- Use `interface{}`, never `any`. Errors wrapped with `fmt.Errorf("...: %w", err)`. `go vet ./...` clean.
- The API key must never appear in a trace, an error, or an API response.
- Never write em dashes in source code.
- Conventional Commits with a scope, subject under 72 characters.
- Verification: `mise run test` and `mise run lint`. Frontend gate: `cd frontend && npx tsc --noEmit`.
- Single Go test example: `go test -run TestElevenLabsSynthesize ./pkg/media/`.

### File Map

| Action | Path | Responsibility |
| :--- | :--- | :--- |
| Create | `pkg/media/elevenlabs_tts.go` | Client: constructor, `Synthesize`, `ListVoices`, `VoiceOptions`, `Metered`, `SetLogger`, `ErrMissingAPIKey` |
| Create | `pkg/media/elevenlabs_tts_test.go` | Request mapping, option schema, catalog paging and mapping, error mapping, redaction |
| Modify | `pkg/media/providers.go` | `case "elevenlabs"` in `NewTTSClient` |
| Modify | `pkg/media/catalog.go` | `KeyPresent(cfg) bool` |
| Modify | `pkg/media/catalog_test.go` | `KeyPresent` tests |
| Modify | `pkg/media/tts.go` | `media.tts.request` carries provider and model |
| Modify | `pkg/media/tts_test.go` | Provider/model trace test |
| Modify | `pkg/trace/sanitize.go` | `RegisterSecret` and value redaction |
| Create | `pkg/trace/sanitize_secret_test.go` | Registered-secret redaction test |
| Modify | `pkg/config/presets.go` | `TTSPresets["elevenlabs"]` |
| Modify | `frontend/src/templates/providerPresets.ts` | `TTS_PRESETS['elevenlabs']` |
| Modify | `pkg/gui/types.go` | `TTSInspectResponseDTO.KeyPresent` |
| Modify | `pkg/gui/tts_inspect.go` | Set `KeyPresent` from `media.KeyPresent` |
| Modify | `pkg/gui/tts_inspect_test.go` | Key-present assertion |
| Modify | `frontend/src/types.ts` | `TTSInspectResponse.key_present` |
| Modify | `frontend/src/components/SettingsStudio.tsx` | Key-present indicator and env-var hint |

---

## Task 1: The ElevenLabs client, request mapping, and option schema

**Files:**
- Create: `pkg/media/elevenlabs_tts.go`
- Create: `pkg/media/elevenlabs_tts_test.go`

**Interfaces:**
- Consumes: `config.TTSConfig`, `entity.VoiceConfig`, `media.VoiceOption`, `media.ValidateVoiceOptions`, `trace.Logger`.
- Produces: `media.ElevenLabsTTSClient`, `media.NewElevenLabsTTSClient(cfg config.TTSConfig) (*ElevenLabsTTSClient, error)`, `media.ErrMissingAPIKey`, and the four capability methods.

- [ ] **Step 1: Write the failing test**

Create `pkg/media/elevenlabs_tts_test.go`:

```go
package media

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
)

// elevenLabsServer records what the client sent and replies with fixed bytes.
type elevenLabsServer struct {
	lastPath  string
	lastQuery string
	lastHeader string
	lastBody  map[string]interface{}
	status    int
	response  []byte
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
			"stability":        0.35,
			"similarity_boost": 0.8,
			"style":            0.2,
			"use_speaker_boost": false,
			"format":           "pcm_24000",
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
```

Add `"strings"` to the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestNewElevenLabs|TestElevenLabs' ./pkg/media/ -v`
Expected: FAIL with "undefined: NewElevenLabsTTSClient".

- [ ] **Step 3: Write the client**

Create `pkg/media/elevenlabs_tts.go`:

```go
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/trace"
)

// ErrMissingAPIKey reports that an ElevenLabs client was built without a key.
var ErrMissingAPIKey = errors.New("elevenlabs: an API key is required; set media.tts.api_key or ELEVENLABS_API_KEY")

const (
	elevenLabsDefaultBaseURL   = "https://api.elevenlabs.io"
	elevenLabsDefaultModel     = "eleven_multilingual_v2"
	elevenLabsDefaultFormat    = "mp3_44100_128"
	elevenLabsDefaultVoiceID   = "EXAVITQu4vr4xnSDxMaL"
	elevenLabsCatalogPageSize  = 100
	elevenLabsRequestTimeout   = 60 * time.Second
)

// ElevenLabsTTSClient is the built-in provider for ElevenLabs speech. It
// implements TTSClient, VoiceCatalog, VoiceOptions, and MeteredProvider, so the
// pipeline, the catalog cache, and the settings UI need no provider-specific
// handling.
type ElevenLabsTTSClient struct {
	apiKey    string
	model     string
	baseURL   string
	outputFmt string
	client    *http.Client
	logger    trace.Logger
}

// NewElevenLabsTTSClient builds a client, preferring the configured key and
// falling back to ELEVENLABS_API_KEY so a user need not write a key to disk. It
// registers the key for redaction and never records it.
func NewElevenLabsTTSClient(cfg config.TTSConfig) (*ElevenLabsTTSClient, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY"))
	}
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = elevenLabsDefaultModel
	}

	trace.RegisterSecret(apiKey)

	return &ElevenLabsTTSClient{
		apiKey:    apiKey,
		model:     model,
		baseURL:   elevenLabsDefaultBaseURL,
		outputFmt: elevenLabsDefaultFormat,
		client:    &http.Client{Timeout: elevenLabsRequestTimeout},
	}, nil
}

// SetLogger attaches a trace sink. A nil logger records nothing.
func (c *ElevenLabsTTSClient) SetLogger(logger trace.Logger) {
	c.logger = trace.OrNil(logger)
}

// Metered reports that ElevenLabs charges per request.
func (c *ElevenLabsTTSClient) Metered() bool { return true }

// VoiceOptions declares the tunables ElevenLabs accepts. Pitch is deliberately
// absent: the API exposes speed but no pitch, and speech rate is the portable
// baseline, so declaring it twice would put two controls on one knob.
func (c *ElevenLabsTTSClient) VoiceOptions() []VoiceOption {
	return []VoiceOption{
		{Key: "stability", Label: "Stability", Kind: "float", Min: 0, Max: 1, Step: 0.05, Default: 0.5,
			Help: "Lower is more expressive, higher is more consistent."},
		{Key: "similarity_boost", Label: "Similarity", Kind: "float", Min: 0, Max: 1, Step: 0.05, Default: 0.75,
			Help: "How closely to follow the original voice."},
		{Key: "style", Label: "Style", Kind: "float", Min: 0, Max: 1, Step: 0.05, Default: 0,
			Help: "Amplifies the voice's character; adds latency when above zero."},
		{Key: "use_speaker_boost", Label: "Speaker boost", Kind: "bool", Default: true,
			Help: "Improves similarity; adds latency."},
		{Key: "model", Label: "Model", Kind: "enum",
			Options: []string{"eleven_multilingual_v2", "eleven_turbo_v2_5", "eleven_flash_v2_5"},
			Default: "eleven_multilingual_v2",
			Help:    "Turbo and Flash are faster and cheaper; Multilingual has the widest language support."},
		{Key: "format", Label: "Audio format", Kind: "enum",
			Options: []string{"mp3_44100_128", "mp3_22050_32", "pcm_24000", "ulaw_8000"},
			Default: "mp3_44100_128",
			Help:    "Higher bitrates and PCM/WAV may require a paid tier."},
	}
}

// Synthesize renders one utterance. It returns the provider's bytes unmodified;
// the pipeline names and caches the clip from its content.
func (c *ElevenLabsTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	voiceID := elevenLabsDefaultVoiceID
	if voice != nil && strings.TrimSpace(voice.VoiceID) != "" {
		voiceID = voice.VoiceID
	}

	// Defence in depth for a hand-edited note: clamp and drop against the schema
	// before anything reaches the wire.
	options, _ := ValidateVoiceOptions(c.VoiceOptions(), voiceOptions(voice))

	model := c.model
	if value, ok := options["model"].(string); ok && value != "" {
		model = value
	}
	outputFormat := c.outputFmt
	if value, ok := options["format"].(string); ok && value != "" {
		outputFormat = value
	}

	body := map[string]interface{}{
		"text":     text,
		"model_id": model,
	}
	if settings := voiceSettings(voice, options); len(settings) > 0 {
		body["voice_settings"] = settings
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1/text-to-speech/%s?%s",
		strings.TrimRight(c.baseURL, "/"), url.PathEscape(voiceID), url.Values{"output_format": {outputFormat}}.Encode())

	resp, err := c.do(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := elevenLabsError(resp); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

// voiceOptions is a voice's provider options, or nil when it carries none.
func voiceOptions(voice *entity.VoiceConfig) map[string]interface{} {
	if voice == nil {
		return nil
	}
	return voice.Options
}

// voiceSettings maps the declared options onto ElevenLabs' voice_settings. It
// returns an empty map when nothing was tuned, so the voice's stored server-side
// settings apply unchanged.
func voiceSettings(voice *entity.VoiceConfig, options map[string]interface{}) map[string]interface{} {
	settings := make(map[string]interface{})
	for _, key := range []string{"stability", "similarity_boost", "style", "use_speaker_boost"} {
		if value, ok := options[key]; ok {
			settings[key] = value
		}
	}

	// Speed is only sent when it was deliberately set away from the provider's
	// default, because a stock voice must keep its own server-side settings.
	if voice != nil && voice.SpeechRate > 0 && voice.SpeechRate != 1.0 {
		settings["speed"] = voice.SpeechRate
	}
	return settings
}

// do sends a request, retrying once on 429 after honouring Retry-After.
func (c *ElevenLabsTTSClient) do(ctx context.Context, method, endpoint string, payload []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("elevenlabs: build request: %w", err)
		}
		req.Header.Set("xi-api-key", c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("elevenlabs: request to %s failed: %w", requestHost(endpoint), err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			delay := retryAfter(resp)
			resp.Body.Close()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		return resp, nil
	}
}

// retryAfter is the server's Retry-After in seconds, or one second.
func retryAfter(resp *http.Response) time.Duration {
	value := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if seconds, err := time.ParseDuration(value + "s"); err == nil && seconds > 0 {
		return seconds
	}
	return time.Second
}

// requestHost names an endpoint without its path or query, for error messages.
func requestHost(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "the provider"
	}
	return parsed.Host
}

// elevenLabsError maps a failed response onto an actionable message. The body is
// never included wholesale, because it can echo request context.
func elevenLabsError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return errors.New("elevenlabs rejected the API key; check media.tts.api_key")
	case http.StatusPaymentRequired, http.StatusTooManyRequests:
		return errors.New("elevenlabs quota or rate limit reached; check your plan and credits")
	case http.StatusUnprocessableEntity:
		return errors.New("elevenlabs: that voice or model is not available on this account")
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("elevenlabs: the provider returned %d; try again later", resp.StatusCode)
		}
		return fmt.Errorf("elevenlabs: request failed with status %d", resp.StatusCode)
	}
}
```

Task 2 adds `ListVoices` to this file.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestNewElevenLabs|TestElevenLabs' ./pkg/media/ -v`
Expected: PASS. Vet the package: `go vet ./pkg/media/`.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/elevenlabs_tts.go pkg/media/elevenlabs_tts_test.go
git commit -m "feat(media): add a built-in ElevenLabs speech provider"
```

---

## Task 2: The ElevenLabs voice catalog

**Files:**
- Modify: `pkg/media/elevenlabs_tts.go`
- Modify: `pkg/media/elevenlabs_tts_test.go`

**Interfaces:**
- Consumes: `ProviderVoice`, `NormaliseVoiceTags`, the client's `do`.
- Produces: `(*ElevenLabsTTSClient).ListVoices(ctx) ([]ProviderVoice, error)`, which pages `/v2/voices` to completion.

- [ ] **Step 1: Write the failing test**

Append to `pkg/media/elevenlabs_tts_test.go`:

```go
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
	if client.baseURL != server.URL {
		t.Fatalf("test client did not point at the stub")
	}

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestElevenLabsListVoices ./pkg/media/ -v`
Expected: FAIL with "client.ListVoices undefined".

- [ ] **Step 3: Implement `ListVoices`**

Append to `pkg/media/elevenlabs_tts.go`:

```go
// elevenLabsVoice is one entry of the /v2/voices response. Labels are free-form
// strings, never enums, so they are carried as a map.
type elevenLabsVoice struct {
	VoiceID           string                 `json:"voice_id"`
	Name              string                 `json:"name"`
	Category          string                 `json:"category"`
	Description       string                 `json:"description"`
	PreviewURL        string                 `json:"preview_url"`
	Labels            map[string]string      `json:"labels"`
	Settings          map[string]interface{} `json:"settings"`
	AvailableForTiers []string               `json:"available_for_tiers"`
	VerifiedLanguages []struct {
		Language string `json:"language"`
	} `json:"verified_languages"`
}

// elevenLabsVoicePage is one page of the catalog. Pagination is mandatory: the
// v2 endpoint pages and the legacy one stops working past 500 voices.
type elevenLabsVoicePage struct {
	Voices        []elevenLabsVoice `json:"voices"`
	HasMore       bool              `json:"has_more"`
	TotalCount    int               `json:"total_count"`
	NextPageToken string            `json:"next_page_token"`
}

// ListVoices pages the account's voices to completion and maps them into the
// shared shape. Cloned and professional voices appear because they belong to the
// key in use, which is the point of fetching rather than shipping a list.
func (c *ElevenLabsTTSClient) ListVoices(ctx context.Context) ([]ProviderVoice, error) {
	voices := make([]ProviderVoice, 0)
	token := ""
	for {
		query := url.Values{"page_size": {fmt.Sprintf("%d", elevenLabsCatalogPageSize)}}
		if token != "" {
			query.Set("next_page_token", token)
		}
		endpoint := fmt.Sprintf("%s/v2/voices?%s", strings.TrimRight(c.baseURL, "/"), query.Encode())

		resp, err := c.do(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if err := elevenLabsError(resp); err != nil {
			resp.Body.Close()
			return nil, err
		}
		var page elevenLabsVoicePage
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("elevenlabs: decode voice catalog: %w", err)
		}

		for _, voice := range page.Voices {
			voices = append(voices, mapElevenLabsVoice(voice))
		}
		if !page.HasMore || page.NextPageToken == "" {
			return voices, nil
		}
		token = page.NextPageToken
	}
}

// mapElevenLabsVoice maps a catalog entry onto the shared voice shape.
func mapElevenLabsVoice(voice elevenLabsVoice) ProviderVoice {
	mapped := ProviderVoice{
		ID:          voice.VoiceID,
		Name:        voice.Name,
		Gender:      strings.ToLower(strings.TrimSpace(voice.Labels["gender"])),
		Accent:      voice.Labels["accent"],
		Description: voice.Description,
		PreviewURL:  voice.PreviewURL,
		Defaults:    voice.Settings,
	}
	if mapped.Description == "" {
		mapped.Description = voice.Labels["description"]
	}
	if len(voice.VerifiedLanguages) > 0 {
		mapped.Language = voice.VerifiedLanguages[0].Language
	}
	if voice.Category != "" {
		mapped.Categories = []string{voice.Category}
	}
	mapped.Tags = NormaliseVoiceTags(
		voice.Labels["age"],
		voice.Labels["use_case"],
		voice.Labels["gender"],
		voice.Labels["accent"],
		voice.Category,
	)
	mapped.Metadata = map[string]interface{}{
		"category":            voice.Category,
		"available_for_tiers": voice.AvailableForTiers,
		"verified_languages":  voice.VerifiedLanguages,
	}
	return mapped
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestElevenLabsListVoices ./pkg/media/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/elevenlabs_tts.go pkg/media/elevenlabs_tts_test.go
git commit -m "feat(media): list and map the ElevenLabs voice catalog"
```

---

## Task 3: Wire the provider into the factory

**Files:**
- Modify: `pkg/media/providers.go` (`NewTTSClient`)
- Modify: `pkg/media/catalog.go`
- Modify: `pkg/media/catalog_test.go`

**Interfaces:**
- Consumes: `NewElevenLabsTTSClient` (Task 1).
- Produces: `NewTTSClient` returns the ElevenLabs client for `builtin_name: elevenlabs`; `media.KeyPresent(cfg config.TTSConfig) bool`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/media/catalog_test.go`:

```go
func TestKeyPresent(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "")

	if KeyPresent(config.TTSConfig{}) {
		t.Errorf("an unconfigured provider has no key")
	}
	if !KeyPresent(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs", APIKey: "abc"}) {
		t.Errorf("a configured key must report present")
	}

	t.Setenv("ELEVENLABS_API_KEY", "from-env")
	if !KeyPresent(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"}) {
		t.Errorf("the environment key must count as present")
	}
	if KeyPresent(config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880"}) {
		t.Errorf("an unrelated provider must not read the ElevenLabs environment key")
	}
}
```

Append to `pkg/media/providers_test.go` if it exists, otherwise create `pkg/media/elevenlabs_factory_test.go`:

```go
package media

import (
	"errors"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

func TestNewTTSClientBuildsElevenLabs(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "")
	_, err := NewTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs"})
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("err = %v, want ErrMissingAPIKey when no key is set", err)
	}

	client, err := NewTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs", APIKey: "abc"})
	if err != nil {
		t.Fatalf("NewTTSClient: %v", err)
	}
	if _, ok := client.(*ElevenLabsTTSClient); !ok {
		t.Errorf("client = %T, want *ElevenLabsTTSClient", client)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run 'TestKeyPresent|TestNewTTSClientBuildsElevenLabs' ./pkg/media/ -v`
Expected: FAIL with "undefined: KeyPresent" and "undefined: ErrMissingAPIKey" (in the factory path).

- [ ] **Step 3: Implement**

In `pkg/media/catalog.go`, add the `os` import and:

```go
// KeyPresent reports whether a configuration has a usable credential, either in
// the config or from the provider's documented environment variable. It exists so
// the inspect endpoint can answer without echoing the key.
func KeyPresent(cfg config.TTSConfig) bool {
	if strings.TrimSpace(cfg.APIKey) != "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(cfg.BuiltinName), "elevenlabs") {
		return strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY")) != ""
	}
	return false
}
```

In `pkg/media/providers.go`, add to `NewTTSClient`'s `case "builtin"` switch, before `default`:

```go
		case "elevenlabs":
			return NewElevenLabsTTSClient(cfg)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/media/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/providers.go pkg/media/catalog.go pkg/media/catalog_test.go pkg/media/elevenlabs_factory_test.go
git commit -m "feat(media): build the ElevenLabs provider from configuration"
```

(Adjust the test file name to whichever file the factory test landed in.)

---

## Task 4: Redact registered secrets from traces

**Files:**
- Modify: `pkg/trace/sanitize.go`
- Create: `pkg/trace/sanitize_secret_test.go`

**Interfaces:**
- Produces: `trace.RegisterSecret(value string)`, and `Sanitize` replaces any registered value wherever it appears in a recorded string.

- [ ] **Step 1: Write the failing test**

Create `pkg/trace/sanitize_secret_test.go`:

```go
package trace

import (
	"strings"
	"testing"
)

func TestSanitizeRedactsRegisteredSecretValues(t *testing.T) {
	const secret = "sk-test-abcdef0123456789"
	RegisterSecret(secret)

	clean := Sanitize(map[string]interface{}{
		"headers": "xi-api-key: " + secret + "\ncontent-type: application/json",
		"note":    "no secret here",
	}, LevelFull, 20000)

	headers, _ := clean["headers"].(string)
	if strings.Contains(headers, secret) {
		t.Errorf("a registered secret leaked: %q", headers)
	}
	if !strings.Contains(headers, "[redacted]") {
		t.Errorf("headers = %q, want a redaction marker", headers)
	}
	if clean["note"] != "no secret here" {
		t.Errorf("unrelated text was altered: %v", clean["note"])
	}
}

func TestRegisterSecretIgnoresEmptyValues(t *testing.T) {
	RegisterSecret("")
	if got := redactSecrets("nothing to hide"); got != "nothing to hide" {
		t.Errorf("redactSecrets altered text with no registered secret: %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestSanitizeRedacts|TestRegisterSecret' ./pkg/trace/ -v`
Expected: FAIL with "undefined: RegisterSecret".

- [ ] **Step 3: Implement**

In `pkg/trace/sanitize.go`, add `"sync"` to the imports and:

```go
// registeredSecrets are values, not field names, that must never be recorded.
// A provider registers its key at construction, so a payload that embeds it in
// an unexpected field is still redacted.
var (
	secretMu         sync.RWMutex
	registeredSecrets []string
)

// RegisterSecret adds a value to the redaction set. Empty values are ignored,
// because replacing every occurrence of "" would destroy the record.
func RegisterSecret(value string) {
	if value == "" {
		return
	}
	secretMu.Lock()
	defer secretMu.Unlock()
	for _, existing := range registeredSecrets {
		if existing == value {
			return
		}
	}
	registeredSecrets = append(registeredSecrets, value)
}

// redactSecrets replaces every registered value in text.
func redactSecrets(text string) string {
	secretMu.RLock()
	defer secretMu.RUnlock()
	for _, secret := range registeredSecrets {
		if secret != "" && strings.Contains(text, secret) {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	return text
}
```

Change `sanitizeValue`'s string branch to redact before truncating:

```go
	case string:
		return truncate(redactSecrets(typed), payloadChars)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/trace/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/trace/sanitize.go pkg/trace/sanitize_secret_test.go
git commit -m "feat(trace): redact a registered secret wherever it appears"
```

---

## Task 5: Name the provider and model in the TTS trace

**Files:**
- Modify: `pkg/media/tts.go` (`SynthesizeUtterance`)
- Modify: `pkg/media/tts_test.go`

**Interfaces:**
- Consumes: the existing `trace.Memory` sink.
- Produces: `media.tts.request` carries `provider` and `model` fields.

- [ ] **Step 1: Write the failing test**

Append to `pkg/media/tts_test.go`:

```go
func TestTTSPipelineNamesTheProviderInTheTrace(t *testing.T) {
	pipeline := NewTTSPipeline(&mockTTSClient{}, NewContentCache(t.TempDir()))
	memory := trace.NewMemory(trace.LevelFull)
	pipeline.SetLogger(memory)

	voice := &entity.VoiceConfig{
		Provider: "builtin:elevenlabs",
		VoiceID:  "v1",
		Options:  map[string]interface{}{"model": "eleven_turbo_v2_5"},
	}
	if _, err := pipeline.SynthesizeUtterance(context.Background(), "elena", voice, "Hello."); err != nil {
		t.Fatalf("SynthesizeUtterance: %v", err)
	}

	event, ok := memory.Find("media.tts.request")
	if !ok {
		t.Fatalf("no media.tts.request event recorded")
	}
	if event.Fields["provider"] != "builtin:elevenlabs" {
		t.Errorf("provider = %v", event.Fields["provider"])
	}
	if event.Fields["model"] != "eleven_turbo_v2_5" {
		t.Errorf("model = %v", event.Fields["model"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestTTSPipelineNamesTheProvider ./pkg/media/ -v`
Expected: FAIL because `provider` is absent from the event.

- [ ] **Step 3: Implement**

In `pkg/media/tts.go`, inside `SynthesizeUtterance`, extend the existing trace payload. Before the `p.logger.Event("media.tts.request", ...)` call, compute:

```go
	provider, model := "", ""
	if voice != nil {
		provider = voice.Provider
		if value, ok := voice.Options["model"].(string); ok {
			model = value
		}
	}
```

Then add to the event map:

```go
		"provider":  provider,
		"model":     model,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/media/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/media/tts.go pkg/media/tts_test.go
git commit -m "feat(media): name the provider and model in the TTS trace"
```

---

## Task 6: Presets

**Files:**
- Modify: `pkg/config/presets.go`
- Modify: `frontend/src/templates/providerPresets.ts`

**Interfaces:**
- Produces: `config.TTSPresets["elevenlabs"]` and `TTS_PRESETS['elevenlabs']`, carrying no API key and no voice profiles.

- [ ] **Step 1: Add the Go preset**

In `pkg/config/presets.go`, inside `TTSPresets`, add:

```go
	"elevenlabs": {
		Type:         "builtin",
		BuiltinName:  "elevenlabs",
		Model:        "eleven_multilingual_v2",
		DefaultVoice: "EXAVITQu4vr4xnSDxMaL", // "Sarah", a premade stock voice
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
```

Deliberately no `APIKey` and no `VoiceProfiles`: selecting the preset is an explicit act, and the catalog supplies the voices.

- [ ] **Step 2: Add the frontend preset**

In `frontend/src/templates/providerPresets.ts`, inside `TTS_PRESETS`, add:

```ts
  elevenlabs: {
    label: 'ElevenLabs (Cloud, metered)',
    description: 'Cloud voices fetched from your account. Set a key here or via ELEVENLABS_API_KEY; charges per request.',
    config: {
      type: 'builtin',
      builtin_name: 'elevenlabs',
      model: 'eleven_multilingual_v2',
      default_voice: 'EXAVITQu4vr4xnSDxMaL',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
```

- [ ] **Step 3: Verify**

Run: `go test ./pkg/config/ -count=1` and `cd frontend && npx tsc --noEmit`
Expected: PASS and no TypeScript errors.

- [ ] **Step 4: Commit**

```bash
git add pkg/config/presets.go frontend/src/templates/providerPresets.ts
git commit -m "feat: add an opt-in ElevenLabs preset"
```

---

## Task 7: Report key presence without echoing the key

**Files:**
- Modify: `pkg/gui/types.go` (`TTSInspectResponseDTO`)
- Modify: `pkg/gui/tts_inspect.go`
- Modify: `pkg/gui/tts_inspect_test.go`
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx`

**Interfaces:**
- Consumes: `media.KeyPresent`.
- Produces: `TTSInspectResponseDTO.KeyPresent` / `TTSInspectResponse.key_present`, and a Settings indicator.

- [ ] **Step 1: Write the failing test**

Append to `pkg/gui/tts_inspect_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestInspectTTSReportsKeyPresence ./pkg/gui/ -v`
Expected: FAIL with "unknown field KeyPresent".

- [ ] **Step 3: Implement the backend field**

In `pkg/gui/types.go`, add both fields to `TTSInspectResponseDTO`:

```go
	// KeyPresent reports whether a credential is configured. The key itself is
	// never included.
	KeyPresent bool `json:"key_present"`
	// KeyRequired reports whether this provider needs a credential at all, so the
	// editor can explain a missing key instead of showing one to every provider.
	KeyRequired bool `json:"key_required"`
```

In `pkg/gui/tts_inspect.go`, add `"strings"` to the imports and set both where the provider key is set:

```go
	response := &TTSInspectResponseDTO{
		ProviderKey: media.ProviderKey(cfg),
		KeyPresent:  media.KeyPresent(cfg),
		KeyRequired: strings.EqualFold(strings.TrimSpace(cfg.BuiltinName), "elevenlabs"),
		Catalog:     VoiceCatalogDTO{Voices: []media.ProviderVoice{}},
	}
```

- [ ] **Step 4: Add the frontend field and indicator**

In `frontend/src/types.ts`, add to `TTSInspectResponse`:

```ts
  key_present: boolean;
  key_required: boolean;
```

In `SettingsStudio.tsx`, inside the block the previous plan added for the metered badge and catalog strip, add:

```tsx
            {inspect?.key_required && !inspect.key_present && (
              <div className="text-[11px] font-mono text-stone-400">
                No API key configured. Enter one below, or set ELEVENLABS_API_KEY in the environment.
              </div>
            )}
```

- [ ] **Step 5: Run tests and typecheck**

Run: `go test ./pkg/gui/ -count=1` and `cd frontend && npx tsc --noEmit`
Expected: PASS and no TypeScript errors.

- [ ] **Step 6: Commit**

```bash
git add pkg/gui/types.go pkg/gui/tts_inspect.go pkg/gui/tts_inspect_test.go frontend/src/types.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(gui): report key presence without echoing the key"
```

---

## Task 8: Full gate

**Files:** none (verification only).

- [ ] **Step 1: Run the whole suite**

Run: `go test ./... -count=1`
Expected: every package ok.

- [ ] **Step 2: Vet and typecheck**

Run: `mise run lint` and `cd frontend && npx tsc --noEmit`
Expected: `go vet ./...` clean and no TypeScript errors.

- [ ] **Step 3: Confirm the acceptance criteria this plan owns**

- 1 and 5: the client synthesises and omits `voice_settings` when a profile declares nothing (Tasks 1-3).
- 2: Markdown reduction applies automatically, because the client is not `MarkdownAware` (Task 1; the pipeline's `SpeakableTextFor` path).
- 3: the catalog pages, maps, and is owned by `CachedVoiceCatalog` (Task 2 plus the backend plan's cache).
- 4: six options come from the schema, and `stability` already enters the cache key via `ComputeAudioCacheKeyForVoice` (Task 1).
- 6: a missing or rejected key is actionable at configuration time, and the key never appears in traces or responses (Tasks 1, 4, 7).
- 7: `Metered()` is true; the interactive warning stays deferred as recorded in the frontend plan.
- 8: the gate above.

---

## Self-Review

**Spec coverage:**

- 3.1 configuration and preset: Task 6 (plus the existing `TTSConfig` fields, no new ones).
- 3.2 the client and factory: Tasks 1 and 3.
- 3.3 request mapping: Task 1 (model precedence, speed, voice_settings omission, no pitch, no language_code).
- 3.4 option schema: Task 1.
- 3.5 catalog mapping with pagination: Task 2.
- 3.6 error mapping and rate limits: Task 1 (`elevenLabsError`, one `Retry-After` retry).
- 3.7 secrets: Task 1 (env fallback, `RegisterSecret`), Task 4 (value redaction), Task 7 (`key_present`/`key_required`, never the key).
- 3.8 diagnostics: Task 5.
- 5 File Map: every listed file is touched except `pkg/config/types.go` (no new field is needed, as the spec allows) and `SettingsStudio.tsx` (the indicator lands in Task 7).

**Placeholder scan:** no "TBD"/"implement later" text; every code step carries real code. The Task 3 factory test names a file that may or may not exist and says so explicitly; land it wherever the other provider tests live.

**Type consistency:** `ElevenLabsTTSClient`, `NewElevenLabsTTSClient`, `ErrMissingAPIKey` are defined in Task 1 and used in Task 3. `ListVoices` and the `elevenLabsVoice*` structs are Task 2. `KeyPresent` is Task 3 and used in Task 7. `RegisterSecret`/`redactSecrets` are Task 4. `TTSInspectResponseDTO.KeyPresent` and `KeyRequired` are Task 7. The option keys (`stability`, `similarity_boost`, `style`, `use_speaker_boost`, `model`, `format`) are identical in the schema, the request mapping, and the tests.
