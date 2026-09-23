# Google Gemini TTS Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Implement a Google Gemini text-to-speech (TTS) provider in `pkg/media` using the `google.golang.org/genai` SDK, supporting preview models (`gemini-3.1-flash-tts-preview`, `gemini-2.5-flash-preview-tts`, `gemini-2.5-pro-preview-tts`), 30 prebuilt voices, audio tags, and seamless integration with the existing LocalRPG TTS pipeline and settings GUI.

**Architecture:** A new `GeminiTTSClient` implements `TTSClient`, `VoiceCatalog`, `MeteredProvider`, and `SpeechCueAdvertiser`. Synthesis uses `client.Models.GenerateContent` with `ResponseModalities: ["AUDIO"]` and `SpeechConfig` specifying prebuilt voice names. Credentials resolve through `media.tts.api_key` -> `providers.gemini.api_key` -> `GEMINI_API_KEY` -> `GOOGLE_API_KEY`. Audio tags (`[whispers]`, `[shouting]`, etc.) are declared via `SpeechCueCapabilities` so the GM prompt harness and player stage direction modes operate out-of-the-box.

**Tech Stack:** Go (1.27.1), `google.golang.org/genai` (v1.71.0), React 19, TypeScript, Tailwind CSS v4.

---

### File Map

| Action | File | Responsibility |
|---|---|---|
| Modify | `pkg/config/presets.go` | Define `gemini-3.1-flash-tts`, `gemini-2.5-flash-tts`, `gemini-2.5-pro-tts` in `TTSPresets` |
| Modify | `pkg/config/presets_test.go` | Unit test ensuring Gemini TTS presets are properly defined |
| Modify | `pkg/media/catalog.go` | Update `ProviderKey` for Gemini TTS and add `KeyPresentWithSharedKey` |
| Modify | `pkg/media/catalog_test.go` | Unit tests for `ProviderKey` and `KeyPresentWithSharedKey` with Gemini configs |
| Create | `pkg/media/gemini_tts.go` | `GeminiTTSClient` implementation, 30-voice catalog, audio tags capabilities, error mapping |
| Create | `pkg/media/gemini_tts_test.go` | Offline unit tests for API key resolution, voice catalog, speech cues, and synthesis |
| Modify | `pkg/media/providers.go` | Add `NewTTSClientWithSharedKey` and wire `"gemini"` and `"builtin":"gemini"` |
| Modify | `pkg/media/providers_test.go` | Unit tests for building Gemini TTS clients via factory |
| Modify | `pkg/gui/tts_inspect.go` | Use shared Gemini key and recognize Gemini TTS as `KeyRequired` |
| Modify | `pkg/gui/service.go` | Pass shared Gemini key in `ttsClientFor` and `TestProvider` |
| Modify | `frontend/src/types.ts` | Add `'gemini'` to `TTSConfig.type` union |
| Modify | `frontend/src/templates/providerPresets.ts` | Add 3 Gemini TTS presets |
| Modify | `frontend/src/components/SettingsStudio.tsx` | Add Gemini TTS engine options, model pills, and API key override UI |

---

### Task 1: Gemini TTS Presets

**Files:**
- Modify: `pkg/config/presets.go:150-155`
- Modify: `pkg/config/presets_test.go:40-60`

- [x] **Step 1: Write failing test for Gemini TTS presets**

Add `TestGetGeminiTTSPresets` in `pkg/config/presets_test.go`:

```go
func TestGetGeminiTTSPresets(t *testing.T) {
	expected := []struct {
		id    string
		model string
	}{
		{"gemini-3.1-flash-tts", "gemini-3.1-flash-tts-preview"},
		{"gemini-2.5-flash-tts", "gemini-2.5-flash-preview-tts"},
		{"gemini-2.5-pro-tts", "gemini-2.5-pro-preview-tts"},
	}

	for _, tc := range expected {
		p, ok := TTSPresets[tc.id]
		if !ok {
			t.Fatalf("expected preset %q to exist in TTSPresets", tc.id)
		}
		if p.Type != "gemini" {
			t.Errorf("preset %q: expected Type 'gemini', got %q", tc.id, p.Type)
		}
		if p.Model != tc.model {
			t.Errorf("preset %q: expected Model %q, got %q", tc.id, tc.model, p.Model)
		}
		if p.DefaultVoice != "Aoede" {
			t.Errorf("preset %q: expected DefaultVoice 'Aoede', got %q", tc.id, p.DefaultVoice)
		}
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestGetGeminiTTSPresets ./pkg/config/`
Expected: FAIL (`expected preset "gemini-3.1-flash-tts" to exist in TTSPresets`)

