# Fish Audio S2 TTS Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Fish Audio S2 Pro as a dedicated local TTS provider (`tts:fish-audio`) connecting to vLLM-Omni with vocal steering and zero-shot voice cloning.

**Architecture:** A new `pkg/provider/ttsfishaudio` package implements `media.TTSClient`, `media.VoiceCatalog`, `media.SpeechCueAdvertiser`, and `media.VoiceOptions`. The client communicates with vLLM-Omni over OpenAI-compatible `POST /v1/audio/speech` and `GET /v1/audio/voices`. Local file references for zero-shot cloning are encoded into base64 data URLs. Registered under `provider.KeyTTSFishAudio` and wired into `pkg/media/exports.go`, `pkg/provider/all`, `SettingsStudio.tsx`, and documentation.

**Tech Stack:** Go 1.27, React 19, TypeScript, vLLM-Omni HTTP API, standard library `net/http` and `testing`.

---

### Task 1: Provider Key & Key Resolution

**Files:**
- Modify: `pkg/provider/keys.go:12-43`
- Modify: `pkg/media/exports.go:21-43`
- Test: `pkg/media/key_test.go:10-55`

- [x] **Step 1: Write the failing test for KeyTTSFishAudio resolution**

Add test cases in `pkg/media/key_test.go`:

```go
func TestTTSKeyForFishAudio(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.TTSConfig
		want provider.Key
		ok   bool
	}{
		{
			name: "explicit fish-audio type",
			cfg:  config.TTSConfig{Type: "fish-audio", Endpoint: "http://localhost:8091"},
			want: provider.InstanceOrSelf(provider.KeyTTSFishAudio, provider.HostDiscriminator("http://localhost:8091")),
			ok:   true,
		},
		{
			name: "http type with fishaudio model",
			cfg:  config.TTSConfig{Type: "http", Endpoint: "http://localhost:8091", Model: "fishaudio/s2-pro"},
			want: provider.InstanceOrSelf(provider.KeyTTSFishAudio, provider.HostDiscriminator("http://localhost:8091")),
			ok:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := media.TTSKeyFor(tc.cfg)
			if ok != tc.ok {
				t.Fatalf("TTSKeyFor(%+v) ok = %v, want %v", tc.cfg, ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("TTSKeyFor(%+v) = %q, want %q", tc.cfg, got, tc.want)
			}
		})
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestTTSKeyForFishAudio ./pkg/media/...`
Expected: FAIL with compilation error: undefined `provider.KeyTTSFishAudio`.

- [x] **Step 3: Define KeyTTSFishAudio in pkg/provider/keys.go**

In `pkg/provider/keys.go`:
1. Add `KeyTTSFishAudio Key = "tts:fish-audio"` to the `const` block.
2. Add `KeyTTSFishAudio` to `AllKeys()`.

```go
const (
	KeyTTSGemini     Key = "tts:gemini"
	KeyTTSElevenLabs Key = "tts:elevenlabs"
	KeyTTSNativeOS   Key = "tts:native-os"
	KeyTTSSherpaONNX Key = "tts:sherpa-onnx"
	KeyTTSPiper      Key = "tts:piper"
	KeyTTSFishAudio  Key = "tts:fish-audio"
	KeyTTSHTTP       Key = "tts:http"
    // ...
)
```

- [x] **Step 4: Update TTSKeyFor in pkg/media/exports.go**

In `pkg/media/exports.go`, update `TTSKeyFor`:

```go
func TTSKeyFor(cfg config.TTSConfig) (provider.Key, bool) {
	switch cfg.Type {
	case "fish-audio":
		return provider.InstanceOrSelf(provider.KeyTTSFishAudio, provider.HostDiscriminator(cfg.Endpoint)), true
	case "gemini":
		return provider.KeyTTSGemini, true
	case "builtin":
		switch cfg.BuiltinName {
		case "gemini":
			return provider.KeyTTSGemini, true
		case "sherpa-onnx", "kokoro":
			return provider.KeyTTSSherpaONNX, true
		case "native-os":
			return provider.KeyTTSNativeOS, true
		case "elevenlabs":
			return provider.KeyTTSElevenLabs, true
		}
		return "", false
	case "cli":
		return provider.InstanceOrSelf(provider.KeyTTSPiper, provider.CommandDiscriminator(cfg.Command)), true
	case "http":
		lowerModel := strings.ToLower(cfg.Model)
		if strings.Contains(lowerModel, "fishaudio") || strings.Contains(lowerModel, "s2-pro") {
			return provider.InstanceOrSelf(provider.KeyTTSFishAudio, provider.HostDiscriminator(cfg.Endpoint)), true
		}
		return provider.InstanceOrSelf(provider.KeyTTSHTTP, provider.HostDiscriminator(cfg.Endpoint)), true
	}
	return "", false
}
```

