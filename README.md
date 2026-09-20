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
- **Multimodal Engines:** Configure TTS (Piper, Kokoro, AllTalk, native-os), STT (Whisper), and Image Generation (ComfyUI, Automatic1111, procedural-art) with master volume, auto-play, and auto-generate art toggles.
- **Live Provider Diagnostics:** Test model and media engine connections directly from the UI with latency and preview feedback.
- **Dual-Access UI:** Access settings anytime from the **Settings** studio tab in Launcher Hub, or via the in-game header gear icon without leaving an active session.

---

## Built-in Engines & Local Provider Presets

LocalRPG works completely out of the box with zero external dependencies, servers, or GPU requirements, while also supporting 1-click presets for popular local inference tools:

### Zero-GPU Built-in Engines
- **`narrative-oracle` Agent:** Pure-Go deterministic procedural storyteller that evaluates player action modes and dice roll outcome tiers, generating responsive narrative prose woven with entity wikilinks.
- **`native-os` TTS Client:** Dispatches narration to operating system speech synthesizers (`spd-say` on Linux, `/usr/bin/say` on macOS, PowerShell on Windows) with procedural audio waveform fallback.
- **`procedural-art` Image Generator:** Pure-Go vector dark fantasy SVG generator producing multi-layered atmospheric citadels, moonlit ridgelines, and misty swamp ruins customized by scene keywords.

### 1-Click Quick Presets
Settings Studio includes 1-click loaders that instantly prefill endpoint, model, and parameter defaults:
- **LLM / Agents:** Ollama (`localhost:11434`), LM Studio (`localhost:1234`), LocalAI (`localhost:8080`), vLLM (`localhost:8000`), `llama-cli`, `claude-cli`, `narrative-oracle`.
- **TTS (Speech):** Kokoro-FastAPI (`localhost:8880`), AllTalk (`localhost:7851`), Piper (`piper`), `native-os`, OpenAI Audio.
- **STT (Transcription):** Faster-Whisper (`localhost:8000`), Whisper.cpp (`whisper-cli`), OpenAI Whisper.
- **Image Generation:** ComfyUI (`127.0.0.1:8188`), Automatic1111 (`127.0.0.1:7860`), LocalAI (`localhost:8080`), `sd-cli`, `procedural-art`, DALL-E 3.

### NPC Voice Profiles Library
- **Archetype Catalog:** Ships with default fantasy archetypes (`elder_sage`, `young_scout`, `gruff_blacksmith`, `sinister_cultist`) configuring `voice_id`, `pitch`, and `speech_rate`.
- **Automatic GM Voice Assignment:** The GM prompt is automatically injected with the active voice profile catalog. When new NPCs are introduced, the world extractor auto-assigns matching voice profiles based on tags or deterministic hash.
- **Per-Character Codex Overrides:** Select and inject voice profile frontmatter directly from the Codex Drawer note editor with one click.
- **Audio Cache Separation:** Speech cache keys uniquely isolate combinations of speaker, voice ID, pitch, speech rate, and text to eliminate audio cache collisions.