- [x] **Step 3: Add presets to `TTSPresets`**

In `pkg/config/presets.go`, add to `TTSPresets`:

```go
	"gemini-3.1-flash-tts": {
		Type:         "gemini",
		Model:        "gemini-3.1-flash-tts-preview",
		DefaultVoice: "Aoede",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"gemini-2.5-flash-tts": {
		Type:         "gemini",
		Model:        "gemini-2.5-flash-preview-tts",
		DefaultVoice: "Aoede",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
	"gemini-2.5-pro-tts": {
		Type:         "gemini",
		Model:        "gemini-2.5-pro-preview-tts",
		DefaultVoice: "Aoede",
		Pitch:        1.0,
		SpeechRate:   1.0,
		MasterVolume: 1.0,
	},
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestGetGeminiTTSPresets ./pkg/config/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/config/presets.go pkg/config/presets_test.go
git commit -m "feat(config): add Google Gemini TTS presets"
```

---

### Task 2: ProviderKey and KeyPresent Updates in Catalog

**Files:**
- Modify: `pkg/media/catalog.go:62-130`
- Modify: `pkg/media/catalog_test.go` (or add unit test in `pkg/media/` test suite)

- [x] **Step 1: Write failing tests for ProviderKey and KeyPresent with Gemini**

Create or update test in `pkg/media/catalog_test.go`:

```go
func TestGeminiTTSProviderKeyAndKeyPresent(t *testing.T) {
	// ProviderKey test
	geminiCfg := config.TTSConfig{Type: "gemini"}
	if key := ProviderKey(geminiCfg); key != "gemini:tts" {
		t.Errorf("expected ProviderKey 'gemini:tts', got %q", key)
	}

	builtinGeminiCfg := config.TTSConfig{Type: "builtin", BuiltinName: "gemini"}
	if key := ProviderKey(builtinGeminiCfg); key != "builtin:gemini" {
		t.Errorf("expected ProviderKey 'builtin:gemini', got %q", key)
	}

	// KeyPresentWithSharedKey test
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	if KeyPresentWithSharedKey(geminiCfg, "") {
		t.Errorf("expected false when no key set")
	}

	if !KeyPresentWithSharedKey(geminiCfg, "shared-secret") {
		t.Errorf("expected true when shared key provided")
	}

	geminiCfgWithKey := config.TTSConfig{Type: "gemini", APIKey: "own-key"}
	if !KeyPresentWithSharedKey(geminiCfgWithKey, "") {
		t.Errorf("expected true when config has APIKey")
	}

	t.Setenv("GEMINI_API_KEY", "env-key")
	if !KeyPresentWithSharedKey(geminiCfg, "") {
		t.Errorf("expected true when GEMINI_API_KEY set")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestGeminiTTSProviderKeyAndKeyPresent ./pkg/media/`
Expected: FAIL (`undefined: KeyPresentWithSharedKey` or `expected ProviderKey 'gemini:tts'`)

- [x] **Step 3: Update `ProviderKey`, `KeyPresent`, and add `KeyPresentWithSharedKey`**

In `pkg/media/catalog.go`:
In `ProviderKey(cfg config.TTSConfig)`:
```go
	case "", "disabled":
		return "disabled"
	case "gemini":
		return "gemini:tts"
	case "builtin":
```