- [x] **Step 5: Run tests to verify they pass**

Run: `go test -v -run TestTTSKeyFor ./pkg/media/...`
Expected: PASS for all `TestTTSKeyFor*`.

- [x] **Step 6: Commit**

```bash
git add pkg/provider/keys.go pkg/media/exports.go pkg/media/key_test.go
git commit -m "feat(provider): add KeyTTSFishAudio and key resolution"
```

---

### Task 2: FishAudioTTSClient Implementation & Unit Tests

**Files:**
- Create: `pkg/provider/ttsfishaudio/client.go`
- Create: `pkg/provider/ttsfishaudio/client_test.go`

- [x] **Step 1: Write unit tests in pkg/provider/ttsfishaudio/client_test.go**

Create `pkg/provider/ttsfishaudio/client_test.go`:

```go
package ttsfishaudio_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/provider/ttsfishaudio"
)

func TestFishAudioSynthesis_Standard(t *testing.T) {
	expectedAudio := []byte("RIFFmockwavdata")
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &receivedBody)

		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(expectedAudio)
	}))
	defer server.Close()

	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{
		Endpoint: server.URL,
		Model:    "fishaudio/s2-pro",
		APIKey:   "test-key",
	})

	audio, err := client.Synthesize(context.Background(), "[excited] Greetings traveler!", &entity.VoiceConfig{
		VoiceID:    "speaker-1",
		SpeechRate: 1.2,
	})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}

	if string(audio) != string(expectedAudio) {
		t.Errorf("expected audio %q, got %q", expectedAudio, audio)
	}

	if receivedBody["input"] != "[excited] Greetings traveler!" {
		t.Errorf("expected input %q, got %q", "[excited] Greetings traveler!", receivedBody["input"])
	}
	if receivedBody["voice"] != "speaker-1" {
		t.Errorf("expected voice %q, got %q", "speaker-1", receivedBody["voice"])
	}
	if receivedBody["speed"] != 1.2 {
		t.Errorf("expected speed 1.2, got %v", receivedBody["speed"])
	}
}

func TestFishAudioSynthesis_VoiceCloning_LocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	sampleWavPath := filepath.Join(tmpDir, "sample.wav")
	sampleBytes := []byte("RIFFfakeaudio")
	if err := os.WriteFile(sampleWavPath, sampleBytes, 0644); err != nil {
		t.Fatalf("failed to write sample wav: %v", err)
	}

	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &receivedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{
		Endpoint: server.URL,
		Model:    "fishaudio/s2-pro",
	})

	_, err := client.Synthesize(context.Background(), "Cloned dialogue", &entity.VoiceConfig{
		VoiceID: "default",
		Options: map[string]interface{}{
			"ref_audio": sampleWavPath,
			"ref_text":  "This is the reference transcript.",
		},
	})
	if err != nil {
		t.Fatalf("Synthesize with cloning failed: %v", err)
	}

	refAudio, ok := receivedBody["ref_audio"].(string)
	if !ok || !strings.HasPrefix(refAudio, "data:audio/wav;base64,") {
		t.Fatalf("expected ref_audio data URL, got %v", receivedBody["ref_audio"])
	}
	b64Part := strings.TrimPrefix(refAudio, "data:audio/wav;base64,")
	decoded, err := base64.StdEncoding.DecodeString(b64Part)
	if err != nil || string(decoded) != string(sampleBytes) {
		t.Errorf("decoded ref_audio mismatch: got %q, want %q", decoded, sampleBytes)
	}

	if receivedBody["ref_text"] != "This is the reference transcript." {
		t.Errorf("expected ref_text %q, got %q", "This is the reference transcript.", receivedBody["ref_text"])
	}
}

func TestFishAudioSpeechCueCapabilities(t *testing.T) {
	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{})
	caps := client.SpeechCueCapabilities()

	if !caps.AudioTags {
		t.Errorf("expected AudioTags to be true")
	}
	if len(caps.SupportedTags) == 0 {
		t.Errorf("expected non-empty SupportedTags")
	}
}

func TestFishAudioVoiceCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/audio/voices" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"voices": ["voice-alpha", "voice-beta"]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{Endpoint: server.URL})
	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices failed: %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(voices))
	}
	if voices[0].ID != "voice-alpha" {
		t.Errorf("expected first voice ID voice-alpha, got %s", voices[0].ID)
	}
}

func TestFishAudioVoiceCatalogFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := ttsfishaudio.NewFishAudioTTSClient(config.TTSConfig{Endpoint: server.URL})
	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices fallback failed: %v", err)
	}
	if len(voices) != 1 || voices[0].ID != "default" {
		t.Errorf("expected single default fallback voice, got %+v", voices)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/provider/ttsfishaudio/...`
