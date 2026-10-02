# Design Spec: Inworld AI Provider Ecosystem (LLM Router, TTS, STT)

**Date:** 2026-10-02  
**Status:** Approved  
**Target:** `pkg/config`, `pkg/provider`, `pkg/harness`, `pkg/media`, `pkg/gui`, `frontend`  
**Scope:** Unified Inworld AI integration across LLM routing, Text-to-Speech, and Speech-to-Text with shared credentials and offline test coverage.

---

## 1. Executive Summary

This specification defines the integration of Inworld AI as a first-class provider in LocalRPG. Inworld provides three distinct AI capabilities:
1. **LLM Router**: An OpenAI-compatible chat completions gateway at `https://api.inworld.ai/v1/chat/completions` providing intelligent routing across frontier models (defaulting to `inworld/compare-frontier-models`) and custom router configurations.
2. **Text-to-Speech (TTS)**: High-quality neural speech synthesis via `POST https://api.inworld.ai/tts/v1/voice` using the `inworld-tts-2` model and voices like `Ashley` and `Dennis`, with dynamic voice catalog discovery and offline fallbacks.
3. **Speech-to-Text (STT)**: Cloud audio transcription via `POST https://api.inworld.ai/stt/v1/transcribe` using the `inworld/inworld-stt-1` model, accepting base64-encoded audio in formats such as `LINEAR16`, `OGG_OPUS`, and `MP3`.

All three capabilities share a unified credential architecture (`providers.inworld.api_key`, `INWORLD_API_KEY`, or per-service overrides) and authenticate with HTTP `Authorization: Basic <base64-key>` headers.

Realtime duplex speech-to-speech WebSockets are explicitly excluded from this design to preserve LocalRPG's turn-based orchestration model.

---

## 2. Architecture & Configuration

### 2.1 Credential Resolution & Precedence

A user commonly configures Inworld once to use it across Game Master narration, voice synthesis, and microphone transcription. A top-level `providers.inworld` configuration block is introduced.

For any Inworld operation, authorization credentials resolve with the following strict priority:
1. **Explicit Role or Media Key**:
   - For LLM: `agents.roles.<role>.api_key`
   - For TTS: `media.tts.api_key`
   - For STT: `media.stt.api_key`
2. **Shared Provider Key**: `providers.inworld.api_key`
3. **Environment Variable**: `INWORLD_API_KEY`
4. **Missing Key**: Returns an actionable error:
   `"inworld: an API key is required; set providers.inworld.api_key, agents.roles.<role>.api_key (or media.tts/stt.api_key), or INWORLD_API_KEY"`

### 2.2 Wire Authentication & Secret Redaction

- Inworld APIs expect `Authorization: Basic <base64-key>`. The key provided by the Inworld Portal or CLI is already base64-encoded and is passed directly in the header.
- During provider instantiation, any resolved API key is registered with `trace.RegisterSecret(apiKey)` so it is automatically redacted from traces, log sinks, and error messages.

### 2.3 Configuration Schema

#### `pkg/config/types.go`
```go
// ProvidersConfig groups shared credentials and defaults for external ecosystem providers.
type ProvidersConfig struct {
    Gemini   GeminiProviderConfig  `yaml:"gemini,omitempty" json:"gemini,omitempty"`
    Inworld  InworldProviderConfig `yaml:"inworld,omitempty" json:"inworld,omitempty"`
    Currency string                `yaml:"currency,omitempty" json:"currency,omitempty"`
    Prices   []PriceConfig         `yaml:"prices,omitempty" json:"prices,omitempty"`
}

type InworldProviderConfig struct {
    APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
}
```

#### `frontend/src/types.ts`
```typescript
export interface InworldProviderConfig {
  api_key?: string;
}

export interface ProvidersConfig {
  gemini?: GeminiProviderConfig;
  inworld?: InworldProviderConfig;
  currency?: string;
  prices?: PriceConfig[];
}
```

### 2.4 Canonical Provider Keys (`pkg/provider/keys.go`)
Three canonical adapter keys are registered:
```go
const (
    KeyLLMInworld Key = "llm:inworld"
    KeyTTSInworld Key = "tts:inworld"
    KeySTTInworld Key = "stt:inworld"
)
```
And added to `AllKeys()`.

---

## 3. LLM Router Provider (`pkg/provider/inworldllm`)

### 3.1 Descriptor & Preset
Registered in `pkg/provider/inworldllm/inworldllm.go`:
- **ID**: `llm:inworld`
- **Family**: `FamilyLLM`
- **Label**: `"Inworld LLM Router"`
- **Description**: `"Gateway routing prompts across frontier models with automatic fallbacks and tool calling."`
- **Source**: `"http"`
- **Features**: `FeatureStreaming`, `FeatureTools`, `FeatureKeyRequired`, `FeatureMetered`
- **Presets**:
  - `ID`: `"inworld-frontier"`
  - `Label`: `"Inworld Frontier Router"`
  - `Order`: 5
  - `Description`: `"Routes prompts across frontier LLMs with automatic fallbacks."`
  - `Config`:
    ```json
    {
      "type": "builtin",
      "builtin_name": "inworld",
      "model": "inworld/compare-frontier-models",
      "temperature": 0.7,
      "max_tokens": 2048
    }
    ```