Update `KeyPresent`:
```go
// KeyPresent reports whether a configuration has a usable credential.
func KeyPresent(cfg config.TTSConfig) bool {
	return KeyPresentWithSharedKey(cfg, "")
}

// KeyPresentWithSharedKey reports whether a configuration has a usable credential,
// either in config, through the shared provider key, or via documented environment variables.
func KeyPresentWithSharedKey(cfg config.TTSConfig, sharedKey string) bool {
	if strings.TrimSpace(cfg.APIKey) != "" {
		return true
	}
	if strings.TrimSpace(sharedKey) != "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(cfg.BuiltinName), "elevenlabs") {
		return strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY")) != ""
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Type), "gemini") || strings.EqualFold(strings.TrimSpace(cfg.BuiltinName), "gemini") {
		return strings.TrimSpace(os.Getenv("GEMINI_API_KEY")) != "" || strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")) != ""
	}
	return false
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `go test -v -run TestGeminiTTSProviderKeyAndKeyPresent ./pkg/media/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media/catalog.go pkg/media/catalog_test.go
git commit -m "feat(media): add Gemini TTS support to ProviderKey and KeyPresent"
```

---

### Task 3: GeminiTTSClient Implementation and Offline Unit Tests

**Files:**
- Create: `pkg/media/gemini_tts.go`
- Create: `pkg/media/gemini_tts_test.go`

- [x] **Step 1: Write failing offline tests for `GeminiTTSClient`**

Create `pkg/media/gemini_tts_test.go`:

```go
package media_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestResolveGeminiTTSAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	// 1. None provided
	_, err := media.ResolveGeminiTTSAPIKey("", "")
	if err == nil {
		t.Errorf("expected error when no key provided")
	}

	// 2. Fallback to GOOGLE_API_KEY
	t.Setenv("GOOGLE_API_KEY", "env-google-key")
	k, err := media.ResolveGeminiTTSAPIKey("", "")
	if err != nil || k != "env-google-key" {
		t.Errorf("expected env-google-key, got %q", k)
	}

	// 3. Fallback to GEMINI_API_KEY
	t.Setenv("GEMINI_API_KEY", "env-gemini-key")
	k, err = media.ResolveGeminiTTSAPIKey("", "")
	if err != nil || k != "env-gemini-key" {
		t.Errorf("expected env-gemini-key, got %q", k)
	}

	// 4. Shared provider key
	k, err = media.ResolveGeminiTTSAPIKey("", "shared-key")
	if err != nil || k != "shared-key" {
		t.Errorf("expected shared-key, got %q", k)
	}

	// 5. Config TTS key override
	k, err = media.ResolveGeminiTTSAPIKey("override-key", "shared-key")
	if err != nil || k != "override-key" {
		t.Errorf("expected override-key, got %q", k)
	}
}

func TestGeminiTTSVoiceCatalog(t *testing.T) {
	client := media.NewGeminiTTSClientOffline("gemini-3.1-flash-tts-preview", "Aoede")
	voices, err := client.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(voices) != 30 {
		t.Fatalf("expected 30 voices, got %d", len(voices))
	}

	for _, v := range voices {
		if v.ID == "" || v.Name == "" {
			t.Errorf("expected non-empty voice ID and Name: %+v", v)
		}
		if len(v.Tags) == 0 {
			t.Errorf("expected non-empty tags for voice %s", v.ID)
		}
	}
}

func TestGeminiTTSSpeechCueCapabilities(t *testing.T) {
	client := media.NewGeminiTTSClientOffline("gemini-3.1-flash-tts-preview", "Aoede")
	caps := client.SpeechCueCapabilities()
	if !caps.AudioTags {
		t.Errorf("expected AudioTags to be true")
	}
	if caps.MarkdownEmphasis {
		t.Errorf("expected MarkdownEmphasis to be false")
	}
	if len(caps.SupportedTags) == 0 {
		t.Errorf("expected non-empty SupportedTags")
	}
	if !strings.Contains(caps.PromptGuidance, "[tag]") {
		t.Errorf("expected PromptGuidance to describe tag usage")
	}
}

