# Kokoro-FastAPI Provider Enhancements Design

## Context & Motivation

LocalRPG supports both built-in Sherpa-ONNX Kokoro and HTTP-based TTS providers. When connecting to a local Kokoro-FastAPI instance (typically running on `http://localhost:8880`), the user previously had to:
1. Provide the complete URL `http://localhost:8880/v1/audio/speech` instead of just `http://localhost:8880`.
2. Rely on a static list of 11 hardcoded voice profiles (`af_bella`, `am_adam`, etc.), missing dozens of voices available in Kokoro-FastAPI (including British, French, Japanese, Chinese, Hindi, Italian, Spanish, Portuguese, graded voices, and custom combined voices).
3. Lack dynamic voice enumeration via `/v1/audio/voices`.

Kokoro-FastAPI exposes:
- `GET /v1/audio/voices`: Lists all available voices with metadata (`id`, `name`, `target_quality`, `training_duration`, `overall_grade`).
- `POST /v1/audio/speech`: OpenAI-compatible speech synthesis endpoint accepting `model`, `input`, `voice`, `speed`, `response_format`, `allow_voice_tags`, etc.

## Goals

1. **Flexible Endpoint Configuration:**
   - Allow users to configure the endpoint as `http://localhost:8880` (base URL), while still supporting full URLs like `http://localhost:8880/v1/audio/speech` or `http://localhost:8880/v1` for backwards compatibility.
   - Preserve custom endpoints like AllTalk (`http://localhost:7851/api/tts-generate`).
   - Update default presets and frontend templates to use `http://localhost:8880`.

2. **Dynamic Voice Catalog:**
   - Implement `media.VoiceCatalog` (`ListVoices(ctx context.Context) ([]ProviderVoice, error)`) on `httpTTSClient`.
   - Query `GET /v1/audio/voices` when voice enumeration is requested.
   - Parse Kokoro-FastAPI voice payloads (handling `{ "voices": [...] }` or `[...]`).
   - Parse Kokoro voice naming conventions to enrich each voice with Language (`en-US`, `en-GB`, `ja`, `zh`, etc.), Gender (`female`, `male`), Accent (`American`, `British`, etc.), and search tags (`american`, `female`, `quality:A`, etc.).
   - Integrate with LocalRPG's `CachedVoiceCatalog` so voices are cached to disk under `cache/voices/http-localhost-8880.json` with standard TTL and served immediately to the UI.

3. **Advanced Kokoro-FastAPI Support:**
   - In `httpTTSClient.Synthesize`, when targeting Kokoro, pass `response_format: "mp3"` and `allow_voice_tags: true`.
   - Support voice combination notation (`af_bella(2)+af_sky`) seamlessly via voice ID inputs.

## Architecture & Data Flow

```
+-------------------------------------------------------------+
| Frontend (SettingsStudio / VoiceCombobox)                   |
| - InspectTTS(config)                                        |
+------------------------------+------------------------------+
                               |
                               v
+-------------------------------------------------------------+
| GUI Service (InspectTTS)                                    |
| - Builds TTS client via media.NewTTSClient                  |
| - Checks if client implements media.VoiceCatalog            |
| - Calls CachedVoiceCatalog.Load(providerKey, client)        |
+------------------------------+------------------------------+
                               |
                               v
+-------------------------------------------------------------+
| httpTTSClient.ListVoices(ctx)                               |
| - Resolves base voices URL: <baseURL>/v1/audio/voices       |
| - Fetches JSON, parses voices list                          |
| - Formats ProviderVoice metadata (gender, language, tags)   |
+------------------------------+------------------------------+
                               |
                               v
+-------------------------------------------------------------+
| Kokoro-FastAPI Server (http://localhost:8880)               |
| - GET /v1/audio/voices                                      |
| - POST /v1/audio/speech                                     |
+-------------------------------------------------------------+
```

## URL Resolution Logic

```go
func resolveHTTPEndpoints(endpoint string) (speechURL, voicesURL string)
```

- Normalizes input by trimming whitespace and trailing slashes.
- Checks if endpoint is an AllTalk or custom endpoint (`/api/tts-generate`); if so, leaves `speechURL` as-is and `voicesURL` empty.
- Strips `/v1/audio/speech`, `/v1/audio`, or `/v1` suffix to obtain `baseURL`.
- `speechURL = baseURL + "/v1/audio/speech"`
- `voicesURL = baseURL + "/v1/audio/voices"`

## Voice Metadata Mapping

Kokoro voices use prefix convention `[lang][gender]_[name]`:
- First character:
  - `a`: American English (`en-US`, Accent: American)
  - `b`: British English (`en-GB`, Accent: British)
  - `e`: Spanish (`es`, Accent: Spanish)
  - `f`: French (`fr`, Accent: French)
  - `h`: Hindi (`hi`, Accent: Hindi)
  - `i`: Italian (`it`, Accent: Italian)
  - `j`: Japanese (`ja`, Accent: Japanese)
  - `p`: Brazilian Portuguese (`pt-BR`, Accent: Portuguese)
  - `z`: Mandarin Chinese (`zh`, Accent: Chinese)
- Second character:
  - `f`: Female
  - `m`: Male
- Rest after `_`: Display name capitalization, e.g. `af_heart` -> Name: `Heart (American Female)` or `af_heart`.
- Quality / grades: Added to `Tags` (e.g. `quality:A`, `grade:A`).
