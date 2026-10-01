# LocalRPG

A local-first, turn-based tabletop RPG client orchestrated by local and CLI LLMs, built in Go, Wails v3, and React 19.

---

## Features

- **Core Engine & Manifests:** Schema-agnostic YAML campaign manifests, path resolver, Markdown entity notes, and one canonical SQLite index per campaign.
- **Rule Extensibility:** Deterministic dice roller (`github.com/darkliquid/roll`), sandboxed JavaScript rules via Goja, and Wasm runtime via Wazero.
- **LLM Harness Router:** CLI subprocess integration (`claude`, `codex`, `agy`), streaming HTTP Ollama, 4-layer context assembler, and background entity extraction.
- **Campaign Timeline:** Every turn is recorded with the player's prompt, the narrator's rewrite, the entities involved, and who spoke which line — so a campaign's history is queryable rather than re-inferred from prose.
- **Headless Terminal TUI:** Interactive Bubbletea client with Glamour Markdown rendering, dice rolling, and `/gm` steering.
- **Multimodal Pipelines:** State-aware audio and art caching, per-character voice playback from recorded dialogue, ComfyUI image generator, and Whisper STT.
- **Wails v3 Desktop GUI:** Twintail Launcher inspired glassmorphic aesthetic in React 19 + TypeScript + Tailwind CSS.
- **Story Theater & Exporter:** In-game Visual Novel replay player, and two exports built from one scene script — an animated web bundle that plays through the theatre's own player, and a video rendered frame by frame in Go.

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

## Campaign Timeline & Entity Memory

A campaign lives in `games/<id>/` and answers "what happened, and who was involved" without re-reading prose:

- **One database:** `cache/index.db` is the single canonical index, opened through one code path by both the CLI and the GUI. A pre-existing `game.db` is retired automatically on first open.
- **Markdown is truth:** entity notes carry state, voice, and the turn numbers that touched them (`history: [3, 7]`); the index is derived and rebuilt from the notes plus `history.jsonl` whenever the two disagree.
- **Every turn is extracted:** entities are matched against existing notes (by ID, name, then location/role), merged when they already exist and created when they do not, then written to disk and synced.
- **Every turn is linked:** each `history.jsonl` record holds the player's raw prompt, the narrator's rewrite, and the entities involved, tagged by how (`player`, `location`, `wikilink`, `extracted`, `speech`).
- **Dialogue carries its speaker:** turns store ordered narration and speech segments with speakers resolved to entities, so playback uses each character's own voice instead of guessing from prose.
- **Undo is coherent:** `/undo` rewinds the log, the index, and entity turn links together. Entity prose is deliberately kept as the world's memory of what happened.

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

---

## Exporting a Campaign

Both exports are built from the same scene script, so they group turns by location, pace each beat by reading time (or by a clip's real length when one exists), and speak each line in the recorded speaker's own voice. Neither re-derives its own pacing or its own idea of what a scene is.

```bash
localrpg export web <game-id> [--out DIR] [--no-art] [--no-audio]
localrpg export video <game-id> [--out FILE|DIR] [--still] [--fps N] [--size WxH] [--quality N] [--effort N] [--intro D] [--outro D] [--gap D] [--progress] [--no-art] [--no-audio]
```

**Web bundle.** Writes a self-contained visual-novel player to `dist/<game-id>-web`, rendered by the same player the app's theatre uses: the stage and its scrim, the protagonist and the speaker on either side with the active one lit, the dialogue panel with its name plate, and markdown prose. It runs itself, blending between locations, revealing each beat's text as it is read, and ducking into a click-to-play state when the browser refuses to start audio without a gesture. Art, portraits, and audio are copied beside the page as sidecar assets and referenced by relative path, so the bundle works from a file:// URL with no server and no network access.

**Video.** Draws every frame in Go with the theatre's own stage, portraits, and dialogue panel, then encodes VP8 video and the campaign's Opus clips into one `.webm` with a seek index. No browser, no external binary, and no `ffmpeg` is required. `--still` renders one fully revealed frame per beat instead of animating, which is the fast path on a weak machine. `--quality` sets the VP8 quality (0-100) and `--effort` the encoder's effort (0-6, higher is slower and cleaner on coloured text and fine art). `--intro` and `--outro` hold the opening and closing picture before and after the story, and `--gap` holds every segment a little longer than the script paces it. `--out` takes a file or an existing directory, in which case the file is named after the game. `--progress` draws a live bar of frames drawn, frames repeated, and audio muxed against totals worked out before the render starts; without it an export stays quiet until it finishes.

**Requirements.** Video export has no external requirements: the frames are composited, encoded, and muxed entirely in Go, and clip lengths are read from the Opus stream itself.

---

## Debugging & Diagnostics

LocalRPG includes an embedded debugging suite for diagnosing turn failures and inspecting prompts:

- **Interactive Debug Server**: Run `localrpg debug server --port 8080 --debugger-port 8089` to play in your browser with real-time prompt, span waterfall, and raw LLM completion inspection.
- **Automated Test Runner**: Run `localrpg debug test-run --scenario scenarios/smoke-test.yaml` to execute declarative browser scenarios headlessly and generate standalone HTML reports.

See [docs/debugging.md](docs/debugging.md) for full instructions and scenario syntax.

---

## License

LocalRPG is released under the [MIT License](LICENSE). It is built from
open-source libraries and uses fonts and icons that carry their own terms;
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) lists them.