func TestGeminiTTSSynthesize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)

		if !strings.Contains(bodyStr, "Hello world") {
			t.Errorf("expected text in request body, got: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "Kore") {
			t.Errorf("expected voice Kore in request body, got: %s", bodyStr)
		}

		w.Header().Set("Content-Type", "application/json")
		// Return dummy base64 RIFF wav audio bytes: "RIFF....WAVE"
		fmt.Fprint(w, `{
			"candidates": [
				{
					"content": {
						"parts": [
							{
								"inlineData": {
									"data": "UklGRiQAAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQAAAAA=",
									"mimeType": "audio/wav"
								}
							}
						],
						"role": "model"
					}
				}
			]
		}`)
	}))
	defer server.Close()

	ctx := context.Background()
	genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     "test-key",
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	})
	if err != nil {
		t.Fatalf("create genai client: %v", err)
	}

	ttsClient, err := media.NewGeminiTTSClientWithClient(genaiClient, config.TTSConfig{
		Model:        "gemini-3.1-flash-tts-preview",
		DefaultVoice: "Aoede",
	})
	if err != nil {
		t.Fatalf("NewGeminiTTSClientWithClient: %v", err)
	}

	audioBytes, err := ttsClient.Synthesize(ctx, "Hello world", &entity.VoiceConfig{VoiceID: "Kore"})
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if ext := media.AudioExtension(audioBytes); ext != ".wav" {
		t.Errorf("expected .wav audio extension, got %s", ext)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestResolveGeminiTTSAPIKey ./pkg/media/`
Expected: FAIL (`undefined: media.ResolveGeminiTTSAPIKey`)

- [x] **Step 3: Implement `pkg/media/gemini_tts.go`**

Create `pkg/media/gemini_tts.go`:
- Define `ErrGeminiTTSAPIKeyRequired = errors.New("gemini: an API key is required for speech synthesis; set media.tts.api_key, providers.gemini.api_key, or GEMINI_API_KEY")`
- `ResolveGeminiTTSAPIKey(ttsKey, sharedKey string) (string, error)`
- Define `geminiPrebuiltVoices = []ProviderVoice{...}` (all 30 voices with ID, Name, Description, and Tags as specified in the design doc).
- `GeminiTTSClient` struct:
  - `client *genai.Client`
  - `model string`
  - `defaultVoice string`
  - `logger trace.Logger`
- Implement `Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error)`:
  - If text is empty, return error.
  - Determine voice name: `voice != nil && voice.VoiceID != ""` -> `voice.VoiceID`, otherwise `c.defaultVoice`. If empty, `"Aoede"`.
  - Config:
    ```go
    cfg := &genai.GenerateContentConfig{
        ResponseModalities: []string{"AUDIO"},
        SpeechConfig: &genai.SpeechConfig{
            VoiceConfig: &genai.VoiceConfig{
                PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{
                    VoiceName: voiceName,
                },
            },
        },
    }
    ```
  - Call `c.client.Models.GenerateContent(ctx, c.model, genai.Text(text), cfg)`
  - Check candidates, parts, and `part.InlineData.Data`.
  - Return bytes or descriptive error.
- Implement `ListVoices(ctx context.Context) ([]ProviderVoice, error)`:
  - Return copy of `geminiPrebuiltVoices`.
- Implement `Metered() bool`: returns `true`.
- Implement `SpeechCueCapabilities() SpeechCueCapabilities`:
  - `AudioTags: true`
  - `MarkdownEmphasis: false`
  - `SupportedTags: [...]`
  - `PromptGuidance: "Use [tag] inline modifiers in the transcript to control delivery. Examples: [whispers], [shouting], [laughs], [sighs], [trembling]. Tags can be combined and placed mid-sentence. Use English tags even for non-English text."`
- Implement `SupportsMarkdown() bool`: returns `false`.
- Implement `SetLogger(logger trace.Logger)`.
- Constructors:
  - `NewGeminiTTSClientOffline(model, defaultVoice string) *GeminiTTSClient` (for tests/catalog)
  - `NewGeminiTTSClientWithClient(client *genai.Client, cfg config.TTSConfig) (*GeminiTTSClient, error)`
  - `NewGeminiTTSClient(cfg config.TTSConfig, sharedKey string) (*GeminiTTSClient, error)`

- [x] **Step 4: Run tests to verify they pass**

Run: `go test -v -run "TestGeminiTTS" ./pkg/media/`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add pkg/media/gemini_tts.go pkg/media/gemini_tts_test.go
git commit -m "feat(media): implement Google Gemini TTS client"
```

---

### Task 4: Factory Wiring and Service Integration

**Files:**
- Modify: `pkg/media/providers.go`
- Modify: `pkg/media/providers_test.go`
- Modify: `pkg/gui/tts_inspect.go`
- Modify: `pkg/gui/service.go`

- [x] **Step 1: Write failing factory test**

In `pkg/media/providers_test.go`:

```go
func TestNewTTSClientBuildsGemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "env-key")

	// 1. type: "gemini"
	client, err := media.NewTTSClientWithSharedKey(config.TTSConfig{
		Type: "gemini",
	}, "")
	if err != nil {
		t.Fatalf("expected gemini client to build, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	// 2. type: "builtin", builtin_name: "gemini"
	client, err = media.NewTTSClientWithSharedKey(config.TTSConfig{
		Type:        "builtin",
		BuiltinName: "gemini",
	}, "")
	if err != nil {
		t.Fatalf("expected builtin gemini client to build, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestNewTTSClientBuildsGemini ./pkg/media/`
