# System Design Specification: Sherpa-ONNX TTS, Oto v3 Playback & Model Download Manager

**Date:** 2026-09-22  
**Status:** Approved  
**Target:** LocalRPG Engine & Desktop GUI (`pkg/media`, `pkg/models`, `pkg/gui`, `frontend`)

---

## 1. Executive Summary

LocalRPG's out-of-the-box experience currently relies on `native-os` TTS (which invokes system speech utilities like `spd-say` / `say` or falls back to a 400ms synthetic sine wave tone) and requires users to manually host external Docker containers or servers for neural speech.

This specification designs the first milestone of the next-generation built-in capabilities:
1. **Sherpa-ONNX In-Process TTS (`pkg/media/sherpa_tts.go`)**:
   - Integrates `github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx` directly into the Go binary.
   - Runs Kokoro-82M offline on CPU, delivering natural voice narration and character dialogue.
   - Maps LocalRPG's authored voice profiles (`af_bella`, `bm_george`, `am_adam`, `bf_emma`, `bm_daniel`) directly to Kokoro speaker style IDs (`sid`).
   - Generates 24kHz 16-bit mono PCM WAV audio in-memory, plugging directly into the media playback pipeline and cache.
2. **Modern Audio Backend Migration (`pkg/media/playback/player.go`)**:
   - Replaces `github.com/darkliquid/mago` (miniaudio bindings) with **`github.com/ebitengine/oto/v3`**.
   - Leverages battle-tested cross-platform audio device management (ALSA/PulseAudio/PipeWire on Linux, CoreAudio on macOS, WASAPI on Windows).
3. **Model Download & Cache Manager (`pkg/models/manager.go`)**:
   - Automatically checks for the presence of required model weights in the user cache directory (`<cacheDir>/models/tts/kokoro/`).
   - Downloads official packaged releases (~86MB) on demand with SHA-256 integrity checks and atomic extraction.
   - Strictly scoped: model checks and prompts **only** trigger when built-in TTS is active (`type: "builtin"`, `builtin_name: "sherpa-onnx"` or `"kokoro"`).
4. **Non-Blocking GUI Confirmation & Progress Streaming**:
   - When a turn runs without the voice model installed, the orchestrator proceeds without blocking, rendering prose and dialogue in the chronicle while emitting a `model_missing` event.
   - The frontend prompts the player with a clean dialog offering to download the voice pack (~86MB).
   - Real-time download progress streams over a Server-Sent Events (SSE) endpoint (`GET /api/models/events`).
   - If declined, the game proceeds in silence, and the model can be downloaded later via settings or turn replay.

---

## 2. Architecture & Component Data Flow

```mermaid
flowchart TD
    subgraph Frontend
        Console[Action Console / Turn Request]
        Chronicle[Chronicle View]
        PromptModal[Download Prompt Modal]
        ProgBar[Download Progress Bar]
    end

    subgraph Backend Engine
        TurnAPI[POST /api/game/{id}/turn]
        Orchestrator[Turn Orchestrator]
        TTSClient[SherpaTTSClient]
        ModelsMgr[Models Manager]
        OtoPlayer[Oto v3 Playback Player]
    end

    subgraph Model Cache
        KokoroFiles[cacheDir/models/tts/kokoro/]
    end

    Console -->|Submit Turn| TurnAPI
    TurnAPI --> Orchestrator
    Orchestrator -->|Synthesize Voice| TTSClient
    
    TTSClient -->|Check Weights| KokoroFiles
    KokoroFiles -.->|Missing| TTSClient
    TTSClient -->|ErrModelNotLoaded| Orchestrator
    
    Orchestrator -->|Stream Turn Prose & model_missing Event| Chronicle
    Chronicle -->|Trigger Prompt| PromptModal
    
    PromptModal -->|User Confirms Download| ModelsMgr
    ModelsMgr -->|Fetch & Verify SHA256| KokoroFiles
    ModelsMgr -->|Stream SSE Progress| ProgBar
    
    KokoroFiles -.->|Installed| TTSClient
    TTSClient -->|Generate 24kHz PCM WAV| OtoPlayer
    OtoPlayer -->|Device Audio| Speakers[Audio Output Device]
```

---