### 3.2 Client Implementation (`pkg/provider/inworldllm/client.go`)
- **Endpoint**: Defaults to `https://api.inworld.ai/v1/chat/completions` (or `cfg.Endpoint` if a custom proxy/endpoint is configured).
- **Authentication**: `Authorization: Basic <apiKey>`.
- **Protocol**:
  - Implements `harness.ModelProvider`, `harness.ToolCaller`, `trace.LogAware`, and `media.MeteredProvider`.
  - Maps `harness.GenerateRequest` (`System`, `Messages`, `Tools`, `Temperature`, `MaxTokens`) to the standard OpenAI-compatible Chat Completions payload.
  - Streaming parses SSE (`data: {...}\n\n`), reconstructing delta text and tool call frames (`delta.tool_calls`) across chunks.
  - Tracks token usage (`PromptTokens`, `CompletionTokens`) from the final stream frame or non-streaming response body.

### 3.3 Engine Wiring
- In `pkg/harness/exports.go`:
  - `KeyFor` maps `type: "inworld"` or `builtin_name: "inworld"` to `provider.KeyLLMInworld`.
- In `pkg/harness/factory.go`:
  - Passes `cfg.Providers.Inworld.APIKey` as `SharedKey` in `ModelBuildPayload`.

---

## 4. TTS Provider & Voice Catalog (`pkg/provider/inworldtts`)

### 4.1 Descriptor & Preset
Registered in `pkg/provider/inworldtts/inworldtts.go`:
- **ID**: `tts:inworld`
- **Family**: `FamilyTTS`
- **Label**: `"Inworld TTS (Cloud, metered)"`
- **Description**: `"Natural-sounding dialogue and narration with inworld-tts-2."`
- **Source**: `"http"`
- **Features**: `FeatureMetered`, `FeatureKeyRequired`, `FeatureVoiceCatalog`
- **Presets**:
  - `ID`: `"inworld-tts"`
  - `Label`: `"Inworld TTS"`
  - `Order`: 8
  - `Description`: `"Inworld Cloud TTS using inworld-tts-2. Set key in Providers tab or via INWORLD_API_KEY."`
  - `Config`:
    ```json
    {
      "type": "builtin",
      "builtin_name": "inworld",
      "model": "inworld-tts-2",
      "default_voice": "Ashley",
      "pitch": 1.0,
      "speech_rate": 1.0,
      "auto_play": true,
      "master_volume": 1.0
    }
    ```

### 4.2 Synthesis Implementation (`pkg/provider/inworldtts/client.go`)
- **Endpoint**: `POST https://api.inworld.ai/tts/v1/voice`
- **Headers**:
  - `Authorization: Basic <apiKey>`
  - `Content-Type: application/json`
- **Request Body**:
  ```json
  {
    "text": "<text>",
    "voice_id": "<voice-id>",
    "model_id": "inworld-tts-2",
    "audio_config": {
      "audio_encoding": "MP3",
      "sample_rate_hertz": 48000
    }
  }
  ```
- **Voice Selection**: Resolves `voice.VoiceID` from the entity voice config, falling back to `cfg.DefaultVoice` or `"Ashley"`.
- **Audio Output**: Decodes base64 string from `{ "audioContent": "<base64>" }` and returns raw MP3 bytes. LocalRPG's `media.DecodeProviderAudio` automatically decodes the MP3 stream into PCM samples and normalizes it to Ogg/Opus in `cache/`.
- **Metered Tracking**: Records characters synthesized in `LastUsage()`.

### 4.3 Voice Catalog (`media.VoiceCatalog`)
- Queries Inworld voices API when credentials are present.
- Provides fallback to built-in curated voices (`Ashley` [Female/Natural], `Dennis` [Male/Conversational], etc.) when offline or unauthenticated, allowing character creation and voice previewing to work reliably.

### 4.4 Engine Wiring
- In `pkg/media/exports.go`:
  - `TTSKeyFor` maps `type: "inworld"` or `builtin_name: "inworld"` to `provider.KeyTTSInworld`.
- In `pkg/gui/service.go`:
  - Passes `cfg.Providers.Inworld.APIKey` as `sharedKey` to `media.NewTTSClientWithSharedKey`.

---

## 5. STT Provider (`pkg/provider/inworldstt`)

### 5.1 Descriptor & Preset
Registered in `pkg/provider/inworldstt/inworldstt.go`:
- **ID**: `stt:inworld`
- **Family**: `FamilySTT`
- **Label**: `"Inworld STT (Cloud, metered)"`
- **Description**: `"Cloud speech recognition with voice profiling using inworld/inworld-stt-1."`
- **Source**: `"http"`
- **Features**: `FeatureMetered`, `FeatureKeyRequired`
- **Presets**:
  - `ID`: `"inworld-stt"`
  - `Label`: `"Inworld STT"`
  - `Order`: 5
  - `Description`: `"Cloud transcription via Inworld STT. Set key in Providers tab or via INWORLD_API_KEY."`
  - `Config`:
    ```json
    {
      "type": "builtin",
      "builtin_name": "inworld",
      "model": "inworld/inworld-stt-1"
    }
    ```