Expected: FAIL with compilation error: package does not exist or types missing.

- [x] **Step 3: Implement pkg/provider/ttsfishaudio/client.go**

Create `pkg/provider/ttsfishaudio/client.go`:

```go
package ttsfishaudio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// FishAudioTTSClient interacts with Fish Audio S2 Pro served via vLLM-Omni.
type FishAudioTTSClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

// NewFishAudioTTSClient constructs a new FishAudioTTSClient.
func NewFishAudioTTSClient(cfg config.TTSConfig) *FishAudioTTSClient {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		endpoint = "http://localhost:8091"
	}
	model := cfg.Model
	if model == "" {
		model = "fishaudio/s2-pro"
	}

	return &FishAudioTTSClient{
		endpoint: endpoint,
		model:    model,
		apiKey:   cfg.APIKey,
		client:   &http.Client{Transport: telemetry.HTTPTransport(nil), Timeout: 120 * time.Second},
	}
}

// Synthesize sends a speech synthesis request to vLLM-Omni's /v1/audio/speech endpoint.
func (c *FishAudioTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	speechURL := c.endpoint
	if !strings.HasSuffix(speechURL, "/v1/audio/speech") {
		speechURL = c.endpoint + "/v1/audio/speech"
	}

	voiceID := "default"
	if voice != nil && voice.VoiceID != "" {
		voiceID = voice.VoiceID
	}

	payloadMap := map[string]interface{}{
		"model":           c.model,
		"input":           text,
		"voice":           voiceID,
		"response_format": "wav",
	}

	if voice != nil && voice.SpeechRate > 0 && voice.SpeechRate != 1.0 {
		payloadMap["speed"] = voice.SpeechRate
	}

	if voice != nil && len(voice.Options) > 0 {
		if refAudioRaw, ok := voice.Options["ref_audio"]; ok {
			if refAudioStr, isStr := refAudioRaw.(string); isStr && strings.TrimSpace(refAudioStr) != "" {
				encodedAudio, err := resolveReferenceAudio(refAudioStr)
				if err != nil {
					return nil, fmt.Errorf("fish-audio: resolve reference audio: %w", err)
				}
				payloadMap["ref_audio"] = encodedAudio
			}
		}
		if refTextRaw, ok := voice.Options["ref_text"]; ok {
			if refTextStr, isStr := refTextRaw.(string); isStr && strings.TrimSpace(refTextStr) != "" {
				payloadMap["ref_text"] = refTextStr
			}
		}
	}

	bodyBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return nil, fmt.Errorf("fish-audio: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", speechURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("fish-audio: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fish-audio: execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		detailBytes, _ := io.ReadAll(io.LimitReader(resp.Body, provider.MaxProviderDetailBytes))
		return nil, fmt.Errorf("fish-audio tts failed (%d): %s", resp.StatusCode, provider.TruncateDetail(detailBytes))
	}

	return io.ReadAll(resp.Body)
}

func resolveReferenceAudio(ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "data:") {
		return trimmed, nil
	}

	data, err := os.ReadFile(trimmed)
	if err != nil {
		return "", err
	}

	mimeType := "audio/wav"
	ext := strings.ToLower(filepath.Ext(trimmed))
	switch ext {
	case ".mp3":
		mimeType = "audio/mpeg"
	case ".flac":
		mimeType = "audio/flac"
	case ".ogg":
		mimeType = "audio/ogg"
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded), nil
}

// SpeechCueCapabilities advertises bracketed vocal cues support.
func (c *FishAudioTTSClient) SpeechCueCapabilities() media.SpeechCueCapabilities {
	return media.SpeechCueCapabilities{
		AudioTags:        true,
		MarkdownEmphasis: false,
		SupportedTags: []string{
			"whisper", "excited", "angry", "sad", "laugh",
			"sigh", "gasp", "cough", "cry", "screaming", "shouting",
		},
		PromptGuidance: "Use bracketed emotional cues like [whisper] or [excited] directly before dialogue lines to steer delivery and vocal expression.",
	}
}

// VoiceOptions declares provider tunables for reference audio and transcript cloning.
func (c *FishAudioTTSClient) VoiceOptions() []media.VoiceOption {
	return []media.VoiceOption{
		{
			Key:         "ref_audio",
			Label:       "Reference Audio (Voice Clone)",
			Kind:        "string",
			Description: "Local file path (e.g. assets/voices/hero.wav), URL, or base64 data URI for zero-shot cloning.",
		},
		{
			Key:         "ref_text",
			Label:       "Reference Audio Transcript",
			Kind:        "string",
			Description: "Exact transcript of the reference audio clip (required by S2 Pro for voice cloning).",
		},
	}
}

// ListVoices retrieves available voices from vLLM-Omni, with a fallback to the default speaker.
func (c *FishAudioTTSClient) ListVoices(ctx context.Context) ([]media.ProviderVoice, error) {
	voicesURL := c.endpoint
	if !strings.HasSuffix(voicesURL, "/v1/audio/voices") {
		voicesURL = c.endpoint + "/v1/audio/voices"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", voicesURL, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil || resp.StatusCode == http.StatusNotFound {
		return defaultFallbackVoices(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return defaultFallbackVoices(), nil
	}

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return defaultFallbackVoices(), nil
	}

	var envelope struct {
		Voices []string `json:"voices"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Voices) > 0 {
		out := make([]media.ProviderVoice, len(envelope.Voices))
		for i, v := range envelope.Voices {
			out[i] = media.ProviderVoice{
				ID:       v,
				Name:     v,
				Language: "Multi",
				Tags:     []string{"fish-audio", "s2-pro"},
			}
		}
		return out, nil
	}

	return defaultFallbackVoices(), nil
}

