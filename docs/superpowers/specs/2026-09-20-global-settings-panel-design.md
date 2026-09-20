# Global Settings Panel & Configuration Architecture Specification

- **Date:** 2026-09-20
- **Status:** Approved
- **Scope:** Global configuration system, backend dynamic re-binding, media/agent provider framework, and dual-access frontend Settings Studio & slide-over drawer.

---

## 1. Overview & Goals

LocalRPG requires a unified global settings system that governs app-wide behavior independent of individual games, worlds, or rule systems. 

Key capabilities:
1. **Configurable Paths**: Customize storage directories for systems, worlds, games, and media cache.
2. **AI Agent & Role Routing**: Configure LLM providers and role assignments (`gm`, `narrator`, `evaluator`) across `builtin`, `http`, `cli`, and `disabled` types.
3. **Media Engines**: Configure Text-To-Speech (TTS), Speech-To-Text (STT), and Image Generation providers, with support for local binaries, OpenAI-compatible HTTP endpoints, and future built-in WASM modules (e.g. Kokoro WASM).
4. **Interactive Diagnostics**: Provide a "Test Connection / Test Engine" facility in the UI that validates credentials, endpoints, or local binaries before playing.
5. **App Preferences**: Audio master volume, auto-play narration, auto-generate scene art, token streaming, and cinematic visual styling (vignette/noise overlays, font scaling).
6. **Dual-Access UI**: Accessible as a top-level tab in LauncherHub, and as an in-game slide-over drawer from the active session header.

---

## 2. Configuration Storage & Resolution Order

Configuration is managed by a new package `pkg/config` using a hierarchical resolution order:

1. **Local Override**: `./localrpg.yaml` in the current working directory (takes precedence if present).
2. **User Global Config**: `~/.config/localrpg/config.yaml` (default location for user-level persistence).
3. **Internal Defaults**: Hardcoded safe defaults if neither file exists.

### Save Semantics:
- If `./localrpg.yaml` exists, GUI saves update `./localrpg.yaml`.
- Otherwise, saves persist to `~/.config/localrpg/config.yaml` (creating `~/.config/localrpg/` if missing).
- The REST API reports the active file path and whether a local workspace override is in effect.

---

## 3. Configuration Data Model

```yaml
version: "1"

# Storage and discovery paths
paths:
  systems: "./systems"
  worlds: "./worlds"
  games: "./games"
  cache: "./cache"

# AI Agents & Role Routing
agents:
  default_role: "gm"
  roles:
    gm:
      type: "cli"             # "builtin" | "http" | "cli" | "disabled"
      builtin_name: ""        # Used when type == "builtin"
      command: "echo"         # Used when type == "cli"
      args: []
      endpoint: ""            # Used when type == "http"
      model: ""
      api_key: ""
      temperature: 0.7
      max_tokens: 1024
    narrator:
      type: "disabled"
    evaluator:
      type: "disabled"
  fallbacks:
    gm: ""
    narrator: ""

# Media Engines
media:
  tts:
    type: "disabled"          # "builtin" | "http" | "cli" | "disabled"
    builtin_name: ""          # e.g. "kokoro-wasm"
    command: ""               # e.g. "piper"
    args: []
    endpoint: ""              # e.g. "http://localhost:8880/v1/audio/speech"
    model: ""
    api_key: ""
    default_voice: "default"
    pitch: 1.0
    speech_rate: 1.0
    auto_play: false
    master_volume: 1.0        # 0.0 to 1.0

  stt:
    type: "disabled"          # "builtin" | "http" | "cli" | "disabled"
    builtin_name: ""
    command: ""               # e.g. "whisper-cli"
    args: []
    endpoint: ""              # e.g. "http://localhost:8080/v1/audio/transcriptions"
    model: ""
    api_key: ""

  image:
    type: "disabled"          # "builtin" | "http" | "cli" | "disabled"
    builtin_name: ""
    command: ""
    args: []
    endpoint: ""              # e.g. "http://localhost:7860/v1/images/generations"
    model: ""
    api_key: ""
    auto_generate: false

# App Preferences & UI Styling
preferences:
  streaming: true
  typing_speed_ms: 15
  cinematic_effects: true     # Vignette & CRT/noise backdrop overlays
  font_scale: "medium"        # "small" | "medium" | "large"
```

---

## 4. Backend Engine Architecture & Provider Abstraction

### 4.1 Provider Types
LocalRPG standardizes on 4 provider types across LLM, TTS, STT, and Image generation:

1. **`builtin`**: Dispatches to in-process implementations registered in a `BuiltinRegistry`. Enables future embedding of WASM runtimes (e.g. Kokoro WASM for client/server offline TTS) or lightweight internal mock/echo engines without config changes.
2. **`http`**: Standard HTTP REST client adhering to OpenAI-compatible specs:
   - LLM: `/v1/chat/completions` (streaming SSE support)
   - TTS: `/v1/audio/speech`
   - STT: `/v1/audio/transcriptions`
   - Image: `/v1/images/generations`
