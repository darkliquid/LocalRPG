# Cartesia TTS and STT Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Cartesia as a remote, metered provider for text-to-speech (`sonic-3.6`) and speech-to-text (`ink-whisper`) with shared credentials, voice catalog pagination, voice tunables, and speech cues.

**Architecture:** Add two modular provider packages (`pkg/provider/ttscartesia` and `pkg/provider/sttcartesia`) that register canonical keys `tts:cartesia` and `stt:cartesia`. Wire credentials through `providers.cartesia.api_key`, `CARTESIA_API_KEY` env var, and subsystem overrides, and integrate them into the media registry, catalog capabilities, and Settings Studio UI.

**Tech Stack:** Go (1.27.1), React 19 + TypeScript, Cartesia HTTPS REST API (`Cartesia-Version: 2026-08-14`).

---

### File Map

| Action | File Path | Responsibility |
|---|---|---|
| **Modify** | `pkg/provider/keys.go` | Define `KeyTTSCartesia`, `KeySTTCartesia`, add to `AllKeys()` |
| **Modify** | `pkg/config/types.go` | Add `Cartesia CartesiaProviderConfig` to `ProvidersConfig` |
| **Modify** | `pkg/config/presets.go` | Add `cartesia` presets to `TTSPresets` and `STTPresets` |
| **Modify** | `pkg/config/types_test.go` | Test serialization and preset retrieval for Cartesia |
| **Modify** | `pkg/media/exports.go` | Resolve `tts:cartesia` and `stt:cartesia` in `TTSKeyFor` and `STTKeyFor`, wire build payloads |
| **Modify** | `pkg/media/catalog.go` | Check Cartesia key presence in `KeyPresentWithSharedKey` |
| **Modify** | `pkg/media/key_test.go` | Assert key resolution for `tts:cartesia` and `stt:cartesia` |
| **Create** | `pkg/provider/ttscartesia/ttscartesia.go` | Register `tts:cartesia` descriptor and preset |
| **Create** | `pkg/provider/ttscartesia/client.go` | `CartesiaTTSClient` implementation (`TTSClient`, `VoiceCatalog`, `VoiceOptions`, `SpeechCueCapabilities`, `MeteredProvider`) |
| **Create** | `pkg/provider/ttscartesia/client_test.go` | Unit tests for synthesis, catalog pagination, retry, usage, errors |
| **Create** | `pkg/provider/sttcartesia/sttcartesia.go` | Register `stt:cartesia` descriptor and preset |
| **Create** | `pkg/provider/sttcartesia/client.go` | `CartesiaSTTClient` implementation (`STTClient`, `MeteredProvider`) |
| **Create** | `pkg/provider/sttcartesia/client_test.go` | Unit tests for transcription multipart upload, retry, usage, errors |
| **Modify** | `pkg/provider/all/all.go` | Blank-import `ttscartesia` and `sttcartesia` |
| **Modify** | `pkg/provider/all/all_test.go` | Verify drift-guard capability derivation for Cartesia descriptors |
| **Modify** | `frontend/src/types.ts` | Add Cartesia provider credential type in `ProvidersConfig` |
| **Modify** | `frontend/src/components/SettingsStudio.tsx` | UI controls for Cartesia API key, TTS/STT presets, and tunables |

---

### Task 1: Canonical Keys and Configuration Schema

**Files:**
- Modify: `pkg/provider/keys.go:6-43`
- Modify: `pkg/config/types.go:239-265`
- Modify: `pkg/config/presets.go:160-230`
- Test: `pkg/config/types_test.go`

- [ ] **Step 1: Write the failing test for configuration and preset resolution**

Add a test in `pkg/config/types_test.go` that verifies `CartesiaProviderConfig` serializes and `GetTTSPreset("cartesia")` and `GetSTTPreset("cartesia")` return valid configurations:

```go
func TestCartesiaConfigAndPresets(t *testing.T) {
	ttsPreset, ok := GetTTSPreset("cartesia")
	if !ok {
		t.Fatal("expected cartesia TTS preset")
	}
	if ttsPreset.Type != "builtin" || ttsPreset.BuiltinName != "cartesia" {
		t.Errorf("unexpected TTS preset type/name: %+v", ttsPreset)
	}
	if ttsPreset.Model != "sonic-3.6" {
		t.Errorf("unexpected TTS preset model: %s", ttsPreset.Model)
	}

	sttPreset, ok := GetSTTPreset("cartesia")
	if !ok {
		t.Fatal("expected cartesia STT preset")
	}
	if sttPreset.Type != "builtin" || sttPreset.BuiltinName != "cartesia" {
		t.Errorf("unexpected STT preset type/name: %+v", sttPreset)
	}
	if sttPreset.Model != "ink-whisper" {
		t.Errorf("unexpected STT preset model: %s", sttPreset.Model)
	}

	var root Config
	root.Providers.Cartesia.APIKey = "sk_car_test"
	data, err := yaml.Marshal(root)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	var unmarshaled Config
	if err := yaml.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if unmarshaled.Providers.Cartesia.APIKey != "sk_car_test" {
		t.Errorf("got %q, want sk_car_test", unmarshaled.Providers.Cartesia.APIKey)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestCartesiaConfigAndPresets ./pkg/config/`  
Expected: Compilation failure or FAIL (undefined `Cartesia` on `ProvidersConfig`, `cartesia` preset missing).

- [ ] **Step 3: Implement canonical keys and config types**

1. In `pkg/provider/keys.go`, add:
```go
	KeyTTSCartesia Key = "tts:cartesia"
	KeySTTCartesia Key = "stt:cartesia"
```
and include both in `AllKeys()`.

2. In `pkg/config/types.go`:
Add `Cartesia CartesiaProviderConfig` to `ProvidersConfig`:
```go
type ProvidersConfig struct {
	Gemini   GeminiProviderConfig   `yaml:"gemini,omitempty" json:"gemini,omitempty"`
	Cartesia CartesiaProviderConfig `yaml:"cartesia,omitempty" json:"cartesia,omitempty"`
	// ...
```
And declare the struct:
```go
type CartesiaProviderConfig struct {
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}
```

3. In `pkg/config/presets.go`:
Add `cartesia` to `TTSPresets`:
```go
	"cartesia": {
		Type:         "builtin",
		BuiltinName:  "cartesia",
		Model:        "sonic-3.6",
		DefaultVoice: "db6b0ed5-d5d3-463d-ae85-518a07d3c2b4",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
```
Add `cartesia` to `STTPresets`:
```go
	"cartesia": {
		Type:        "builtin",
		BuiltinName: "cartesia",
		Model:       "ink-whisper",
	},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestCartesiaConfigAndPresets ./pkg/config/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/keys.go pkg/config/types.go pkg/config/presets.go pkg/config/types_test.go
git commit -m "feat(config): add Cartesia provider keys, config struct, and presets"
```

---

### Task 2: Provider Key Mapping and Credential Detection

**Files:**
- Modify: `pkg/media/exports.go:21-76`
- Modify: `pkg/media/catalog.go:98-115`
- Test: `pkg/media/key_test.go`
- Test: `pkg/media/catalog_test.go`

- [ ] **Step 1: Write the failing tests for key mapping and credential detection**

In `pkg/media/key_test.go`, add test cases for `TTSKeyFor` and `STTKeyFor`:

```go
func TestCartesiaKeyFor(t *testing.T) {
	ttsCases := []config.TTSConfig{
		{Type: "cartesia"},
		{Type: "builtin", BuiltinName: "cartesia"},
	}
	for _, tc := range ttsCases {
		got, ok := TTSKeyFor(tc)
		if !ok || got != provider.KeyTTSCartesia {
			t.Errorf("TTSKeyFor(%+v) = %q, %v; want %q, true", tc, got, ok, provider.KeyTTSCartesia)
		}
	}

	sttCases := []config.STTConfig{
		{Type: "cartesia"},
		{Type: "builtin", BuiltinName: "cartesia"},
	}
	for _, tc := range sttCases {
		got, ok := STTKeyFor(tc)
		if !ok || got != provider.KeySTTCartesia {
			t.Errorf("STTKeyFor(%+v) = %q, %v; want %q, true", tc, got, ok, provider.KeySTTCartesia)
		}
	}
}
```

