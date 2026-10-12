# LocalRPG

<p align="center">
  <img src="assets/logo.svg" alt="LocalRPG Logo" width="128" height="128" />
</p>

A local-first tabletop RPG client for crafting original worlds, designing custom mechanics, and playing immersive AI-orchestrated solo campaigns.

---

## Features

- **First-Class Local AI & Embeddings:** Route GM, narrator, and evaluator roles across local models (Ollama, LM Studio, vLLM, LocalAI) or cloud APIs, with local ONNX embeddings and zero-GPU built-ins.
- **AI World Generation & Living Lore:** Author lore, factions, and entities in Worlds Studio, or generate full settings through schema-first AI pipelines and draft review.
- **Custom Systems & Scenario Testing:** Design dice rules, outcome tiers, and character creation prompts with worked examples, verified against automated test scenarios.
- **Dynamic Scene Art & Style Packs:** Generate atmospheric scenes and character portraits with curated style packs, keeping baseline world aesthetics distinct from live campaign appearances.
- **TTS Narration & Voice Dictation:** Listen to narrated story prose with distinct voice profiles for each NPC, and interact hands-free with speech-to-text voice dictation.
- **Story Theatre, Packages & Export:** Step through visual novel replays, share creations via standalone content packages, and export campaigns as self-contained web bundles or rendered video.

---

## Creating Worlds & Playing Campaigns

LocalRPG is built around the tabletop creative cycle:

1. **Craft a World in Worlds Studio:** Author lore, tone, factions, and key locations by hand or generate complete settings with schema-first AI pipelines, batch entity expansion, and draft review.
2. **Define Rules in Systems Studio:** Create custom resolution systems, dice rules, and character creation prompts with worked examples, and verify system balance using automated test scenarios.
3. **Embark on a Campaign:** Take actions, roll checks, and shape the story. Speak your actions with microphone dictation while the AI Game Master adjudicates rules and narrates vivid scenes with voice and art.
4. **Relive & Export in Story Theatre:** Review past sessions in visual novel Theatre Mode, share worlds and rules via standalone content packages, or export campaigns as interactive web bundles or rendered video.

---

## Quickstart (`mise`)