func defaultFallbackVoices() []media.ProviderVoice {
	return []media.ProviderVoice{
		{
			ID:       "default",
			Name:     "Default Speaker (Fish Audio S2)",
			Language: "Multi",
			Tags:     []string{"dual-ar", "44.1khz", "zero-shot-capable"},
			Defaults: map[string]interface{}{"pitch": 1.0, "speech_rate": 1.0},
		},
	}
}

var _ media.TTSClient = (*FishAudioTTSClient)(nil)
var _ media.VoiceCatalog = (*FishAudioTTSClient)(nil)
var _ media.VoiceOptions = (*FishAudioTTSClient)(nil)
var _ media.SpeechCueAdvertiser = (*FishAudioTTSClient)(nil)
```

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/provider/ttsfishaudio/...`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/provider/ttsfishaudio/
git commit -m "feat(ttsfishaudio): implement FishAudioTTSClient and tests"
```

---

### Task 3: Provider Registration & Descriptor Validation

**Files:**
- Create: `pkg/provider/ttsfishaudio/ttsfishaudio.go`
- Modify: `pkg/provider/all/all.go`
- Test: `pkg/provider/all/all_test.go`
- Test: `pkg/provider/all/preset_parity_test.go`

- [x] **Step 1: Write pkg/provider/ttsfishaudio/ttsfishaudio.go**

Create `pkg/provider/ttsfishaudio/ttsfishaudio.go`:

```go
package ttsfishaudio

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyTTSFishAudio),
			Family:      provider.FamilyTTS,
			Label:       "Fish Audio S2 (vLLM-Omni)",
			Description: "Dual-AR speech synthesis with fine-grained emotional tags and zero-shot voice cloning.",
			Source:      "http",
			Features: []provider.Feature{
				provider.FeatureVoiceCatalog,
				provider.FeatureVoiceOptions,
				provider.FeatureSpeechCues,
				provider.FeatureOffline,
			},
			Presets: []provider.Preset{
				{
					ID:          "fish-audio-s2-vllm",
					Order:       5,
					Label:       "Fish Audio S2 Pro (Local vLLM-Omni)",
					Description: "Local Fish Audio S2 Pro running via vLLM-Omni on port 8091.",
					Config: map[string]interface{}{
						"type":          "http",
						"endpoint":      "http://localhost:8091",
						"model":         "fishaudio/s2-pro",
						"default_voice": "default",
						"pitch":         1.0,
						"speech_rate":   1.0,
						"auto_play":     true,
						"master_volume": 1.0,
					},
				},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var payload media.TTSBuildPayload
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					return nil, err
				}
			}
			return NewFishAudioTTSClient(payload.Config), nil
		},
	})
}
```

- [x] **Step 2: Blank-import in pkg/provider/all/all.go**

Add `_ "github.com/darkliquid/localrpg/pkg/provider/ttsfishaudio"` to `pkg/provider/all/all.go`.

- [x] **Step 3: Run all provider tests to verify descriptor and preset parity**

Run: `go test -v ./pkg/provider/all/...`
Expected: PASS (`TestTTSDescriptorsBuildAndFeaturesAreBacked`, `TestPresetsBuildMatchingClients`, `TestPresetParity`).

- [x] **Step 4: Commit**

```bash
git add pkg/provider/ttsfishaudio/ttsfishaudio.go pkg/provider/all/all.go
git commit -m "feat(provider): register tts:fish-audio and preset"
```

---

### Task 4: Settings Studio & Frontend Integration

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx:1570-1700`

