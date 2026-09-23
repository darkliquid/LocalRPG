# Design Spec: ElevenLabs TTS Provider

**Date:** 2026-09-22
**Status:** Draft
**Target:** `pkg/media`, `pkg/config`, `pkg/gui`, `frontend`
**Depends on:** `docs/superpowers/specs/2026-09-22-tts-provider-capabilities-design.md` (capability
interfaces, options model, catalog cache, inspect endpoint)
**Source proposal:** `docs/proposals/2026-09-22-speech-and-llm-provider-expansion.md` (P3)
**Research:** `~/.local/share/crush/research/localrpg-voice-and-llm-providers/report.md`

---

## 1. Executive Summary

ElevenLabs is the first provider whose voices and tunables are not ours. Its REST API is small enough
to implement natively - one `POST` for speech, one paged `GET` for the voice catalog - so this spec
adds a built-in `elevenlabs` client with no new dependency, built on the capability interfaces from the
platform spec.

It covers: configuration and a disabled-by-default preset, the request mapping from
`entity.VoiceConfig` to ElevenLabs fields, the provider option schema that drives the Settings UI, the
catalog mapping into `ProviderVoice`, text handling (Markdown reduction, since ElevenLabs does not
read Markdown), error mapping, and secret hygiene.

---

## 2. Findings

### 2.1 Speech endpoint

`POST https://api.elevenlabs.io/v1/text-to-speech/{voice_id}` (non-streaming) and
`POST .../{voice_id}/stream` (streaming, same body), with header `xi-api-key` and
`Content-Type: application/json`.

Body fields:

| Field | Notes |
| --- | --- |
| `text` | required |
| `model_id` | default `eleven_multilingual_v2` |
| `language_code` | ISO 639-1; **not supported by `multilingual_v2` models** |
| `voice_settings.stability` | default 0.5 |
| `voice_settings.similarity_boost` | default 0.75 |
| `voice_settings.style` | default 0; non-zero adds latency |
| `voice_settings.use_speaker_boost` | default true; adds latency |
| `voice_settings.speed` | default 1 |
| `seed` | 0..4294967295, for reproducibility |
| `previous_text` / `next_text` | continuity (unused here) |
| `apply_text_normalization` | `auto` / `on` / `off` |
| `pronunciation_dictionary_locators` | max 3 (unused here) |