Expected: FAIL (`undefined: media.NewTTSClientWithSharedKey`)

- [x] **Step 3: Implement `NewTTSClientWithSharedKey` in `providers.go`**

In `pkg/media/providers.go`:
```go
func NewTTSClient(cfg config.TTSConfig) (TTSClient, error) {
	return NewTTSClientWithSharedKey(cfg, "")
}

func NewTTSClientWithSharedKey(cfg config.TTSConfig, sharedKey string) (TTSClient, error) {
	switch cfg.Type {
	case "disabled", "":
		return &disabledTTSClient{}, nil
	case "gemini":
		return NewGeminiTTSClient(cfg, sharedKey)
	case "builtin":
		switch cfg.BuiltinName {
		case "gemini":
			return NewGeminiTTSClient(cfg, sharedKey)
		case "sherpa-onnx", "kokoro":
			modelDir := cfg.ModelPath
			if modelDir == "" {
				modelDir = "./cache/models/tts/kokoro"
			}
			return NewSherpaTTSClient(modelDir), nil
		case "native-os":
			return NewNativeOSTTSClient(), nil
		case "elevenlabs":
			return NewElevenLabsTTSClient(cfg)
		default:
			return &echoTTSClient{}, nil
		}
	case "cli":
		return &cliTTSClient{command: cfg.Command, args: cfg.Args}, nil
	case "http":
		return &httpTTSClient{endpoint: cfg.Endpoint, model: cfg.Model, apiKey: cfg.APIKey, client: &http.Client{Timeout: 30 * time.Second}}, nil
	default:
		return nil, fmt.Errorf("unsupported tts provider type: %s", cfg.Type)
	}
}
```

- [x] **Step 4: Update `pkg/gui/tts_inspect.go` and `pkg/gui/service.go`**

In `pkg/gui/tts_inspect.go`:
- Update `InspectTTS`:
  ```go
  sharedKey := ""
  if s.configMgr != nil && s.configMgr.Get() != nil {
      sharedKey = s.configMgr.Get().Providers.Gemini.APIKey
  }
  isGemini := strings.EqualFold(strings.TrimSpace(cfg.Type), "gemini") || strings.EqualFold(strings.TrimSpace(cfg.BuiltinName), "gemini")
  response := &TTSInspectResponseDTO{
      ProviderKey: media.ProviderKey(cfg),
      KeyPresent:  media.KeyPresentWithSharedKey(cfg, sharedKey),
      KeyRequired: strings.EqualFold(strings.TrimSpace(cfg.BuiltinName), "elevenlabs") || isGemini,
      Catalog:     VoiceCatalogDTO{Voices: []media.ProviderVoice{}},
  }
  ```
- Update `ttsClientFor(cfg config.TTSConfig)`:
  ```go
  func (s *Service) ttsClientFor(cfg config.TTSConfig) (media.TTSClient, error) {
      if s.newTTSClient != nil {
          return s.newTTSClient(cfg)
      }
      sharedKey := ""
      if s.configMgr != nil && s.configMgr.Get() != nil {
          sharedKey = s.configMgr.Get().Providers.Gemini.APIKey
      }
      return media.NewTTSClientWithSharedKey(cfg, sharedKey)
  }
  ```

In `pkg/gui/service.go`:
- In `SynthesizeSegment` (line ~1420): replace `media.NewTTSClient(cfg.Media.TTS)` with `s.ttsClientFor(cfg.Media.TTS)`.
- In `TestProvider` for `"tts"` (line ~2392): replace `media.NewTTSClient(ttsCfg)` with `media.NewTTSClientWithSharedKey(ttsCfg, cfg.Providers.Gemini.APIKey)`.

- [x] **Step 5: Run tests to verify they pass**

Run: `go test -v -run TestNewTTSClientBuildsGemini ./pkg/media/`
Run: `go test -v ./pkg/gui/`
Expected: PASS

- [x] **Step 6: Commit**

```bash
git add pkg/media/providers.go pkg/media/providers_test.go pkg/gui/tts_inspect.go pkg/gui/service.go
git commit -m "feat(media): wire Gemini TTS client into factory and gui service"
```

