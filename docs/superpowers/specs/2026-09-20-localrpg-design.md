# System Design Specification: LocalRPG

**Date:** 2026-09-20  
**Status:** Approved  
**Target Platform:** Cross-platform Desktop (Linux, macOS, Windows) & Headless CLI  

---

## 1. Executive Summary

**LocalRPG** is a private, local-first, modular interactive storytelling and roleplaying game platform inspired by Voyage.io and AI Dungeon. It decouples game mechanics, narrative settings, and active campaigns into reusable layers, giving players total freedom to run any RPG system (or pure freeform narrative) using local open-weight models, external CLI harnesses (`claude`, `codex`, `agy`, `opencode`), or remote APIs.

### Key Capabilities
* **Dual-Mode Execution:** Operates as both an immersive desktop GUI app (via Wails v3 + React) and a full-featured headless terminal TUI (via Bubbletea).
* **Three-Tier Architecture:** Complete separation between **Systems** (mechanics/rules), **Worlds** (lore/settings/voices), and **Games** (isolated campaign instances), allowing systems and settings to be mixed and matched freely with optional world-level overrides.
* **Schema-Agnostic Extensibility:** No hardcoded RPG attributes (HP, Mana, Classes). Custom mechanics, dice checks, and state progressions are defined via declarative YAML schemas and executed in sandboxed **JavaScript (Goja)** or **WebAssembly (Wazero)**.
* **Bi-directional Knowledge Graph & Markdown Memory:** Every entity (NPC, location, item, faction, quest) is maintained as a transparent Markdown document with YAML frontmatter and `[[wikilinks]]`, synced continuously into a local SQLite database with `sqlite-vec` vector embeddings.
* **Living World Simulation:** Background narrative arcs and off-screen faction clocks continue progressing over in-game time, indirectly coloring atmosphere, prices, rumors, and NPC attitudes.
* **Role-Based Model & Harness Router:** Supports running different models or external CLI harnesses for distinct sub-tasks (GM Narrator, Graph Extractor, Living World Ticker, Image/TTS generation).
* **Multi-Voice TTS & Image Generation:** Out-of-the-box local multi-voice speech synthesis powered by Kokoro (ONNX) with pluggable providers, coupled with local ComfyUI/WebUI image generation. Includes state-aware content caching (e.g. invalidating audio when a character's voice is injured).
* **Director Steering & Errata Correction:** Multi-tiered timeline correction allowing inline text editing, `/gm` director steering directives with automatic entity state rollback, and snapshot-based timeline branching.
* **Visual Novel Story Replay & Multimodal Export:** Plays back campaigns from start to finish like a non-interactive visual novel (synchronized audio, text typewriter reveals, and scene transitions). Exports to in-app theater mode, standalone self-contained web player, or encoded MP4 video via FFmpeg.

---

## 2. Architecture & Dual-Mode Runtime

The platform is compiled as a single native Go binary with a decoupled core engine:

```text
┌─────────────────────────────────────────────────────────────┐
│                    User Interface Layer                     │
│  ┌──────────────────────────────┐  ┌─────────────────────┐  │
│  │ Wails v3 Desktop App (React) │  │ Terminal TUI (CLI)  │  │
│  └──────────────┬───────────────┘  └──────────┬──────────┘  │
└─────────────────┼─────────────────────────────┼─────────────┘
                  │       Typed Event Bus       │
┌─────────────────┴─────────────────────────────┴─────────────┐
│                     Core Engine (Go)                        │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Turn Orchestrator & Role-Based Model Router             │ │
│ ├────────────────────────────┬────────────────────────────┤ │
│ │ Extensibility Runtime      │ Memory & Knowledge Graph   │ │
│ │ (JS/Goja + Wasm/Wazero)    │ (Markdown Sync + SQLite)   │ │
│ ├────────────────────────────┼────────────────────────────┤ │
│ │ Audio & TTS (Kokoro/ONNX)  │ Image Gen (ComfyUI/APIs)   │ │
│ └────────────────────────────┴────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### 2.1 Runtime Modes
1. **Desktop GUI (`localrpg gui`):**
   * Uses **Wails v3** for a lightweight desktop footprint, native window management, menus, and system tray.
   * Frontend built with **React 19, TypeScript, and Tailwind CSS**.
   * Implements Option C layout: Immersive full-width reading chronicle with collapsible flyout drawers for character sheet, knowledge graph, markdown editor, and media deck.
2. **Headless Terminal TUI (`localrpg play <game-id>`):**
   * Uses **Bubbletea**, **Lipgloss**, and **Glamour** for rich terminal rendering.
   * Enables complete gameplay with formatted markdown, colored dialogue, inline roll checks, hotkey inspection of entities/arcs, and background audio playback.
3. **Core Engine Daemon (`pkg/engine`):**
   * Self-contained Go library containing the turn lifecycle, state persistence, graph synchronization, and harness runners. Accessible headless by CLI or via RPC/bindings by Wails.

---

## 3. Three-Tier Separation: Systems, Worlds, and Games

```text
LocalRPG/
├── systems/                        # Reusable Game Mechanics / Rulesets
│   ├── d20-classic/
│   │   ├── system.yaml             # Manifest (name, author, version)
│   │   ├── schema.yaml             # Generic state schemas, action types, UI widget hints
│   │   ├── mechanics.js            # Roll resolution, contest checks, damage logic
│   │   ├── mechanics.wasm          # (Optional) compiled Wasm rules module
│   │   └── prompts/rules.md        # GM system prompt instructions on how to adjudicate rules
│   └── pbta-narrative/
│       └── ...
│
├── worlds/                         # Reusable Settings / Universes / Lore
│   └── forgotten-reach/
│       ├── world.yaml              # Metadata (name, genre, recommended system: "d20-classic")
│       ├── prompts/lore.md         # Setting style, atmosphere, GM world-building instructions
│       ├── voices.yaml             # Voice archetype definitions
│       ├── entities/               # Base template lore notes (factions, locations, default NPCs)
│       │   ├── Eldoria.md
│       │   └── Iron-Pact.md
│       └── system_overrides/       # Optional system-specific tweaks
│           └── d20-classic/        # Overrides applied when paired with "d20-classic"
│               ├── custom_stats.yaml  # e.g., Adds "Corruption" or "Sanity" stat
│               └── hooks.js           # Custom event hooks
│
└── games/                          # Isolated Game Instances (Campaigns)
    └── campaign-01/
        ├── game.yaml               # Composition config (system, world, player, model routing)
        ├── history.jsonl           # Chronological turn log (inputs, outputs, rolls, audio links)
        ├── entities/               # Active campaign markdown notes (player sheet, mutated lore, new NPCs)
        │   ├── Player-Sean.md
        │   ├── Eldoria.md          # Mutated state (e.g. "Tavern burned down in turn 14")
        │   └── Lady-Evelyn.md      # Newly met NPC
        ├── assets/                 # Campaign-specific media
        │   ├── audio/              # Generated speech clips (.wav / .ogg)
        │   └── images/             # Generated scene illustrations & portraits (.webp)
        ├── cache/
        │   └── index.db            # SQLite + vector embeddings (rebuildable from entities/)
        └── saves/                  # Turn snapshots for rewinding or branching timelines
```

### 3.1 Composition Order
1. **Load System:** Core state structure, action types, and base mechanics from `systems/<system-id>/`.
2. **Apply World Overrides:** If `worlds/<world-id>/system_overrides/<system-id>/` exists, merge custom stats, additional actions, and hook overrides.
3. **Instantiate Game:** Load the active player profile and world entity templates into `games/<game-id>/entities/`, initializing `cache/index.db`.

---

## 4. Extensibility & Mechanics Runtime

The engine is **strictly schema-agnostic**—it contains no built-in assumptions about stats (HP, Mana, STR), classes, or inventory slots.

### 4.1 Schema Definition (`schema.yaml`)
Systems declare state properties, data types, and UI representation hints:
```yaml
schema_version: "1.0"
entities:
  character:
    properties:
      stats:
        type: object
        properties:
          strength: { type: integer, default: 10, ui: "number_stepper" }
          agility: { type: integer, default: 10, ui: "number_stepper" }
      resources:
        type: object
        properties:
          health: { type: integer, min: 0, max: 100, default: 100, ui: "progress_bar", color: "red" }
          stamina: { type: integer, min: 0, max: 100, default: 100, ui: "progress_bar", color: "green" }
      conditions:
        type: array
        items: { type: string }
        ui: "tag_list"
actions:
  - id: roll_attack
    label: "Attack"
    params: ["target", "weapon"]
```

### 4.2 Scripting Runtime (JavaScript & Wasm)
* **JavaScript Runtime (`Goja`):** Pure Go ECMAScript 5.1+ engine. Zero CGO dependencies.
* **WebAssembly Runtime (`Wazero`):** Zero-dependency WebAssembly runtime for Go. Allows compiling mechanics from Rust, Zig, Go, C, or AssemblyScript to `wasm32-unknown-unknown`.
* **Sandboxed Host API:**
```typescript
interface GameHostAPI {
  roll(notation: string): { total: number; dice: number[]; modifier: number };
  getEntity(id: string): EntityData;
  getStat(entityId: string, path: string): any;
  setStat(entityId: string, path: string, value: any): void;
  addItem(entityId: string, itemId: string, quantity?: number): void;
  removeItem(entityId: string, itemId: string, quantity?: number): boolean;
  onAction(actionType: string, handler: (ctx: ActionContext) => ActionResult): void;
  onTurnEnd(handler: (ctx: TurnContext) => void): void;
  onWorldTick(handler: (ctx: WorldContext) => void): void;
  injectGMDirection(directive: string): void;
  log(message: string): void;
}
```

---

## 5. Memory, Markdown Sync & Living World Simulation

### 5.1 Markdown Document Format
Entities are stored as standard Markdown files with YAML frontmatter:
```markdown
---
id: lady-evelyn
name: Lady Evelyn Vance
type: character
tags: [npc, noble, rogue]
voice:
  provider: kokoro
  voice_id: bf_emma
  pitch: 1.0
portrait: ./assets/images/characters/lady-evelyn.webp
location: "[[Alden-Tavern]]"
faction: "[[Iron-Pact]]"
state:
  health: 35
  armor: 14
  disposition: guarded
---

# Lady Evelyn Vance

Former lieutenant in the Iron Guard, Evelyn now operates in secret from the shadows of [[Alden-Tavern]].

## Known Facts
- Carries a concealed [[Vorpal-Dagger]] engraved with the seal of [[House-Vance]].
- Suspicious of outsiders, but will share information on [[The-Iron-Pact]] for coin or favors.
```

### 5.2 SQLite & Vector Synchronization (`cache/index.db`)
* **Tables:**
  * `entities`: `id`, `name`, `type`, `frontmatter_json`, `body_markdown`, `file_hash`, `updated_at`.
  * `edges`: `source_id`, `target_id`, `relation_type`, `context_snippet`.
  * `embeddings`: `entity_id`, `chunk_index`, `vector` (via `sqlite-vec`).
* **Ingestion:** Filesystem watcher monitors `games/<game-id>/entities/` and incrementally parses modified files. Rebuilding `index.db` from scratch is an idempotent operation taking under 2 seconds for hundreds of notes.

### 5.3 Living World Simulation & Narrative Arcs
* **Narrative Arc Entities (`type: arc`):** Track long-term off-screen developments (e.g. `Arc-The-Iron-Siege.md`) with progress clocks (e.g. `progress: 3/6`).
* **World Tick Evaluation:** Periodically (or on resting/travel), the engine triggers an off-screen tick. The `world_sim` model updates background faction agendas, advances clocks, and generates ambient rumors.
* **4-Layer Context Assembler:**
  1. **Immediate Scene Context:** Current location, present NPCs, player character sheet, active quest.
  2. **Living World Arcs & Background Agendas:** Top active arcs, current faction clocks, and thematic mood directives.
  3. **1-Hop Relationship Graph:** Directly linked lore, items, and factions of present entities.
  4. **Semantic Memory Retrieval:** Vector search matching the player's action against all past turn histories and lore.

---

## 6. Model & Harness Orchestration

### 6.1 Unified Provider Interface
```go
type ModelProvider interface {
    ID() string
    Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
    Stream(ctx context.Context, req GenerateRequest, out chan<- StreamChunk) error
}
```

### 6.2 Provider Backends
* **CLI Subprocess Harnesses (`cli`):** Invokes local CLI assistants (`claude`, `codex`, `agy`, `opencode`) via streaming standard I/O. Supports custom environment variables, working directories, and prompt arguments.
* **Local HTTP Services (`local_api`):** Native SSE streaming client for **Ollama**, **llama.cpp server**, **vLLM**, and **LM Studio**.
* **Direct Embedded Local Models (`embedded`):** In-process GGUF execution via `llama.cpp` bindings for zero-network, zero-dependency play.
* **Remote Cloud APIs (`remote_api`):** Anthropic (Claude 3.5/3.7), OpenAI (GPT-4o), OpenRouter, Groq, and Google Gemini.

### 6.3 Role-Based Configuration (`game.yaml`)
```yaml
models:
  gm:
    provider: cli
    command: "claude"
    args: ["--dangerously-skip-permissions"]
    temperature: 0.85
  extractor:
    provider: local_api
    endpoint: "http://localhost:11434"
    model: "qwen2.5:7b"
    temperature: 0.1
  world_sim:
    provider: local_api
    endpoint: "http://localhost:11434"
    model: "llama3.2:3b"
    temperature: 0.7
  mechanics:
    provider: script_engine
```

---

## 7. Media Generation & State-Aware Caching

### 7.1 Multi-Voice TTS Pipeline
* **Dialogue Parsing:** GM output is parsed into *Narrator Prose* and *Attributed Character Dialogue*.
* **Voice Resolution:** Resolves voice settings from entity markdown frontmatter (`voice: { provider: kokoro, voice_id: "bf_emma" }`) with fallback to world narrator voice.
* **Default Engine:** **Kokoro ONNX** running locally, supporting multiple natural voices. Pluggable adapters for Piper, OpenAI TTS, and ElevenLabs.
* **State-Aware Audio Cache Key:**
  $$\text{Key} = \text{SHA256}(\text{speaker\_id} + \text{voice\_config\_hash} + \text{utterance\_text})$$
  * If an NPC's voice is modified (e.g. pitch change or injury in markdown), subsequent lines synthesize new audio without invalidating historical lines.

### 7.2 Image Generation Pipeline
* **Triggers:** Automatic on visiting new locations or meeting new named NPCs; on-demand via "Illustrate Scene" and "Reroll Portrait" buttons.
* **Providers:** Local ComfyUI (HTTP/WebSocket API), Stable Diffusion WebUI (A1111/Forge), and remote APIs (OpenAI DALL-E, Stability AI, Fal).
* **State-Aware Art Cache Key:**
  $$\text{Key} = \text{SHA256}(\text{entity\_id} + \text{appearance\_hash} + \text{world\_style\_hash})$$
  * Changes to physical traits in markdown frontmatter invalidate the cached image and prompt fresh generation.

### 7.3 Speech-to-Text (STT) Player Voice Input
* **Functionality:** Provides optional push-to-talk voice dictation for players directly into the action console.
* **Engines Supported:**
  * **Local Engines:** Whisper via `whisper.cpp` (embedded/local binary) or `sherpa-onnx` for offline, low-latency speech recognition; Web Speech API in supported desktop webviews.
  * **Remote Fallback:** OpenAI Whisper API or Groq Whisper (for near-instant remote transcription).
* **Workflow:** Player holds the push-to-talk hotkey or clicks the microphone icon (🎙️), speaks their action or dialogue, and the transcribed text populates the input bar for review/submission.

---

## 8. User Interface & Interaction Design

### 8.1 Desktop App (Wails v3 + React 19)
* **Chronicle View:** Immersive reading view with serif typography, dialogue attribution bubbles, audio play/pause chips, and clickable `[[wikilinks]]`.
* **Flyout Drawers (Option C Layout):**
  * **Character Sheet Drawer:** Dynamic widgets (progress bars, stat dials, slot grids) generated automatically from `schema.yaml`.
  * **Knowledge Graph Drawer:** Interactive 2D force-directed graph with nodes colored by entity type and active scene nodes highlighted.
  * **Codex / Markdown Drawer:** Built-in markdown editor for inspecting and editing entity notes on the fly.
  * **Living World Drawer:** Visualization of active background arcs, faction clocks, and rumors.
* **Action Console:** Quick-select modes (`Do`, `Say`, `Story`, `Roll`), `@` and `[[` autocomplete for known entities, and a push-to-talk microphone button (🎙️) for Speech-to-Text input.

### 8.2 Terminal TUI (`bubbletea`)
* High-contrast, keyboard-driven terminal interface.
* Background audio playback of TTS voice clips during narration.
* Quick hotkeys for character stats (`c`), world arcs (`w`), and manual dice rolls (`r`).

---

## 9. GM Steering, Correction & Timeline Reconciliation

1. **Level 1: Direct Inline Edit:** Click and edit any story segment directly in the Chronicle; updates `history.jsonl`.
2. **Level 2: GM Steering ("Director's Note"):**
   * Accessible via `/gm <instruction>` or clicking "Correct Turn".
   * Packages context + previous flawed response + player's directive into a correction prompt.
   * Runs the background extractor on the corrected response to revert invalid entity state changes, cancel obsolete media jobs, and purge misattributed audio clips.
3. **Level 3: Timeline Rewind & Branching:**
   * Step back to any turn $N$.
   * Restores entity states from `saves/turn_N.json`, pruning orphaned history and asset entries.

---

## 10. Visual Novel Story Replay & Multimodal Export Pipeline

Because campaigns log every turn chronologically into `history.jsonl`—including player choices, GM responses, spoken audio clips, and generated scene illustrations—the game maintains an exact multimodal script of the unfolding story. LocalRPG leverages this to provide a non-interactive **Visual Novel Replay Engine**.

### 10.1 Chronicle Replay Engine ("Story Theater")
* **Playback Mechanics:**
  * **Audio-Paced Text Reveal:** Dialogue and narrative prose unfold using a typewriter effect synchronized with the audio duration of the accompanying speech clip.
  * **Visual Staging:** Background images crossfade smoothly as the player moves between locations; speaking NPC portraits slide in or highlight with a subtle breathing/focus glow during their spoken dialogue.
  * **Player Controls:**
    * Play / Pause toggle (`Spacebar`).
    * Scrubbing timeline bar with turn-by-turn tick marks.
    * Playback speed multiplier (1x, 1.25x, 1.5x, 2x).
    * Turn forward/backward skipping and Chapter/Location select.
    * Auto-advance toggle (moves to next turn automatically after audio finishes).

### 10.2 Export Targets
1. **In-App Theater Mode:**
   * Full-screen cinematic viewer directly inside the Wails v3 desktop app, as well as an auto-scrolling reader mode in the terminal TUI.
2. **Standalone Web Player (`localrpg export web <game-id>`):**
   * Exports the campaign into a portable, zero-dependency HTML5/JS bundle (`dist/web/index.html` + `assets/`).
   * Can be hosted on GitHub Pages, shared as a `.zip`, or opened locally in any web browser without installing LocalRPG.
3. **Headless Video Encoder (`localrpg export video <game-id>`):**
   * Uses an automated headless **FFmpeg** pipeline to composite scene illustrations, animated dialogue subtitle bars, character portraits, and synthesized audio tracks.
   * Produces a clean `.mp4` / `.webm` video file suitable for archival, Discord sharing, or YouTube uploads.

---

## 11. Development Roadmap & Implementation Milestones

* **Milestone 1: Core Engine & Data Storage**
  * Three-tier directory structure (`systems/`, `worlds/`, `games/`).
  * Markdown entity parser and `sqlite-vec` indexer.
  * Schema-agnostic state get/set methods.
* **Milestone 2: Extensibility Runtime & Mechanics**
  * Sandboxed JavaScript runtime via Goja.
  * WebAssembly runtime via Wazero.
  * Dice roller and pre/post action hook lifecycle.
* **Milestone 3: Model & Harness Router**
  * CLI subprocess harness runner (`claude`, `codex`, `agy`, `opencode`).
  * Local Ollama/OpenAI HTTP client with streaming support.
  * 4-Layer context assembler and background entity extractor.
* **Milestone 4: Headless Terminal TUI**
  * Bubbletea-based interactive terminal RPG client.
  * Turn history, roll inputs, and GM steering commands.
* **Milestone 5: Media Pipelines (Kokoro TTS, ComfyUI & Whisper STT)**
  * Kokoro ONNX speech synthesis with dialogue attribution and state-aware caching.
  * ComfyUI / SD WebUI image generation integration.
  * Push-to-talk Speech-to-Text (STT) player dictation via local Whisper/Sherpa-ONNX and remote APIs.
* **Milestone 6: Wails v3 Desktop GUI**
  * React + TypeScript + Tailwind desktop application.
  * Immersive chronicle view with flyout drawers (Character Sheet, Graph Canvas, Codex Editor, Living World Arcs).
* **Milestone 7: Story Theater & Multimodal Export**
  * In-app Visual Novel replay player with audio-synced text pacing.
  * Standalone web bundle exporter (`localrpg export web`).
  * Headless FFmpeg video rendering pipeline (`localrpg export video`).