Query parameter `output_format` (`codec_sampleRate_bitrate`), default `mp3_44100_128`:
`mp3_22050_32`, `mp3_44100_128`, `mp3_44100_192` (Creator tier+), `pcm_16000`, `pcm_24000`,
`pcm_44100` (Pro tier+), `ulaw_8000`, `alaw_8000`, `opus_48000_128`, `wav_16000`, ...
([convert](https://elevenlabs.io/docs/api-reference/text-to-speech/convert),
[stream](https://elevenlabs.io/docs/api-reference/text-to-speech/stream))

### 2.2 Voice catalog

`GET https://api.elevenlabs.io/v2/voices` with `search`, `category` (`premade`, `cloned`, `generated`,
`professional`, `famous`, `high_quality`), `voice_type`, `gender`, `age`, `accent`, `language`,
`use_cases`, `fine_tuning_state`, `high_quality`, `voice_ids` (max 100), `page_size` (max 100),
`next_page_token`, `sort`, `sort_direction`.

Response `{voices, has_more, total_count, next_page_token}`. Voice fields: `voice_id`, `name`,
`category`, `labels{accent,age,gender,description,use_case}`, `description`, `preview_url`,
`settings` (same shape as `voice_settings`), `available_for_tiers`, `samples`, `verified_languages`,
`is_owner`, `created_at_unix`.

The legacy `GET /v1/voices` has no filters and "stops working once the workspace exceeds 500 voices",
so **pagination on `/v2/voices` is mandatory**. Labels are free-form strings (`"middle-aged"`,
`"American"`), never enums.
([voices search](https://elevenlabs.io/docs/api-reference/voices/search),
[legacy](https://elevenlabs.io/docs/api-reference/legacy/voices/get-all))

### 2.3 Authentication and cost

`xi-api-key` header on every request; regional hosts exist (`api.us.`, `api.eu.residency.`,
`api.in.`, `api.sg.residency.elevenlabs.io`); keys can be endpoint-scoped, credit-limited, and
IP-allowlisted (403 otherwise). ([authentication](https://elevenlabs.io/docs/api-reference/authentication))

Cost is roughly $0.18/min of generated speech, versus Deepgram Aura as the cheaper alternative.
([voice architecture](https://plexusone.dev/omnivoice-core/voice-architecture/))

### 2.4 LocalRPG has an OpenAI-compatible HTTP client, but it is not this

`httpTTSClient` (`pkg/media/providers.go`) posts `{model, input, voice, speed}` to an OpenAI-shaped
endpoint (or AllTalk's `text_input` shape). ElevenLabs uses a different path, a different body, a
different auth header, and a voice-settings object, so it is a **separate client**, not an `http`
config.

### 2.5 Markdown is not interpreted

ElevenLabs does not read Markdown emphasis. The provider-agnostic `SpeakableText` reduction
(`pkg/media/speakable.go`) already applies to any client that does not implement `MarkdownAware`, so
ElevenLabs automatically receives speakable text. The `markdown` policy in `TTSConfig` remains the
escape hatch.

### 2.6 No pitch control

ElevenLabs exposes `speed` but no pitch. `entity.VoiceConfig.Pitch` therefore cannot be honoured
directly; the schema states the limitation in `Help` rather than silently dropping it.

---

## 3. Design

### 3.1 Configuration and preset

Config (existing `TTSConfig` fields; no new fields required):

```yaml
media:
  tts:
    type: builtin
    builtin_name: elevenlabs
    api_key: ${ELEVENLABS_API_KEY}      # see 3.7
    model: eleven_multilingual_v2
    default_voice: EXAVITQu4vr4xnSDxMaL
    speech_rate: 1.0
    master_volume: 1.0
    auto_play: true
    voice_profiles: [...]
```

Add an opt-in preset, in the Go and frontend preset tables
(`pkg/config/presets.go`, `frontend/src/templates/providerPresets.ts`):

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

The preset carries **no API key and no voice profiles**; consistent with the repository's no-defaults
stance, selecting it is an explicit act and the catalog supplies the voices.

`ProviderKey(cfg)` for this provider is `builtin:elevenlabs`.

### 3.2 The client (`pkg/media/elevenlabs_tts.go`)

```go
type ElevenLabsTTSClient struct {
	apiKey     string
	model      string
	baseURL    string        // default https://api.elevenlabs.io
	outputFmt  string        // default mp3_44100_128
	client     *http.Client
	logger     trace.Logger
}

// Implements:
//   media.TTSClient       Synthesize
//   media.VoiceCatalog    ListVoices
//   media.VoiceOptions    VoiceOptions
//   media.MeteredProvider Metered
```

Fallback for `BuiltinName` in `media.NewTTSClient` (`pkg/media/providers.go:486`) gains a
`case "elevenlabs"`. The constructor validates that `api_key` is non-empty and returns a typed
`ErrMissingAPIKey` rather than failing at synthesis time.

### 3.3 Request mapping

| Source | ElevenLabs |
| --- | --- |
| `voice.VoiceID` | path `{voice_id}` |
| `options["model"]` else `cfg.Model` else `eleven_multilingual_v2` | `model_id` |
| `voice.SpeechRate` (else `cfg.SpeechRate`, else 1.0) | `voice_settings.speed` |
| `voice.Pitch` | *unsupported*; ignored, documented in the `pitch` limitation note |
| `options["stability"]` | `voice_settings.stability` |
| `options["similarity_boost"]` | `voice_settings.similarity_boost` |
| `options["style"]` | `voice_settings.style` |
| `options["use_speaker_boost"]` | `voice_settings.use_speaker_boost` |
| `options["format"]` else `cfg`/default | `output_format` query |
| input text, after `SpeakableTextFor` | `text` |

- `voice_settings` is only sent when at least one key is present, so the voice's stored server-side
  settings apply otherwise. This matters: ElevenLabs persists per-voice defaults, and overriding them
  unintentionally would change how a stock voice sounds.
- `language_code` is **not sent**; it is rejected for `multilingual_v2` models, and auto-detection is
  adequate for a single-language campaign. Documented as a follow-up.
- Response bytes are returned unmodified; the existing `AudioExtension`/`AudioContentType` sniffing
  (`pkg/media/tts.go`) already names and types mp3/pcm output correctly.

### 3.4 Option schema

```go
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
			Help: "Turbo and Flash are faster and cheaper; Multilingual has the widest language support."},
		{Key: "format", Label: "Audio format", Kind: "enum",
			Options: []string{"mp3_44100_128", "mp3_22050_32", "pcm_24000", "ulaw_8000"},
			Default: "mp3_44100_128",
			Help: "Higher bitrates and PCM/WAV may require a paid tier."},
	}
}
```

`Pitch` is deliberately not declared. `Speed` is not declared either: it is the portable baseline
(`speech_rate`), and declaring it twice would put two controls on one knob.

Schema values are validated by `media.ValidateVoiceOptions` (`pkg/media/options.go`), so an
out-of-range `stability` from a hand-edited note is clamped rather than sent.

### 3.5 Catalog mapping

`ListVoices` pages `GET /v2/voices` (`page_size=100`, following `next_page_token` until
`has_more` is false), then maps each voice:

| ElevenLabs | `ProviderVoice` |
| --- | --- |
| `voice_id` | `ID` |
| `name` | `Name` |
| `labels.gender` | `Gender` (lowercased) |
| `labels.accent` | `Accent` |
| `verified_languages[0].language` else `""` | `Language` |
| `labels.age`, `labels.use_case`, `category` | folded into `Tags` via `NormaliseVoiceTags` |
| `category` | `Categories` |
| `description`, `labels.description` | `Description` |
| `preview_url` | `PreviewURL` |
| `settings` | `Defaults` |
| `category`, `available_for_tiers`, `verified_languages` | `Metadata` |

- Account scope is respected: cloned and professional voices appear because they belong to the key
  being used, which is the point of fetching rather than shipping a list.
- Tags are normalised so `labels.age: "middle-aged"` and a hand-authored `middle-aged` tag score
  identically in `harness.AssignVoiceProfile`.
- A voice whose labels are empty still works; matching falls back to the deterministic hash, as it
  does today.
- Paged results are assembled in a single slice and handed to `CachedVoiceCatalog` (platform spec
  §3.5), which owns TTL, staleness, and the on-disk snapshot.

### 3.6 Error mapping and rate limits

| Condition | Behaviour |
| --- | --- |
| Missing `api_key` | `ErrMissingAPIKey` at construction; inspect reports it as a config error |
| 401 | "ElevenLabs rejected the API key" |
| 402 / 429 | "ElevenLabs quota or rate limit reached"; on 429 honour `Retry-After` for one retry, then fail |
| 422 | "Voice or model not available on this account" |
| 5xx | Transient; failure surfaces to the existing per-beat skip so one line cannot silence a turn |
| Network error | Wrapped with the endpoint host, no key |

A failed `ListVoices` never fails the inspect request: the catalog reports `available: true` with
`error` set and `stale: true` when a snapshot exists.

### 3.7 Secrets

- `api_key` is read from `TTSConfig.APIKey`; an empty config value falls back to the
  `ELEVENLABS_API_KEY` environment variable. This lets a user avoid writing a key to disk.
- The key is never logged. `pkg/trace` already has `sanitize.go`; add the key to the redaction set so
  a trace payload containing the request headers cannot leak it.
- The inspect response echoes only a boolean `key_present`, never the key.

### 3.8 Diagnostics

The existing `media.tts.request` trace event gains `provider: "builtin:elevenlabs"` and
`model`, so a trace explains which backend produced a clip. Because the provider is metered,
`Metered()` returns true and the guardrails from the platform spec (§3.9) apply.

---

## 4. Data Flow

```text
Settings: select "ElevenLabs" preset
  └─ POST /api/tts/inspect {config}
       ├─ ElevenLabsTTSClient{apiKey, model}
       ├─ VoiceOptions() ──► 6 controls rendered
       ├─ Metered() ──► badge
       └─ ListVoices() ──► /v2/voices (paged) ──► CachedVoiceCatalog ──► settings UI list

Codex: pick "Sarah", tune stability 0.35, Add as profile
  └─ config.VoiceProfile{provider: builtin:elevenlabs, voice_id: EXAVIT..., options:{stability:0.35}}

Turn: character speaks
  └─ voiceFor -> entity.VoiceConfig
       └─ TTSPipeline.SynthesizeSegment
            ├─ SpeakableTextFor(auto, client, text)      // strips markdown
            ├─ ComputeAudioCacheKeyForVoice(speaker, voice, spoken)
            └─ ElevenLabsTTSClient.Synthesize -> POST /v1/text-to-speech/{id}
                 └─ mp3 bytes -> cache -> playback
```

---

## 5. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| Create | `pkg/media/elevenlabs_tts.go` | Client: Synthesize, ListVoices, VoiceOptions, Metered |
| Create | `pkg/media/elevenlabs_tts_test.go` | Request mapping, option schema, catalog mapping, error mapping, redaction |
| Modify | `pkg/media/providers.go` | `case "elevenlabs"` in `NewTTSClient`; `ErrMissingAPIKey` |
| Modify | `pkg/config/presets.go` | `TTSPresets["elevenlabs"]` |
| Modify | `pkg/config/types.go` | (Only if `Metered` is set in the preset) |
| Modify | `pkg/trace/sanitize.go` | Redact configured API keys |
| Modify | `frontend/src/templates/providerPresets.ts` | `TTS_PRESETS['elevenlabs']` |
| Modify | `frontend/src/components/SettingsStudio.tsx` | "Key present" indicator; env-var hint |

No frontend control is ElevenLabs-specific: the option schema drives the UI.

---

## 6. Acceptance Criteria

1. Selecting the ElevenLabs preset with a valid key synthesises speech for a campaign turn, and the
   clip is served from the content cache on replay.
2. Markdown emphasis in narration is never read aloud (the existing reduction applies).
3. The voice catalog lists the account's voices, is searchable, shows preview URLs, and is cached with
   a 24h TTL and stale-on-error behaviour.
4. The six declared options render in Settings with no provider-specific frontend code; changing
   `stability` changes the audio cache key.
5. `voice_settings` is omitted when a profile carries no options, so a stock voice keeps its
   server-side defaults.
6. A missing or rejected key produces an actionable message at configuration time, not a silent
   failure mid-turn; the key never appears in traces or API responses.
7. The provider reports itself as metered, and bulk synthesis warns first.
8. `go test -count=1 ./...`, `go vet ./...`, and `npx tsc --noEmit` pass.

---

## 7. Out of Scope

- **Streaming** (`/stream`) - needs streaming playback; the whole-beat pipeline does not.
- **Conversational AI, agents, phone/Twilio integration** - a different product surface.
- **Voice cloning and `pronunciation_dictionary_locators`** - clone consent is a content-policy
  question, and dictionaries duplicate what `SpeakableText` already does.
- **`language_code` and multilingual campaigns** - rejected by `multilingual_v2`; a follow-up when a
  model that supports it (or per-segment language) is wanted.
- **`previous_text`/`next_text`/`previous_request_ids` continuity** - worth revisiting if multi-line
  prosody across beats becomes a complaint.
- **SSML** - ElevenLabs does not expose it; the option schema is the supported tuning path.