---

### Task 5: Frontend Types, Presets, and Settings Studio UI

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/templates/providerPresets.ts`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Update `frontend/src/types.ts`**

In `frontend/src/types.ts:406`:
```typescript
export interface TTSConfig {
  type: 'builtin' | 'http' | 'cli' | 'disabled' | 'gemini';
  builtin_name?: string;
  // ...
```

- [x] **Step 2: Add presets to `frontend/src/templates/providerPresets.ts`**

In `frontend/src/templates/providerPresets.ts`, add to `TTS_PRESETS`:
```typescript
  'gemini-3.1-flash-tts': {
    label: 'Google Gemini 3.1 Flash TTS (Preview)',
    description: 'Fast, natural cloud TTS with 30 prebuilt voices and audio tags support.',
    config: {
      type: 'gemini',
      model: 'gemini-3.1-flash-tts-preview',
      default_voice: 'Aoede',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
  'gemini-2.5-flash-tts': {
    label: 'Google Gemini 2.5 Flash TTS (Preview)',
    description: 'Lightweight cloud TTS with natural prosody and voice acting cues.',
    config: {
      type: 'gemini',
      model: 'gemini-2.5-flash-preview-tts',
      default_voice: 'Aoede',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
  'gemini-2.5-pro-tts': {
    label: 'Google Gemini 2.5 Pro TTS (Preview)',
    description: 'Highest quality expressive cloud TTS for nuanced storytelling.',
    config: {
      type: 'gemini',
      model: 'gemini-2.5-pro-preview-tts',
      default_voice: 'Aoede',
      pitch: 1.0,
      speech_rate: 1.0,
      auto_play: true,
      master_volume: 1.0,
    },
  },
```

- [x] **Step 3: Update `SettingsStudio.tsx`**

In `frontend/src/components/SettingsStudio.tsx`:
1. In the `TTS Engine` select options:
   Add `<option value="gemini">Google Gemini TTS (Cloud)</option>`
   Add `<option value="builtin:gemini">Built-in: Google Gemini TTS (Cloud, metered)</option>`
2. In the `onChange` handler for `TTS Engine`:
   If `val === 'gemini'` or `builtinName === 'gemini'`, set sensible defaults (`model: config.media.tts.model || 'gemini-3.1-flash-tts-preview'`, `default_voice: config.media.tts.default_voice || 'Aoede'`).
3. If `config.media.tts.type === 'gemini' || (config.media.tts.type === 'builtin' && config.media.tts.builtin_name === 'gemini')`:
   Render:
   - Model input with 3 preset pills:
     - `gemini-3.1-flash-tts-preview` ("3.1 Flash TTS")
     - `gemini-2.5-flash-preview-tts` ("2.5 Flash TTS")
     - `gemini-2.5-pro-preview-tts` ("2.5 Pro TTS")
   - API key notice explaining it uses `providers.gemini.api_key` if configured, or `media.tts.api_key`, or `GEMINI_API_KEY`.
4. In the `inspect?.key_required` block, update helper text if it's Gemini TTS to mention `providers.gemini.api_key` and `GEMINI_API_KEY`.

- [x] **Step 4: Run frontend typecheck**

Run: `mise run test:frontend` (or `cd frontend && npx tsc --noEmit`)
Expected: PASS (0 errors)

- [x] **Step 5: Commit**

```bash
git add frontend/src/types.ts frontend/src/templates/providerPresets.ts frontend/src/components/SettingsStudio.tsx
git commit -m "feat(frontend): add Google Gemini TTS support to SettingsStudio"
```

---

### Task 6: Full Verification and Build

- [x] **Step 1: Run all Go tests**

Run: `go test -v -count=1 ./...`
Expected: PASS (all tests pass)

- [x] **Step 2: Run frontend build**

Run: `mise run build:frontend`
Expected: SUCCESS

- [x] **Step 3: Verify clean git status (restore dist/.gitkeep if removed)**

Run: `git status`
Expected: Working tree clean (restore `pkg/gui/dist/.gitkeep` if vite deleted it).

- [x] **Step 4: Commit and update plan**

```bash
git add docs/superpowers/plans/2026-09-23-gemini-tts-provider.md
git commit -m "docs: mark all gemini tts provider plan tasks complete"
```
