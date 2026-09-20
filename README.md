# LocalRPG

A local-first, turn-based tabletop RPG client orchestrated by local and CLI LLMs, built in Go, Wails v3, and React 19.

---

## Features

- **Core Engine & Manifests:** Schema-agnostic YAML campaign manifests, path resolver, Markdown entity notes, and SQLite store.
- **Rule Extensibility:** Deterministic dice roller (`github.com/darkliquid/roll`), sandboxed JavaScript rules via Goja, and Wasm runtime via Wazero.
- **LLM Harness Router:** CLI subprocess integration (`claude`, `codex`, `agy`), streaming HTTP Ollama, 4-layer context assembler, and background entity extraction.
- **Headless Terminal TUI:** Interactive Bubbletea client with Glamour Markdown rendering, dice rolling, and `/gm` steering.
- **Multimodal Pipelines:** State-aware audio and art caching, Kokoro TTS dialogue attribution, ComfyUI image generator, and Whisper STT.
- **Wails v3 Desktop GUI:** Twintail Launcher inspired glassmorphic aesthetic in React 19 + TypeScript + Tailwind CSS.
- **Story Theater & Exporter:** In-app Visual Novel replay player with audio-synced text pacing, standalone HTML5 bundle exporter, and headless FFmpeg video rendering.

---

## Zero-TCP GUI Execution

LocalRPG runs **zero-TCP by default**:

### 1. Native Desktop Window (Default)
```bash
localrpg gui
```
- **0 TCP Ports:** Opens a native Wails v3 desktop window with translucent glassmorphic acrylic panels.
- Assets and REST API routes (`/api/game/...`) are served in-process directly to the WebKit webview via native OS scheme handlers.

### 2. Headless Unix Domain Socket Daemon
```bash
localrpg gui --headless
# Or specify a custom socket path:
localrpg gui --socket /path/to/localrpg.sock
```
- **0 TCP Ports:** Listens exclusively on a local Unix domain socket with `0600` permissions (readable/writable only by your user).
- Default path: `$XDG_RUNTIME_DIR/localrpg.sock` (or `~/.local/state/localrpg/gui.sock`).
- Automatically unlinks on clean shutdown and cleans up stale sockets.

### 3. Opt-in Web Browser Mode (TCP)
```bash
localrpg gui --port 8080
```
- Explicitly binds an HTTP listener on `127.0.0.1:8080` for standard external browser access.

---

## Build & Toolchain (`mise`)

The project uses [mise](https://mise.jdx.dev/) for pinned toolchain versioning (Go 1.27.1, Node 26.9.0) and task automation:

```bash
# Setup dependencies
mise run setup

# Build frontend and backend binary
mise run build

# Run all Go and TypeScript tests
mise run test

# Launch desktop GUI
bin/localrpg gui
```

---

## Configuration & Global Settings

LocalRPG features a unified global settings system that manages app-wide behavior regardless of active campaign, world, or rule system:

- **Hierarchical Loading:** Settings are loaded from `~/.config/localrpg/config.yaml` (global user configuration) and can be overridden per workspace via `./localrpg.yaml`.
- **Custom Storage Paths:** Configure directories for Rule Systems, Worlds, Campaigns, and Media Caches.
- **AI Agent Role Routing:** Route `gm`, `narrator`, and `evaluator` roles across `http` (Ollama, vLLM, OpenAI), `cli` (local binaries like llama-cli), `builtin`, or `disabled`.
- **Multimodal Engines:** Configure TTS (Piper, Kokoro, AllTalk), STT (Whisper), and Image Generation (ComfyUI, Automatic1111) with master volume, auto-play, and auto-generate art toggles.
- **Live Provider Diagnostics:** Test model and media engine connections directly from the UI with latency and preview feedback.
- **Dual-Access UI:** Access settings anytime from the **Settings** studio tab in Launcher Hub, or via the in-game header gear icon without leaving an active session.