## 3. Audio Device & Playback Backend (`pkg/media/playback/player.go`)

### 3.1 Migration from `mago` to `ebitengine/oto/v3`
The miniaudio Go wrapper (`darkliquid/mago`) is replaced by `github.com/ebitengine/oto/v3`.

* **Initialization:**
  ```go
  type Player struct {
      otoCtx   *oto.Context
      otoReady chan struct{}
      mu       sync.Mutex
      player   *oto.Player
      streamer beep.Streamer
      closers  []io.Closer
      gain     float64
      playing  bool
      logger   trace.Logger
  }
  ```
  `Open(volume float64)` initializes `oto.NewContext(&oto.NewContextOptions{SampleRate: 48000, ChannelCount: 2, Format: oto.FormatSignedInt16LE})`.
* **Headless Graceful Degradation:**
  If the host lacks audio hardware or drivers, `oto.NewContext` fails; `Open` returns `ErrUnavailable`. The application logs a trace event and suppresses playback without crashing or interrupting turns.
* **Streaming Bridge:**
  A lightweight reader converts `beep.Streamer` float samples into little-endian 16-bit PCM bytes on the fly, feeding `oto.Player`.
* **Queue Interruption:**
  Starting a new queue replaces the active `oto.Player`, immediately stopping prior narration when a new turn is submitted or manual replay is requested.

---

## 4. Sherpa-ONNX In-Process TTS Provider (`pkg/media/sherpa_tts.go`)

### 4.1 Interface Implementation
Implements `media.TTSClient`:
```go
type SherpaTTSClient struct {
    modelDir string
    tts      *sherpa_onnx.OfflineTts
    mu       sync.Mutex
    logger   trace.Logger
}
```

### 4.2 Voice ID Mapping
Translates authored entity `VoiceConfig.VoiceID` strings to numeric Kokoro speaker IDs:
```go
var kokoroSpeakerMap = map[string]int{
    "af_alloy": 0, "af_aoede": 1, "af_bella": 2, "af_heart": 3,
    "af_jessica": 4, "af_kore": 5, "af_nicole": 6, "af_nova": 7,
    "af_river": 8, "af_sarah": 9, "af_sky": 10,
    "am_adam": 11, "am_echo": 12, "am_eric": 13, "am_fenrir": 14,
    "am_liam": 15, "am_michael": 16, "am_onyx": 17, "am_puck": 18,
    "bf_alice": 19, "bf_emma": 20, "bf_isabella": 21, "bf_lily": 22,
    "bm_daniel": 23, "bm_fable": 24, "bm_george": 25, "bm_lewis": 26,
}
```
* Unmapped voices fallback to `0` (`af_alloy`) or the configured default.
* `voice.SpeechRate` is passed directly as the synthesis length/speed modifier.

### 4.3 PCM WAV Generation
* Calls `audio := tts.Generate(text, sid, speed)`.
* Takes `audio.Samples` (`[]float32` at 24kHz) and serializes them into standard 16-bit mono RIFF WAV byte format with header:
  - Chunk ID: `RIFF`
  - Audio Format: `1` (PCM)
  - Num Channels: `1`
  - Sample Rate: `24000`
  - Bits Per Sample: `16`
* Returns `[]byte, nil`.

---

## 5. Model Download & Cache Manager (`pkg/models/manager.go`)

### 5.1 Storage Directory Layout
Resolved through `core.PathResolver.CacheDir()`:
```
<cacheDir>/models/
└── tts/
    └── kokoro/
        ├── model.onnx
        ├── voices.bin
        ├── tokens.txt
        ├── espeak-ng-data/
        └── manifest.json
```

### 5.2 Model Metadata & Pinned Checksums
```go
type ModelSpec struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    URL         string `json:"url"`
    ArchiveType string `json:"archive_type"` // "tar.bz2"
    SHA256      string `json:"sha256"`
    SizeBytes   int64  `json:"size_bytes"`
    TargetDir   string `json:"target_dir"`
}
```
Kokoro release weights package (~86MB) has a pinned SHA-256 hash.