### 5.2 Transcription Implementation (`pkg/provider/inworldstt/client.go`)
- **Endpoint**: `POST https://api.inworld.ai/stt/v1/transcribe`
- **Headers**:
  - `Authorization: Basic <apiKey>`
  - `Content-Type: application/json`
- **Audio Encoding Adaptation**:
  - Detects incoming audio container:
    - Ogg/Opus (`OggS` prefix): sets `audio_encoding: "OGG_OPUS"`.
    - MP3 (`ID3` or sync word): sets `audio_encoding: "MP3"`.
    - WAV (`RIFF` prefix): decodes PCM via `decodeWAV` and sets `audio_encoding: "LINEAR16"` with raw PCM bytes.
    - Default: encodes as `LINEAR16`.
- **Request Body**:
  ```json
  {
    "transcribe_config": {
      "model_id": "inworld/inworld-stt-1",
      "language": "en-US",
      "audio_encoding": "<encoding>"
    },
    "audio_data": {
      "content": "<base64-audio>"
    }
  }
  ```
- **Response Processing**: Extracts and returns `transcription.transcript`.

### 5.3 Shared Key Parity & Wiring
- Introduce `media.NewSTTClientWithSharedKey(cfg, sharedKey)` and `media.STTBuildPayload` in `pkg/media/` to establish shared key parity across all media providers (TTS, Image, STT).
- In `pkg/media/exports.go`:
  - `STTKeyFor` maps `type: "inworld"` or `builtin_name: "inworld"` to `provider.KeySTTInworld`.
- In `pkg/gui/service.go`:
  - Passes `cfg.Providers.Inworld.APIKey` into `NewSTTClientWithSharedKey`.

---

## 6. Frontend UI & Embedded Documentation

### 6.1 Settings Studio (`frontend/src/components/SettingsStudio.tsx`)
- **Providers Tab**:
  - Add Inworld AI credential card with status badge (`Custom Key Saved` vs `Optional if INWORLD_API_KEY is set`).
  - Help text linking to Inworld Portal (`https://platform.inworld.ai/api-keys`) and Inworld CLI instructions (`inworld auth login`).
  - Input field bound to `config.providers.inworld.api_key`.
- **LLM, TTS, and STT Tabs**:
  - Automatically surface Inworld presets via `useProviderCatalog`.
  - Render `"Using shared key from Providers tab (leave blank)"` placeholder when a shared key is present.
  - "Test Connection" buttons trigger `Service.TestProvider`.

### 6.2 Embedded Documentation
- Run `go test ./pkg/gui -update-docs` to regenerate:
  - Provider catalogue documentation articles (`pkg/gui/docs/providers.md`).
  - Config reference documentation.

---

## 7. Error Handling & Redaction

### 7.1 Status Code Mapping
API errors are converted into clean, actionable error messages:
- **401 / 403 Unauthorized**: `"inworld: invalid API key or permission denied; check providers.inworld.api_key or INWORLD_API_KEY"`
- **429 Rate Limit**: `"inworld: quota exceeded or rate limit reached; check your Inworld account credits"`
- **400 / 404 Bad Request / Not Found**: `"inworld: %s"`
- Missing Key: `"inworld: an API key is required; set providers.inworld.api_key, agents.roles.<role>.api_key (or media.tts/stt.api_key), or INWORLD_API_KEY"`

### 7.2 Credential Redaction
All API keys are registered with `trace.RegisterSecret` on client creation to prevent accidental leakage in telemetry or log exports.

---

## 8. Testing Strategy

All automated tests execute offline without requiring internet access or live credentials:

1. **Inworld LLM Tests (`pkg/provider/inworldllm/client_test.go`)**:
   - `httptest.Server` simulating streaming SSE chunks and tool call frames.
   - Verify `Authorization: Basic <key>` header formatting.
   - Verify credential resolution hierarchy (Role > Shared > Env > Missing error).
2. **Inworld TTS Tests (`pkg/provider/inworldtts/client_test.go`)**:
   - `httptest.Server` simulating `POST /tts/v1/voice` with base64 MP3 payload.
   - Verify audio output decoding to PCM.
   - Verify `ListVoices` parsing and offline fallback behavior.
3. **Inworld STT Tests (`pkg/provider/inworldstt/client_test.go`)**:
   - `httptest.Server` simulating `POST /stt/v1/transcribe`.
   - Verify audio format conversion (WAV to LINEAR16, OGG_OPUS).
   - Verify transcript extraction and usage recording.
4. **Registry & Architecture Validation**:
   - `pkg/provider/all/`: Blank import of new packages and validation with `provider.Validate()`.
   - `pkg/gui/docs_catalogue_test.go`: Verify descriptor presets and docs generation.
5. **Quality Gates**:
   - `mise run test:backend` (`go test -v -count=1 ./...`)
   - `mise run lint` (`go vet ./...`)
   - `mise run test:frontend` (`npx tsc --noEmit`)