In `pkg/media/catalog_test.go`, add:
```go
func TestCartesiaKeyPresentWithSharedKey(t *testing.T) {
	cfg := config.TTSConfig{Type: "builtin", BuiltinName: "cartesia"}
	if KeyPresentWithSharedKey(cfg, "") {
		t.Error("expected false with no key")
	}
	if !KeyPresentWithSharedKey(cfg, "sk_car_shared") {
		t.Error("expected true with sharedKey")
	}
	t.Setenv("CARTESIA_API_KEY", "sk_car_env")
	if !KeyPresentWithSharedKey(cfg, "") {
		t.Error("expected true with CARTESIA_API_KEY env set")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v -run "TestCartesiaKeyFor|TestCartesiaKeyPresentWithSharedKey" ./pkg/media/`  
Expected: FAIL (`TTSKeyFor`, `STTKeyFor`, and `KeyPresentWithSharedKey` don't recognize Cartesia yet).

- [ ] **Step 3: Implement key resolution and credential detection**

1. In `pkg/media/exports.go`:
In `TTSKeyFor`:
```go
	case "cartesia":
		return provider.KeyTTSCartesia, true
	case "builtin":
		switch cfg.BuiltinName {
		case "cartesia":
			return provider.KeyTTSCartesia, true
		// existing cases...
```
In `STTKeyFor`:
```go
	case "cartesia":
		return provider.KeySTTCartesia, true
	case "builtin":
		if cfg.BuiltinName == "cartesia" {
			return provider.KeySTTCartesia, true
		}
		return "", false
```
Update `BuildSTT` payload to pass `STTBuildPayload` with `Config` and `SharedKey` if needed, or pass `SharedKey` in JSON payload.

2. In `pkg/media/catalog.go`:
In `KeyPresentWithSharedKey`:
```go
	if strings.EqualFold(strings.TrimSpace(cfg.BuiltinName), "cartesia") || strings.EqualFold(strings.TrimSpace(cfg.Type), "cartesia") {
		return strings.TrimSpace(os.Getenv("CARTESIA_API_KEY")) != ""
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v -run "TestCartesiaKeyFor|TestCartesiaKeyPresentWithSharedKey" ./pkg/media/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/media/exports.go pkg/media/catalog.go pkg/media/key_test.go pkg/media/catalog_test.go
git commit -m "feat(media): map Cartesia provider keys and check credentials"
```

---

### Task 3: Cartesia TTS Client (`pkg/provider/ttscartesia`)

**Files:**
- Create: `pkg/provider/ttscartesia/ttscartesia.go`
- Create: `pkg/provider/ttscartesia/client.go`
- Test: `pkg/provider/ttscartesia/client_test.go`

- [ ] **Step 1: Write the failing unit tests for Cartesia TTS**

Create `pkg/provider/ttscartesia/client_test.go`:
- Test synthesis sending correct `model_id: "sonic-3.6"`, `output_format` (wav, pcm_s16le, 44100), `generation_config` with clamped speed, `Cartesia-Version: 2026-08-14`, and Bearer auth.
- Test `ListVoices` parsing paginated voices from `GET /voices?limit=100&expand[]=preview_file_url` using `starting_after`.
- Test rate-limit retry on 429 using `Retry-After`.
- Test structured error response handling (401/400).
- Test `LastUsage()` character count and request count.

```go
package ttscartesia

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/provider/ttscartesia/`  
Expected: FAIL (package `pkg/provider/ttscartesia` does not exist).

- [ ] **Step 3: Implement `pkg/provider/ttscartesia`**

Create `pkg/provider/ttscartesia/ttscartesia.go`:
```go
package ttscartesia

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeyTTSCartesia),
			Family:      provider.FamilyTTS,
			Label:       "Cartesia Sonic (Cloud, metered)",
			Description: "Ultra-fast neural voice synthesis via Cartesia Sonic.",
			Source:      "http",
			Features: []provider.Feature{
				provider.FeatureMetered,
				provider.FeatureKeyRequired,
				provider.FeatureVoiceCatalog,
				provider.FeatureVoiceOptions,
				provider.FeatureSpeechCues,
			},
			Presets: []provider.Preset{
				{
					ID:    "cartesia",
					Order: 8,
					Label: "Cartesia Sonic (Cloud, metered)",
					Description: "Fast, natural speech via Cartesia Sonic. Set key here or via CARTESIA_API_KEY; charges per request.",
					Config: map[string]interface{}{
						"type":          "builtin",
						"builtin_name":  "cartesia",
						"model":         "sonic-3.6",
						"default_voice": "db6b0ed5-d5d3-463d-ae85-518a07d3c2b4",
						"pitch":         1.0,
						"speech_rate":   1.0,
						"master_volume": 1.0,
						"auto_play":     true,
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
			return NewCartesiaTTSClient(payload.Config, payload.SharedKey)
		},
	})
}
```

Create `pkg/provider/ttscartesia/client.go`:
- Implement `CartesiaTTSClient` struct with `apiKey`, `model`, `baseURL`, `client`, `usageMu`, `lastUsage`.
- Pin constant `cartesiaAPIVersion = "2026-08-14"`.
- Support credentials in order: `cfg.APIKey` -> `sharedKey` -> `os.Getenv("CARTESIA_API_KEY")`.
- Call `trace.RegisterSecret(apiKey)`.
- Implement `Synthesize`, `ListVoices`, `VoiceOptions`, `SpeechCueCapabilities`, `Metered`, `LastUsage`, `SetLogger`.
- Implement structured Cartesia error parsing (`error_code`, `title`, `message`).
- Implement 429 retry honoring `Retry-After`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/provider/ttscartesia/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/ttscartesia/
git commit -m "feat(provider): implement Cartesia TTS client and descriptor"
```

---

### Task 4: Cartesia STT Client (`pkg/provider/sttcartesia`)

**Files:**
- Create: `pkg/provider/sttcartesia/sttcartesia.go`
- Create: `pkg/provider/sttcartesia/client.go`
- Test: `pkg/provider/sttcartesia/client_test.go`

- [ ] **Step 1: Write the failing unit tests for Cartesia STT**

Create `pkg/provider/sttcartesia/client_test.go`:
- Test `Transcribe`: Mock server on `POST /stt` expecting multipart file upload with `model: ink-whisper`, file part, `Cartesia-Version: 2026-08-14`, and Bearer auth; returns JSON with `{"type":"transcript","text":"Open the door","duration":2.5}`.
- Test `LastUsage()` tracking duration and requests.
- Test 429 rate limit retry.
- Test structured error response handling.

```go
package sttcartesia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/provider/sttcartesia/`  
Expected: FAIL (package does not exist).

- [ ] **Step 3: Implement `pkg/provider/sttcartesia`**

Create `pkg/provider/sttcartesia/sttcartesia.go`:
```go
package sttcartesia

import (
	"context"
	"encoding/json"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider"
)

func init() {
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{
			ID:          string(provider.KeySTTCartesia),
			Family:      provider.FamilySTT,
			Label:       "Cartesia Ink (Cloud, metered)",
			Description: "Cloud speech transcription via Cartesia Ink Whisper.",
			Source:      "http",
			Features: []provider.Feature{
				provider.FeatureMetered,
				provider.FeatureKeyRequired,
			},
			Presets: []provider.Preset{
				{
					ID:    "cartesia",
					Order: 4,
					Label: "Cartesia Ink (Cloud, metered)",
					Description: "Accurate cloud transcription with Cartesia Ink Whisper. Set key here or via CARTESIA_API_KEY; charges per request.",
					Config: map[string]interface{}{
						"type":         "builtin",
						"builtin_name": "cartesia",
						"model":        "ink-whisper",
					},
				},
			},
		},
		Build: func(_ context.Context, raw []byte) (interface{}, error) {
			var cfg config.STTConfig
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &cfg); err != nil {
					return nil, err
				}
			}
			return NewCartesiaSTTClient(cfg, "")
		},
	})
}
```

Create `pkg/provider/sttcartesia/client.go`:
- Implement `CartesiaSTTClient` struct.
- Wire credentials (`cfg.APIKey` -> `sharedKey` -> `CARTESIA_API_KEY`).
- Call `trace.RegisterSecret(apiKey)`.
- Implement `Transcribe(ctx, audioData)` building multipart form with `model` and `file`.
- Parse Cartesia structured errors and implement 429 retry on `Retry-After`.
- Implement `MeteredProvider` (`Metered() bool`, `LastUsage() media.Usage`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/provider/sttcartesia/`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/sttcartesia/
git commit -m "feat(provider): implement Cartesia STT client and descriptor"
```

---

### Task 5: Registry Blank-Import, Drift-Guard Verification, and Catalog Docs Update

**Files:**
- Modify: `pkg/provider/all/all.go`
- Modify: `pkg/provider/all/all_test.go`
- Modify: `pkg/gui/docs/` (generated via `-update-docs`)

- [ ] **Step 1: Blank-import Cartesia providers in `pkg/provider/all/all.go`**

Add blank imports:
```go
	_ "github.com/darkliquid/localrpg/pkg/provider/sttcartesia"
	_ "github.com/darkliquid/localrpg/pkg/provider/ttscartesia"
