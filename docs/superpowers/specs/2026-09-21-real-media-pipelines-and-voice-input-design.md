# System Design Specification: Real Media Pipelines & Voice Input

**Date:** 2026-09-21  
**Status:** Approved  
**Target:** LocalRPG Engine & Desktop GUI (`pkg/media`, `pkg/gui`, `frontend`)

---

## 1. Executive Summary

While LocalRPG ships with procedural and mock built-ins (such as pure-Go vector SVG generation, native OS TTS, and deterministic storytelling) for zero-dependency demos, the core target experience is running **real, high-fidelity local models and media engines** (Faster-Whisper, Whisper.cpp, Kokoro-FastAPI, AllTalk/XTTS, AUTOMATIC1111/Forge WebUI, ComfyUI, and cloud APIs).

This specification defines the production integration for real media pipelines:
1. **Speech-to-Text (STT) & Voice Input**:
   - Implements `httpSTTClient` for OpenAI-compatible Whisper endpoints (`/v1/audio/transcriptions` for local Faster-Whisper, Whisper.cpp server, and OpenAI).
   - Introduces a dedicated `POST /api/stt` backend route for transcribing player audio chunks.
   - Adds native browser **Web Speech API** support alongside `MediaRecorder` audio capture.
   - Wires an interactive click-to-toggle microphone button into the Action Console that populates transcribed text into the prompt input bar for player review.
2. **Real Image Generation Engines**:
   - Upgrades `httpImageClient` with multi-format auto-detection (decoding AUTOMATIC1111/Forge `{"images": [base64]}`, OpenAI `{"data": [{"b64_json": ...}]}`, image URLs, and raw binary streams).
   - Adds a dedicated ComfyUI workflow client that queues a prompt graph to `POST /prompt`, polls `GET /history/{prompt_id}`, and fetches the resulting image from `GET /view`.
3. **Neural Text-to-Speech (TTS) Compatibility**:
   - Enhances `httpTTSClient` to support both OpenAI-standard (`/v1/audio/speech` for Kokoro-FastAPI and OpenAI) and AllTalk (`/api/tts-generate` for Coqui XTTSv2) request/response schemas.

---

## 2. Architecture & Data Flow

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Action Console (Frontend)                         │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │ Input Bar: [ Describe your action...                                ] │  │
│  │ Controls:  [ DO ] [ SAY ] [ STORY ] [ ROLL ]  [ 🎙️ Mic ] [ Submit ]   │  │
│  └───────────────────────────────────┬───────────────────────────────────┘  │
└──────────────────────────────────────┼──────────────────────────────────────┘
                                       │
                    Speech Dictation Path (Dual Mode)
                                       │
        ┌──────────────────────────────┴──────────────────────────────┐
        ▼                                                             ▼
[ Web Speech Mode ]                                         [ Backend STT Mode ]
(window.SpeechRecognition)                                  (MediaRecorder Capture)
  - Zero-latency local streaming                              - Records audio/webm or audio/ogg
  - Transcribes in-browser                                    - POST /api/stt
  - Sets input bar text directly                              - Passes to STTClient (Whisper)
                                                              - Returns {"text": "..."}
                                                              - Sets input bar text
```

---

## 3. Detailed Component Specifications

### 3.1 Speech-to-Text (STT) Backend (`pkg/media/providers.go`)

#### `httpSTTClient`
Implements the `STTClient` interface:
```go
type STTClient interface {
    Transcribe(ctx context.Context, audioData []byte) (string, error)
}
```
- **Payload Generation**:
  - Builds a `multipart/form-data` payload containing:
    - `file`: audio bytes with filename `audio.webm` (or `audio.wav`) and MIME type `audio/webm` (or sniffed MIME).
    - `model`: configured model name (defaults to `"whisper-1"` if empty).
    - `language`: optional language parameter.
  - Sets `Authorization: Bearer <apiKey>` if `apiKey` is non-empty.
- **Endpoint Target**:
  - Configured `endpoint` (e.g. `http://localhost:8000/v1/audio/transcriptions` or `https://api.openai.com/v1/audio/transcriptions`).