### 5.3 Download Lifecycle & Safety
1. **Guard:** Only triggers when called via the download API.
2. **Temporary File:** Writes to `<cacheDir>/models/tmp/<id>.part`.
3. **Verification:** Validates SHA-256 before unpacking. On mismatch, deletes the download and sets error state.
4. **Atomic Extraction:** Extracts archive into `<cacheDir>/models/tmp/<id>_extracted/`, then renames atomically to `<cacheDir>/models/tts/kokoro/`.
5. **Cancellation:** Respects `context.Context`, pruning partial files on abort.

---

## 6. API Surface & GUI Experience

### 6.1 Backend Endpoints (`pkg/gui/server.go`)
* `GET /api/models`: Returns list of models and status (`id`, `name`, `installed`, `downloading`, `progress`, `total_bytes`, `error`).
* `POST /api/models/{id}/download`: Initiates background download.
* `GET /api/models/events`: SSE stream emitting `model_status` events whenever progress changes.

### 6.2 Turn Event Streaming & Provider Guard
* In `pkg/gui/service.go` (`PlayTurnStream`):
  * Checks if TTS is enabled and configured to `builtin` (`sherpa-onnx` or `kokoro`).
  * If the model files are missing, emits chunk:
    ```json
    {"type": "model_missing", "model_id": "kokoro-tts", "name": "Kokoro Voice Pack", "size_bytes": 90177536}
    ```
  * Turn narration continues without error; dialogue and prose display normally.

### 6.3 Frontend Prompt & Download Flow
* `TurnStream` handler detects `model_missing`.
* If not previously dismissed during the session, shows a confirmation dialog:
  > **🎙️ Enable Voice Narration**  
  > High-quality voice narration requires the Kokoro voice model (~86 MB). Would you like to download it now?  
  > `[ Download Voice Pack (86 MB) ]` &nbsp; `[ Continue in Silence ]`
* Clicking **Download** connects to `/api/models/events`, shows real-time percentage/bytes progress, and displays a success notification upon completion.
* Subsequent turns automatically synthesize and play voice acting.

---

## 7. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| **Create** | `pkg/media/sherpa_tts.go` | Sherpa-ONNX TTS client implementing `TTSClient` and voice mapping |
| **Create** | `pkg/media/sherpa_tts_test.go` | Tests for voice translation, WAV serialization, and missing model fallback |
| **Create** | `pkg/models/manager.go` | Model download manager, SHA-256 verification, and atomic unpacking |
| **Create** | `pkg/models/manager_test.go` | Tests for download streaming, checksum validation, and cancellation |
| **Modify** | `pkg/media/playback/player.go` | Replace `mago` with `ebitengine/oto/v3` |
| **Modify** | `pkg/media/playback/player_test.go` | Update audio player tests for Oto v3 |
| **Modify** | `pkg/media/providers.go` | Wire `sherpa-onnx` built-in factory in `NewTTSClient` |
| **Modify** | `pkg/config/types.go` & `presets.go` | Add `sherpa-onnx` TTS preset and config definitions |
| **Modify** | `pkg/gui/server.go` & `service.go` | Add `/api/models` endpoints and `model_missing` turn event chunk |
| **Modify** | `frontend/src/types.ts` & `api/client.ts`| Add model status types and client methods |
| **Modify** | `frontend/src/components/turn/` | Add model download prompt dialog and progress bar |
| **Modify** | `go.mod` & `go.sum` | Add `sherpa-onnx-go` and `oto/v3`, remove `mago` |

---

## 8. Verification & Acceptance Criteria

1. **Compilation & Linting:**
   * `mise run lint` (`go vet ./...`) passes cleanly with no errors.
   * `mise run test:frontend` (`npx tsc --noEmit`) passes with zero unused variables or missing types.
2. **Audio Backend Verification:**
   * `pkg/media/playback/player_test.go` passes cleanly without requiring real audio hardware in CI/test.
3. **Model Download & Integrity Verification:**
   * Test with mock HTTP server demonstrates progress reporting, SHA-256 verification, and atomic extract.
   * Tampered test archives are rejected and deleted without corrupting state.
4. **End-to-End Turn Flow:**
   * First turn with missing model emits `model_missing`, renders prose silently, and triggers GUI prompt.
   * Completing download initializes Sherpa-ONNX; subsequent turns synthesize speech and output audio through the audio player.