```

- [ ] **Step 2: Run drift-guard tests**

Run: `go test -v -run "TestTTSDescriptorsBuildAndFeaturesAreBacked|TestSTTAndImageDescriptorsBuild" ./pkg/provider/all/`  
Expected: PASS (verifies all features declared by `tts:cartesia` and `stt:cartesia` are verified and backed).

- [ ] **Step 3: Update documentation catalog**

Run: `go test ./pkg/gui -update-docs`  
Expected: PASS, regenerates `pkg/gui/docs/providers.md` or catalogue tables with Cartesia included.

- [ ] **Step 4: Verify git status of generated docs**

Run: `git status -s pkg/gui/docs/`  
Expected: Shows modified docs files containing Cartesia.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/all/all.go pkg/provider/all/all_test.go pkg/gui/docs/
git commit -m "feat(provider): register Cartesia in all and regenerate catalogue docs"
```

---

### Task 6: Frontend Settings Studio Integration

**Files:**
- Modify: `frontend/src/types.ts:40-60`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [ ] **Step 1: Update frontend type definitions in `frontend/src/types.ts`**

In `frontend/src/types.ts`, add `cartesia` to `ProvidersConfig`:
```typescript
export interface ProvidersConfig {
  gemini?: { api_key?: string };
  cartesia?: { api_key?: string };
  // ...
}
```

