# Gemini Native Audio TTS Provider Design

**Date:** 2026-09-23  
**Phase:** 3 of Google Gemini Integration (TTS)  
**Status:** Approved

---

## Goal

Implement a `GeminiTTSClient` in `pkg/media` that uses the official `google.golang.org/genai` SDK to synthesise speech via Google Gemini's native audio generation models. This enables per-character voiced narration using the Gemini TTS preview models without any external TTS server.

---

## Background

The Gemini API exposes three TTS-optimised models (in preview as of 2026-09-23):

| Model ID | Description |
|---|---|
| `gemini-3.1-flash-tts-preview` | Newest, fastest, recommended default |
| `gemini-2.5-flash-preview-tts` | Previous-gen fast |
| `gemini-2.5-pro-preview-tts` | Highest quality, higher cost |

All three support single-speaker synthesis with 30 prebuilt voices, audio tags, and automatic language detection across 50+ languages.

The Go SDK (`google.golang.org/genai v1.71.0`) does not yet expose the newer `interactions` REST surface shown in the Python docs. TTS is accessed via `client.Models.GenerateContent(ctx, model, contents, cfg)` with `cfg.ResponseModalities = []string{"AUDIO"}` and `cfg.SpeechConfig`. Audio bytes are returned in `resp.Candidates[0].Content.Parts[0].InlineData.Data` as raw PCM (24kHz, 16-bit signed little-endian) wrapped in a WAV container. The existing `AudioExtension` function already detects the `RIFF` WAV header, so the audio cache pipeline requires no changes.

---

## Architecture

### SDK call path

```go
cfg := &genai.GenerateContentConfig{
    ResponseModalities: []string{"AUDIO"},
    SpeechConfig: &genai.SpeechConfig{
        VoiceConfig: &genai.VoiceConfig{
            PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{
                VoiceName: voiceName, // e.g. "Aoede"
            },
        },
    },
}
resp, err := client.Models.GenerateContent(ctx, model, genai.Text(text), cfg)
// audio bytes: resp.Candidates[0].Content.Parts[0].InlineData.Data
```

One `genai.Client` is created at construction time with `BackendGeminiAPI` and reused for every segment call (the SDK client is thread-safe).

### Integration with existing pipeline

`GeminiTTSClient.Synthesize(ctx, text, voice)` is called per segment by the existing `TTSPipeline` — no pipeline changes needed. Voice resolution:

1. `voice != nil && voice.VoiceID != ""` → use `voice.VoiceID` as the Gemini prebuilt voice name directly.
2. Otherwise → fall back to `defaultVoice` (from `cfg.DefaultVoice`, default `"Aoede"`).

This means:
- **Hand-authored characters**: set `voice: Aoede` in frontmatter → used directly.
- **Auto-assigned NPCs**: `AssignVoiceProfile` tag-scoring selects a `VoiceProfile` whose `voice_id` is a Gemini voice name, which is then stored in `entity.VoiceConfig.VoiceID`.

---

## `GeminiTTSClient` struct

```go
// pkg/media/gemini_tts.go
type GeminiTTSClient struct {
    client       *genai.Client
    model        string  // default: "gemini-3.1-flash-tts-preview"
    defaultVoice string  // default: "Aoede"
    logger       trace.Logger
}
```

### Interfaces implemented

| Interface | Notes |
|---|---|
| `TTSClient` | `Synthesize(ctx, text, voice) ([]byte, error)` |
| `VoiceCatalog` | `ListVoices(ctx) ([]ProviderVoice, error)` — static, no network call |
| `MeteredProvider` | `Metered() bool` → `true` |
| `SpeechCueAdvertiser` | `SpeechCueCapabilities()` → `AudioTags: true`, tags list, prompt guidance |
| `MarkdownAware` | `SupportsMarkdown() bool` → `false` (Markdown is stripped before synthesis) |

`VoiceOptions` is **not** implemented — Gemini TTS has no per-request stability/similarity tunables to expose.

### `SpeechCueCapabilities`

```go
SpeechCueCapabilities{
    AudioTags:        true,
    MarkdownEmphasis: false,
    SupportedTags: []string{
        "whispers", "shouting", "laughs", "sighs", "gasp", "giggles",
        "amazed", "crying", "curious", "excited", "mischievously",
        "panicked", "sarcastic", "serious", "tired", "trembling",
        "cough", "excitedly", "bored", "reluctantly",
    },
    PromptGuidance: "Use [tag] inline modifiers in the transcript to control delivery. " +
        "Examples: [whispers], [shouting], [laughs], [sighs], [trembling]. " +
        "Tags can be combined and placed mid-sentence. Use English tags even for non-English text.",
}
```