- [x] **Step 1: Add Fish Audio S2 to the TTS Engine dropdown in SettingsStudio.tsx**

In `frontend/src/components/SettingsStudio.tsx`:
1. In the TTS Engine `<select>` value calculation, add support for detecting `fish-audio`:
```tsx
config.media.tts.type === 'fish-audio' ||
(config.media.tts.type === 'http' && config.media.tts.model?.includes('fish'))
  ? 'fish-audio'
  : ...
```
2. In the `<select>` options, add:
```tsx
<option value="fish-audio">Fish Audio S2 (vLLM-Omni)</option>
```
3. In `onChange`, when `val === 'fish-audio'`:
```tsx
} else if (val === 'fish-audio') {
  setConfig({
    ...config,
    media: {
      ...config.media,
      tts: {
        ...config.media.tts,
        type: 'http',
        builtin_name: undefined,
        endpoint: config.media.tts.endpoint && config.media.tts.endpoint.includes('8091')
          ? config.media.tts.endpoint
          : 'http://localhost:8091',
        model: 'fishaudio/s2-pro',
        default_voice: config.media.tts.default_voice || 'default',
        options: undefined,
      },
    },
  });
}
```

- [x] **Step 2: Verify frontend type check and bundle build**

Run: `npm --prefix frontend run build`
Expected: PASS with no TypeScript or Vite bundle errors.

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add Fish Audio S2 engine selection in SettingsStudio"
```

---

### Task 5: Documentation & Embedded Catalogue Regeneration

**Files:**
- Modify: `pkg/gui/docs/05-providers.md`
- Run: `go test ./pkg/gui -update-docs`
- Verify: `pkg/gui/docs/12-provider-catalogue.md`

- [x] **Step 1: Document Fish Audio S2 in pkg/gui/docs/05-providers.md**

Add a dedicated section for Fish Audio S2 Pro under Voice Synthesis in `pkg/gui/docs/05-providers.md`:
- Model description: 4B Dual-AR, 44.1 kHz, emotional cues (`[whisper]`, `[excited]`, etc.).
- vLLM-Omni setup command and Docker run command.
- Zero-shot voice cloning parameters (`ref_audio`, `ref_text`).

- [x] **Step 2: Regenerate embedded catalogue documentation**

Run: `go test ./pkg/gui -update-docs`
Expected: PASS, updates `pkg/gui/docs/12-provider-catalogue.md` to reflect `tts:fish-audio` and its preset.

- [x] **Step 3: Run full backend and frontend test suites**

Run:
```bash
go test -v -count=1 ./...
npm --prefix frontend run build
```
Expected: All tests PASS.

- [x] **Step 4: Commit**

```bash
git add pkg/gui/docs/
git commit -m "docs(providers): add Fish Audio S2 documentation and update catalogue"
```