The project uses [mise](https://mise.jdx.dev/) for pinned toolchain versioning (Go 1.27.1, Node 26.9.0) and task automation:

```bash
# Setup dependencies
mise run setup

# Build frontend and backend binary
mise run build

# Run all Go and TypeScript tests
mise run test

# Fuzz the turn-stream parser and the JSON repair for a longer budget
mise run test:fuzz

# Launch desktop GUI
bin/localrpg gui
```

---

## Living Campaigns & The Story Codex

Every campaign keeps track of what happened, who was involved, and how the world responded:

- **Automatic Entity Extraction:** As you explore and converse, newly met characters, places, and faction relationships are identified and cataloged into your campaign's Codex.
- **Dialogue with Speaker Identity:** Turns preserve narrative prose and spoken character dialogue separately, so voice playback uses each character's assigned voice profile.
- **Visual Codex Drawer:** Inspect character relationships, read discovered lore notes, and customize NPC voice profiles without leaving your game.
- **Coherent Story Rewind:** Use `/undo` anytime to step back campaign turns while keeping your world's authored history intact.

---

## Configuration & Provider Setup

LocalRPG features a unified settings system to connect your AI models and media engines:

- **Structured Studio Tabs:** Manage settings across dedicated tabs for Providers (LLMs, ONNX embeddings, shared API keys), AI Agents, Media Engines (TTS, STT, Image Generation), and Advanced preferences.
- **AI Agent Role Routing:** Route `gm`, `narrator`, and `evaluator` roles across local inference (Ollama, LM Studio, vLLM, LocalAI), cloud APIs, or built-in procedural engines.
- **Multimodal Engines:** Configure TTS (Piper, Kokoro, AllTalk, native-os), STT (Whisper, Web Speech API), and Image Generation (ComfyUI, Automatic1111, procedural-art) with volume, auto-play, and auto-generate art toggles.
- **Live Provider Diagnostics:** Test model and media engine connections directly from the UI with latency and preview feedback.
- **Dual-Access UI:** Access settings anytime from the **Settings** tab in Launcher Hub, or via the in-game header gear icon without leaving an active session.

---

## Built-in Engines & Local Provider Presets

LocalRPG works out of the box and doesn't need external dependencies, servers, or a GPU, while also supporting 1-click presets for popular local inference tools:

### Zero-GPU Built-in Engines

- **`narrative-oracle` Agent:** Pure-Go deterministic procedural storyteller that evaluates player action modes and the outcome tiers a dice roll produces, generating responsive narrative prose woven with entity wikilinks.
- **`native-os` TTS Client:** Dispatches narration to operating system speech synthesizers (`spd-say` on Linux, `/usr/bin/say` on macOS, PowerShell on Windows) with procedural audio waveform fallback.
- **`procedural-art` Image Generator:** Pure-Go vector dark fantasy SVG generator producing multi-layered atmospheric citadels, moonlit ridgelines, and misty marsh ruins customized by scene keywords.

### 1-Click Quick Presets

Settings Studio includes 1-click loaders that instantly prefill endpoint, model, and parameter defaults:

- **LLM / Agents:** Ollama (`localhost:11434`), LM Studio (`localhost:1234`), LocalAI (`localhost:8080`), vLLM (`localhost:8000`), `narrative-oracle` (built-in), and experimental CLI runners (`llama-cli`, `claude-cli`).
- **TTS (Speech):** Kokoro-FastAPI (`localhost:8880`), AllTalk (`localhost:7851`), Piper (`piper`), `native-os`, OpenAI Audio.
- **STT (Transcription):** Faster-Whisper (`localhost:8000`), Whisper.cpp (`whisper-cli`), OpenAI Whisper.
- **Image Generation:** ComfyUI (`127.0.0.1:8188`), Automatic1111 (`127.0.0.1:7860`), LocalAI (`localhost:8080`), `sd-cli`, `procedural-art`, DALL-E 3.

### NPC Voice Profiles Library

- **Archetype Catalog:** Includes default fantasy archetypes (`elder_sage`, `young_scout`, `gruff_blacksmith`, `sinister_cultist`) configuring `voice_id`, `pitch`, and `speech_rate`.
- **Automatic GM Voice Assignment:** The GM prompt is automatically injected with the active voice profile catalog. When new NPCs are introduced, the world extractor auto-assigns matching voice profiles based on tags or deterministic hash.
- **Per-Character Codex Overrides:** Select and inject voice profile frontmatter directly from the Codex Drawer note editor with one click.
- **Audio Cache Separation:** Speech cache keys uniquely isolate combinations of speaker, voice ID, pitch, speech rate, and text to remove audio cache collisions.

---

## Exporting a Campaign

Both exports are built from the same scene script, so they group turns by location, pace each beat by reading time (or by a clip's real length when one exists), and speak each line in the recorded speaker's own voice. Neither re-derives its own pacing or its own idea of what a scene is.

```bash
localrpg export web <game-id> [--out DIR] [--no-art] [--no-audio]
localrpg export video <game-id> [--out FILE|DIR] [--still] [--fps N] [--size WxH] [--quality N] [--effort N] [--intro D] [--outro D] [--gap D] [--progress] [--no-art] [--no-audio]
```

**Web bundle.** Writes a self-contained visual-novel player to `dist/<game-id>-web`, rendered by the same player the app's theatre uses: the stage and its scrim, the protagonist and the speaker on either side with the active one lit, the dialogue panel with its name plate, and markdown prose. It runs itself, blending between locations, revealing each beat's text as it's read, and ducking into a click-to-play state when the browser refuses to start audio without a gesture. Art, portraits, and audio are copied beside the page as sidecar assets and referenced by relative path. The bundle works from a file:// URL without a server or network access.

**Video.** Draws every frame in Go with the theatre's own stage, portraits, and dialogue panel, then encodes VP8 video and the campaign's Opus clips into one `.webm` with a seek index. Rendering and encoding happen inside the Go binary itself. `--still` renders one fully revealed frame per beat instead of animating, which is the fast path on a weak machine. `--quality` sets the VP8 quality (0-100) and `--effort` the encoder's effort (0-6, higher is slower and cleaner on coloured text and fine art). `--intro` and `--outro` hold the opening and closing picture before and after the story, and `--gap` lengthens every segment slightly beyond the script's pacing. `--out` takes a file or an existing directory, in which case the file is named after the game. `--progress` draws a live bar of frames drawn, frames repeated, and audio muxed against totals worked out before the render starts. Without it, an export prints only a final summary.

**Requirements.** Video export has no external requirements: the frames are composited, encoded, and muxed entirely in Go, and clip lengths are read from the Opus stream itself.

---

## Documentation

The manual ships inside the binary. Open it from the app's docs panel, or read it in
[`pkg/gui/docs/`](pkg/gui/docs), whose guides the showcase site also renders.

Start with [`00-start-here.md`](pkg/gui/docs/00-start-here.md), which lists every guide and
suggests an order to read them in. From there the shape is:

- **Core concepts** - [`01-overview.md`](pkg/gui/docs/01-overview.md) on the architecture,
  then worlds, systems and campaigns.
- **Studio guides** - building a world and a system in the app, and generating one from a prompt.
- **Configuration and providers** - wiring up LLMs and media, and what they cost.
- **Local AI and self-hosting** - running Ollama, Kokoro, Faster-Whisper and ComfyUI, or all
  of them together with one Compose file.

---

## Debugging & Diagnostics

LocalRPG includes an embedded debugging suite for diagnosing turn failures and inspecting prompts:

- **Interactive Debug Server**: Run `localrpg debug server --port 8080 --debugger-port 8089` to play in your browser with real-time prompt, span waterfall, and raw LLM completion inspection.
- **Automated Test Runner**: Run `localrpg debug test-run --scenario scenarios/smoke-test.yaml` to execute declarative browser scenarios headlessly and generate standalone HTML reports.

See [docs/debugging.md](docs/debugging.md) for full instructions and scenario syntax.

---

## License

LocalRPG is released under the [MIT License](LICENSE). It's built from
open source libraries and uses fonts and icons with their own terms;
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) lists them.