- **Response Parsing**:
  - Expects JSON: `{"text": "transcribed speech content"}`.
  - Returns trimmed string.
- **Factory Wiring**:
  - Updated `NewSTTClient(cfg config.STTConfig)` to instantiate `httpSTTClient` when `cfg.Type == "http"`.

### 3.2 Backend API Server (`pkg/gui/server.go`, `pkg/gui/service.go`)

#### `POST /api/stt`
- **Request**:
  - Accepts `multipart/form-data` containing a `file` field, or raw audio body with content-type `audio/webm`, `audio/ogg`, or `audio/wav`.
  - Max body size bounded to 25 MB (`http.MaxBytesReader`).
- **Processing**:
  - Loads configuration via `s.configMgr.Get()`.
  - If `cfg.Media.STT.Type` is disabled or empty, responds with `400 Bad Request` (`"STT engine is disabled or unconfigured"`).
  - Instantiates `media.NewSTTClient(cfg.Media.STT)`.
  - Executes `client.Transcribe(ctx, audioBytes)`.
- **Response**:
  - `200 OK` JSON:
    ```json
    {
      "text": "The transcribed speech text"
    }
    ```

#### Diagnostics Test Enhancement
- In `Service.TestProvider` for `"stt"`:
  - Generates a minimal valid WAV header/sample (using `media.GenerateTone(100*time.Millisecond)` or equivalent) so real Whisper servers receive a decodable audio stream and successfully respond.

### 3.3 Image Generation Engine (`pkg/media/providers.go`, `pkg/media/comfyui.go`)

#### Multi-Format `httpImageClient`
- **Request Formatter**:
  - If `endpoint` contains `/sdapi/v1/txt2img` (AUTOMATIC1111 / Forge WebUI):
    ```json
    {
      "prompt": "<prompt>",
      "negative_prompt": "blurry, low quality, deformed, mutated",
      "steps": 20,
      "width": 512,
      "height": 512
    }
    ```
  - Otherwise (Standard / OpenAI DALL-E / Cloud):
    ```json
    {
      "model": "<model>",
      "prompt": "<prompt>",
      "n": 1,
      "size": "512x512",
      "response_format": "b64_json"
    }
    ```
- **Response Auto-Decoder**:
  - Inspects body bytes:
    1. **Raw Binary**: If header matches PNG (`\x89PNG`), JPEG (`\xff\xd8\xff`), or WebP (`RIFF...WEBP`), returns bytes immediately.
    2. **A1111 / Forge JSON**: If JSON contains `images: []string`, decodes `base64.StdEncoding.DecodeString(images[0])`.
    3. **OpenAI JSON**: If JSON contains `data[0].b64_json`, decodes `base64.StdEncoding.DecodeString(data[0].b64_json)`.
    4. **Image URL JSON**: If JSON contains `data[0].url`, performs an HTTP GET to fetch the image bytes.

#### Dedicated ComfyUI Client (`comfyUIImageClient`)
- Active when `cfg.Type == "comfyui"` or `cfg.Endpoint` targets port 8188:
  1. Submits standard text-to-image workflow prompt graph to `POST /prompt`.
  2. Receives `{"prompt_id": "..."}`.
  3. Polls `GET /history/{prompt_id}` with interval (500ms) up to 60s timeout.
  4. Extracts output image filename from history outputs.
  5. Downloads image bytes via `GET /view?filename={filename}&subfolder={subfolder}&type=output`.

### 3.4 Text-to-Speech (TTS) Engine (`pkg/media/providers.go`)

#### AllTalk / Coqui XTTS Payload Adapter
In `httpTTSClient.Synthesize`:
- If `endpoint` contains `/api/tts-generate` or AllTalk:
  - Formats payload as:
    ```json
    {
      "text_input": "<text>",
      "character_voice_gen": "<voiceID>",
      "narrator_voice_gen": "<voiceID>",
      "text_filtering": "standard",
      "language": "en"
    }
    ```