---

## Voice catalog

30 prebuilt voices, hardcoded in `gemini_tts.go` as `[]ProviderVoice`. Tags drive `AssignVoiceProfile` scoring for auto-assigned NPCs.

| Voice | Description | Tags |
|---|---|---|
| Aoede | Breezy | `narrator, storyteller, breezy, female` |
| Sulafat | Warm | `warm, elder, wise, female` |
| Sadaltager | Knowledgeable | `knowledgeable, sage, scholar, male` |
| Charon | Informative | `informative, guide, neutral` |
| Kore | Firm | `firm, noble, guard, female` |
| Orus | Firm | `firm, authoritative, elder, male` |
| Alnilam | Firm | `firm, soldier, warrior, male` |
| Fenrir | Excitable | `excitable, warrior, fierce, male` |
| Puck | Upbeat | `upbeat, trickster, youthful, male` |
| Laomedeia | Upbeat | `upbeat, bard, lively, female` |
| Sadachbia | Lively | `lively, merchant, cheerful, neutral` |
| Achernar | Soft | `soft, healer, gentle, neutral` |
| Vindemiatrix | Gentle | `gentle, healer, kind, female` |
| Achird | Friendly | `friendly, innkeeper, warm, male` |
| Zubenelgenubi | Casual | `casual, thief, rogue, neutral` |
| Algenib | Gravelly | `gravelly, villain, rough, male` |
| Gacrux | Mature | `mature, elder, gruff, male` |
| Iapetus | Clear | `clear, herald, noble, male` |
| Erinome | Clear | `clear, scholar, precise, female` |
| Rasalgethi | Informative | `informative, sage, neutral` |
| Leda | Youthful | `youthful, servant, young, female` |
| Zephyr | Bright | `bright, traveller, airy, neutral` |
| Autonoe | Bright | `bright, bard, musical, female` |
| Callirrhoe | Easy-going | `easy-going, companion, relaxed, female` |
| Umbriel | Easy-going | `easy-going, rogue, casual, neutral` |
| Algieba | Smooth | `smooth, noble, courtly, neutral` |
| Despina | Smooth | `smooth, courtier, female` |
| Enceladus | Breathy | `breathy, mystic, ethereal, male` |
| Pulcherrima | Forward | `forward, villain, bold, female` |
| Schedar | Even | `even, narrator, steady, neutral` |

Default narrator voice: `"Aoede"` (breezy, storyteller).

---

## Configuration

No new `TTSConfig` fields are required. Existing fields map directly:

| `TTSConfig` field | Gemini TTS usage |
|---|---|
| `Type` | `"gemini"` |
| `BuiltinName` | `"gemini"` (for `type: "builtin"`) |
| `Model` | TTS model ID; defaults to `"gemini-3.1-flash-tts-preview"` |
| `DefaultVoice` | Narrator voice name; defaults to `"Aoede"` |
| `APIKey` | Per-provider key override |

### Credential resolution order

1. `cfg.APIKey` (media.tts.api_key)
2. `providers.gemini.api_key` (shared key, passed from `gui.Service`)
3. `GEMINI_API_KEY` env var
4. `GOOGLE_API_KEY` env var
5. Error: `ErrGeminiTTSAPIKeyRequired`

### `ProviderKey` and `KeyPresent` updates

- `ProviderKey` for `type: "gemini"` TTS → `"gemini:tts"` (avoids colliding with the LLM `"gemini"` key).
- `KeyPresent` updated to return `true` when `builtin_name == "gemini"` and `GEMINI_API_KEY` is set.

---

## Go presets (`pkg/config/presets.go`)

```go
"gemini-3.1-flash-tts": TTSConfig{
    Type:         "gemini",
    Model:        "gemini-3.1-flash-tts-preview",
    DefaultVoice: "Aoede",
    AutoPlay:     false,
    MasterVolume: 1.0,
},
"gemini-2.5-flash-tts": TTSConfig{
    Type:         "gemini",
    Model:        "gemini-2.5-flash-preview-tts",
    DefaultVoice: "Aoede",
    AutoPlay:     false,
    MasterVolume: 1.0,
},
"gemini-2.5-pro-tts": TTSConfig{
    Type:         "gemini",
    Model:        "gemini-2.5-pro-preview-tts",
    DefaultVoice: "Aoede",
    AutoPlay:     false,
    MasterVolume: 1.0,
},
```

---

## Factory wiring (`pkg/media/providers.go`)

`NewTTSClientWithSharedKey` is introduced (matching the image provider pattern) and wired in:

```go
func NewTTSClient(cfg config.TTSConfig) (TTSClient, error) {
    return NewTTSClientWithSharedKey(cfg, "")
}

func NewTTSClientWithSharedKey(cfg config.TTSConfig, sharedKey string) (TTSClient, error) {
    switch cfg.Type {
    // ... existing cases ...
    case "gemini":
        return NewGeminiTTSClient(cfg, sharedKey)
    case "builtin":
        // ...
        if cfg.BuiltinName == "gemini" {
            return NewGeminiTTSClient(cfg, sharedKey)
        }
        // ...
    }
}
```

`gui.Service` passes `cfg.Providers.Gemini.APIKey` as the shared key when building the TTS client (in `GetVoiceCatalog`, `TestProvider("tts")`, and `SynthesiseSegment`).

---

## Frontend

### `TTSConfig` type (`frontend/src/types.ts`)

Add `'gemini'` to the `type` union — no other field changes needed (all existing optional fields cover model, default_voice, api_key).

### `TTS_PRESETS` (`frontend/src/templates/providerPresets.ts`)

Three new entries: `gemini-3.1-flash-tts`, `gemini-2.5-flash-tts`, `gemini-2.5-pro-tts`.

### `SettingsStudio.tsx`

1. Add `<option value="gemini">Google Gemini TTS (Cloud)</option>` to the TTS Provider Type select.
2. Add `<option value="gemini">gemini (Gemini Native Audio)</option>` to the Built-in TTS Engine select.
3. When `type === 'gemini'` or `(type === 'builtin' && builtin_name === 'gemini')`:
   - **Model selector**: text input + 3 preset pills (`gemini-3.1-flash-tts-preview`, `gemini-2.5-flash-preview-tts`, `gemini-2.5-pro-preview-tts`).
   - **Default voice**: text input + 30 quick-select buttons grouped by tone (Calm / Firm / Energetic / Smooth / Other).
   - **API key override**: password input with shared-key indicator (`providers.gemini.api_key` active).
   - Audio tags are surfaced through the existing `SpeechCueCapabilities` panel — no new UI needed.

---

## Error handling

| Condition | Error |
|---|---|
| No API key found | `ErrGeminiTTSAPIKeyRequired` |
| 401/403 / PERMISSION_DENIED | Mapped to human-readable key error |
| 429 / RESOURCE_EXHAUSTED | Rate limit / quota error |
| 404 / NOT_FOUND | Model not found (with model name) |
| Empty response candidates | "gemini tts: no audio data found in response" |
| Other | Wrapped with `fmt.Errorf("gemini tts: synthesis failed: %w", err)` |

---

## Testing strategy

- **Offline unit tests** in `pkg/media/gemini_tts_test.go`:
  - `TestResolveGeminiTTSAPIKey` — env fallback chain.
  - `TestGeminiTTSClientSynthesisReturnsWAV` — mock HTTP server returning a WAV-headed blob via the genai test helper pattern; verifies `AudioExtension` returns `.wav`.
  - `TestGeminiTTSVoiceCatalogHas30Voices` — `ListVoices` returns exactly 30 entries, all with non-empty Tags.
  - `TestGeminiTTSSpeechCueCapabilitiesReportsAudioTags` — `AudioTags: true`, `SupportedTags` non-empty.
- **Factory tests** in `pkg/media/providers_test.go`:
  - `TestNewTTSClientBuildsGemini` — `type: "gemini"` and `builtin_name: "gemini"` both resolve without error when `GEMINI_API_KEY` is set.
- **No live API tests** — all tests are offline/mock.

---

## File map

| Action | File |
|---|---|
| Create | `pkg/media/gemini_tts.go` |
| Create | `pkg/media/gemini_tts_test.go` |
| Modify | `pkg/media/providers.go` — add `NewTTSClientWithSharedKey`, wire `"gemini"` |
| Modify | `pkg/media/providers_test.go` — add `TestNewTTSClientBuildsGemini` |
| Modify | `pkg/media/catalog.go` — update `ProviderKey` for `type: "gemini"` TTS, `KeyPresent` for `builtin_name: "gemini"` |
| Modify | `pkg/config/presets.go` — add 3 TTS presets |
| Modify | `pkg/config/presets_test.go` — verify 3 presets exist with correct model names |
| Modify | `pkg/gui/service.go` — pass `Providers.Gemini.APIKey` to TTS client factory |
| Modify | `frontend/src/types.ts` — add `'gemini'` to TTSConfig type union |
| Modify | `frontend/src/templates/providerPresets.ts` — add 3 TTS presets |
| Modify | `frontend/src/components/SettingsStudio.tsx` — Gemini TTS UI controls |