- [ ] **Step 2: Update `frontend/src/components/SettingsStudio.tsx`**

1. In Provider API Keys section:
Add Cartesia API Key input:
```tsx
<div>
  <label className="block text-xs text-white/50 mb-1">Cartesia API Key</label>
  <input
    type="password"
    value={config.providers?.cartesia?.api_key || ''}
    onChange={(e) =>
      setConfig({
        ...config,
        providers: {
          ...config.providers,
          cartesia: { ...config.providers?.cartesia, api_key: e.target.value },
        },
      })
    }
    placeholder="sk_car_... or set CARTESIA_API_KEY"
    className="..."
  />
  <p className="text-[11px] text-white/40 mt-1">
    Shared key used for Cartesia Sonic TTS and Ink STT.
  </p>
</div>
```

2. In TTS Provider Dropdown:
Add:
```tsx
<option value="builtin:cartesia">Built-in: Cartesia Sonic (Cloud, metered)</option>
```
When selected, initialize defaults:
```tsx
builtinName === 'cartesia'
  ? {
      model: config.media.tts.model && config.media.tts.model.includes('sonic')
        ? config.media.tts.model
        : 'sonic-3.6',
      default_voice: config.media.tts.default_voice || 'db6b0ed5-d5d3-463d-ae85-518a07d3c2b4',
    }
```

3. In STT Provider Dropdown:
Add:
```tsx
<option value="builtin:cartesia">Built-in: Cartesia Ink (Cloud, metered)</option>
```
When selected, initialize:
```tsx
model: 'ink-whisper'
```

- [ ] **Step 3: Run frontend typecheck**

Run: `mise run test:frontend` (or `cd frontend && npx tsc --noEmit`)  
Expected: PASS with 0 errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add Cartesia provider key and TTS/STT options in Settings"
```

---

### Task 7: End-to-End Verification

**Files:**
- Entire repository

- [ ] **Step 1: Run complete backend test suite**

Run: `mise run test:backend` (or `go test -v -count=1 ./...`)  
Expected: All tests PASS.

- [ ] **Step 2: Run complete frontend verification**

Run: `mise run test:frontend`  
Expected: PASS.

- [ ] **Step 3: Run repository linters**

Run: `mise run lint`  
Expected: Clean lint run (markdownlint, goreleaser, actionlint, go vet).

- [ ] **Step 4: Final verification commit if needed**

Ensure all changes are cleanly committed.