3. **`cli`**: Local process execution with timeout, capturing stdout or writing to temporary files.
4. **`disabled`**: Typed no-op client returning `ErrProviderDisabled`, gracefully bypassing generation without error spam.

### 4.2 Dynamic Re-binding (Hot Swapping)
The backend `gui.Service` holds a live reference to `*config.ConfigManager` and a mutex protecting runtime references:
- When a `PUT /api/settings` request is accepted:
  1. Validate config structure and fields.
  2. Save to active config file on disk.
  3. Reconfigure `core.PathResolver` with updated directory paths.
  4. Ensure target directories exist (`os.MkdirAll`).
  5. Re-initialize and swap `harness.Router` with new role mappings.
  6. Re-initialize and swap `media.TTSPipeline`, `media.STTProvider`, and `media.ImagePipeline`.
  7. Subsequent queries to `/api/systems`, `/api/worlds`, `/api/games`, or player turns immediately reflect updated paths and providers.

---

## 5. REST API Endpoints

### 5.1 `GET /api/settings`
Returns active configuration and file metadata.
- **Response**:
  ```json
  {
    "config": { ... },
    "config_file_path": "/home/user/.config/localrpg/config.yaml",
    "is_local_override": false
  }
  ```

### 5.2 `PUT /api/settings`
Updates and persists settings, triggering dynamic service re-binding.
- **Request**: `{ "config": { ... } }`
- **Response**: `{ "config": { ... }, "status": "applied" }`

### 5.3 `POST /api/settings/test-provider`
Validates provider connectivity and measures round-trip latency without saving to disk.
- **Request**:
  ```json
  {
    "category": "llm", // "llm" | "tts" | "stt" | "image"
    "provider": {
      "type": "http",
      "endpoint": "http://localhost:11434/v1",
      "model": "llama3.2",
      "api_key": ""
    },
    "test_prompt": "Ping"
  }
  ```
- **Response**:
  ```json
  {
    "success": true,
    "latency_ms": 118,
    "message": "Connected successfully",
    "preview": "Pong"
  }
  ```

---

## 6. Frontend UI Design

### 6.1 Access Points
1. **LauncherHub Top Tab**: A **Settings** navigation tab alongside `Campaigns`, `Rule Systems`, and `Worlds Studio`.
2. **In-Game Header Gear Icon**: A `<Settings className="w-3.5 h-3.5" />` button in `App.tsx` acrylic header that opens a `settings` drawer, enabling audio volume, theme, and engine tweaking mid-campaign.

### 6.2 Settings Studio Interface (`frontend/src/components/SettingsStudio.tsx`)
Four organized tabs:
1. **Paths & Storage**:
   - Inputs for Systems, Worlds, Games, and Cache directory paths.
   - Status badge indicating active config file and override state.
   - "Save & Re-index" button.
2. **AI Agents & Roles**:
   - Role selector (`gm`, `narrator`, `evaluator`).
   - Provider Type dropdown (`builtin`, `http`, `cli`, `disabled`).
   - Contextual fields (Endpoint, Model, API Key for HTTP; Command & Args for CLI; Builtin ID for Builtin).
   - Temperature & Max Tokens sliders/inputs.
   - "Test Connection" button with live status badge and latency indicator.
3. **Media Engines**:
   - Segmented sub-sections for **TTS**, **STT**, and **Image Generation**.
   - TTS controls: speech rate, pitch, default voice ID, auto-play narration toggle, master volume slider.
   - STT controls: endpoint/command, model.
   - Image controls: endpoint/command, model, auto-generate scene art toggle.
   - "Test Engine" button for each section.
4. **Preferences & Appearance**:
   - CRT & Noise backdrop overlays toggle.
   - Font scale selector (`small`, `medium`, `large`).
   - Narrative streaming toggle.
   - Simulated typing speed slider.

---

## 7. Testing & Quality Gates

1. **Unit Tests (`pkg/config`)**:
   - Test default config generation.
   - Test hierarchical loading: user global config vs local override.
   - Test config serialization and deserialization.
2. **Unit Tests (`pkg/gui`)**:
   - Test `GET /api/settings`, `PUT /api/settings`.
   - Test dynamic path re-binding in `Service`.
   - Test `POST /api/settings/test-provider` with mock/echo providers.
3. **Frontend Tests / Typecheck**:
   - TypeScript verification (`npx tsc --noEmit`).
   - Form state management, provider switching, and live diagnostics rendering.
4. **CLI Integration**:
   - Verify `cmd/localrpg` respects global config paths and engine defaults when running `play`, `tts`, and `image`.