- Otherwise (Standard OpenAI / Kokoro-FastAPI `/v1/audio/speech`):
  - Formats payload as:
    ```json
    {
      "model": "<model>",
      "input": "<text>",
      "voice": "<voiceID>",
      "speed": <speechRate>
    }
    ```

---

## 4. Frontend UI & Voice Input (`frontend/src/`)

### 4.1 `useVoiceInput` Hook (`frontend/src/hooks/useVoiceInput.ts`)
Encapsulates microphone management:
- **Detection**:
  - Checks if browser supports `window.SpeechRecognition` or `window.webkitSpeechRecognition`.
  - Reads configured `stt.type` from `config` via API client.
- **Modes**:
  - If `stt.type === 'web-speech'` and `SpeechRecognition` available:
    - Creates `SpeechRecognition` instance (`continuous = false`, `interimResults = true`).
    - Updates transcript live; on final result, invokes `onTranscribed(text)`.
  - Otherwise (`stt.type === 'http'` or `'cli'`):
    - Requests `navigator.mediaDevices.getUserMedia({ audio: true })`.
    - Captures chunks with `MediaRecorder` (preferring `audio/webm;codecs=opus`, fallback `audio/ogg`).
    - On stop: sends `Blob` via `APIClient.transcribeAudio(blob)`.
- **States**:
  - `isRecording`: boolean.
  - `isTranscribing`: boolean.
  - `error`: string | null.

### 4.2 Action Console Microphone Control (`ActionConsole.tsx`)
- **Interaction**:
  - Click mic button to start recording.
  - Recording state shows animated pulsing indicator (`bg-red-900/40 text-red-400 border-red-500/50 animate-pulse`).
  - Click again to stop recording.
  - Transcribing state shows loading spinner (`animate-spin text-amber-400`).
  - Upon completion, transcribed string is inserted/appended into the prompt input box for player review and editing.

### 4.3 Settings Studio STT Configuration (`SettingsStudio.tsx`, `providerPresets.ts`)
- Presets list:
  - `Web Speech API (Browser Native)` (`type: 'web-speech'`).
  - `Faster-Whisper (Local HTTP)` (`type: 'http'`, endpoint: `http://localhost:8000/v1/audio/transcriptions`, model: `whisper-1`).
  - `OpenAI Whisper (Cloud API)` (`type: 'http'`, endpoint: `https://api.openai.com/v1/audio/transcriptions`, model: `whisper-1`).
  - `Whisper.cpp (Local CLI)` (`type: 'cli'`).
- UI dropdown includes `Web Speech API (Browser Native)` as a valid provider type.

---

## 5. Error Handling & Edge Cases

1. **Microphone Permission Denied**:
   - `useVoiceInput` catches `NotAllowedError` and sets a user-facing error message without breaking UI state.
2. **Unsupported Audio MIME Type**:
   - `MediaRecorder` checks `MediaRecorder.isTypeSupported` in priority order: `audio/webm;codecs=opus`, `audio/webm`, `audio/ogg`, and bare default.
3. **Empty / Inaudible Audio**:
   - STT server returns empty string or no speech detected; console retains existing text without overwriting.
4. **Image Generation Fallback**:
   - Existing `fallbackImageClient` (`NewSceneImageClient`) automatically falls back to `procedural-art` SVG generator if the primary real model fails or is offline, ensuring turns never crash on art errors.

---

## 6. Testing Strategy

1. **Unit Tests**:
   - `pkg/media/providers_test.go`:
     - Test `httpSTTClient` with mock HTTP server receiving multipart upload and returning JSON transcript.
     - Test `httpImageClient` with A1111 base64 JSON response, OpenAI base64 JSON response, and direct binary image bytes.
     - Test `httpTTSClient` with AllTalk payload format and OpenAI payload format.
   - `pkg/gui/server_test.go`:
     - Test `POST /api/stt` endpoint with valid audio body, disabled STT, and bad input.
2. **Frontend Typechecks & Verification**:
   - TypeScript verification (`npx tsc --noEmit`).
   - Browser verification via Chrome DevTools checking mic toggle states, transcribe API calls, and text population.
