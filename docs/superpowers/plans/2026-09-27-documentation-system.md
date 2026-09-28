# Built-in Help and Documentation System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide an embedded, comprehensive, offline help and usage documentation system with deep-linking, real-time search, and rich markdown rendering accessible via the launcher dock and chronicle header.

**Architecture:** 10 structured Markdown documentation files embedded into the Go binary via `embed.FS` in `pkg/gui/docs/*.md`. `pkg/gui/docs.go` parses frontmatter metadata and exposes `GetDocsList` and `GetDocArticle` through `gui.Service` and `gui.Server` endpoints (`/api/docs`, `/api/docs/{id}`). The frontend `APIClient` fetches summaries and content, and `DocsModal.tsx` renders a two-pane glassmorphic reader with category tree, real-time search, breadcrumb navigation, and `MarkdownDocViewer.tsx` for syntax-highlighted code blocks with copy buttons and callout styling.

**Tech Stack:** Go 1.27 (`embed`, `gopkg.in/yaml.v3`, standard library `net/http`, `testing`), React 19 + TypeScript, Tailwind CSS v4, Lucide React icons.

---

### File Map

| Action | Path | Description |
|---|---|---|
| Create | `pkg/gui/docs/01-overview.md` | Core Concepts: Architecture, Schema-Agnostic Design, 3-tier separation |
| Create | `pkg/gui/docs/02-worlds.md` | Core Concepts: Worlds, Lore Prompts, Starting Locations, Overrides |
| Create | `pkg/gui/docs/03-systems.md` | Core Concepts: Systems, Rules Prompts, Action Modes, Dice, Mechanics Hooks |
| Create | `pkg/gui/docs/04-campaigns.md` | Core Concepts: Campaigns, Turn Lifecycle, History Log, Rewind/Undo, Arcs |
| Create | `pkg/gui/docs/05-providers.md` | Configuration: Builtin/CLI/HTTP/Mock providers, LLMs, Voice, Image, Spend & Limits |
| Create | `pkg/gui/docs/06-agents.md` | Configuration: Agent roles (GM, Narrator, Extractor), models, context token budgets |
| Create | `pkg/gui/docs/07-storage-paths.md` | Configuration: XDG paths, workspace modes, disposable cache/index.db |
| Create | `pkg/gui/docs/08-worlds-studio.md` | Studio Guides: Building worlds, genre, art style tags, starter entities |
| Create | `pkg/gui/docs/09-systems-studio.md` | Studio Guides: Building systems, rules prompts, mechanics.js authoring & testing |
| Create | `pkg/gui/docs/10-codex-yaml.md` | Codex Reference: Frontmatter YAML schema, entity types, state, wikilinks graph |
| Modify | `pkg/gui/types.go` | Add `DocArticleSummaryDTO` and `DocArticleDTO` |
| Create | `pkg/gui/docs.go` | Embedded FS, frontmatter parser, Service methods `GetDocsList`, `GetDocArticle` |
| Modify | `pkg/gui/server.go` | Add `/api/docs` and `/api/docs/{id}` routes, handler, and metric label |
| Create | `pkg/gui/docs_test.go` | Unit & HTTP endpoint tests for docs listing, retrieval, sorting, and 404s |
| Modify | `frontend/src/types.ts` | Add `DocArticleSummary` and `DocArticle` TypeScript interfaces |
| Modify | `frontend/src/api/client.ts` | Add `getDocsList` and `getDocArticle` API methods |
| Create | `frontend/src/components/MarkdownDocViewer.tsx` | Markdown renderer with syntax-highlighted code blocks, copy buttons, callouts |
| Create | `frontend/src/components/DocsModal.tsx` | Two-pane docs modal with categories, search, breadcrumbs, article reader |
| Modify | `frontend/src/components/launcher/LauncherDock.tsx` | Add Help/Docs icon button (`HelpCircle`) in bottom dock |
| Modify | `frontend/src/components/LauncherHub.tsx` | Add `isDocsOpen` state and mount `DocsModal` |
| Modify | `frontend/src/App.tsx` | Add `Docs` header button and mount `DocsModal` with optional deep-linking |

---

### Task 1: Embedded Markdown Documentation Content

Create all 10 Markdown files under `pkg/gui/docs/` with complete, accurate frontmatter and detailed reference prose reflecting the actual LocalRPG engine.

**Files:**
- Create: `pkg/gui/docs/01-overview.md`
- Create: `pkg/gui/docs/02-worlds.md`
- Create: `pkg/gui/docs/03-systems.md`
- Create: `pkg/gui/docs/04-campaigns.md`
- Create: `pkg/gui/docs/05-providers.md`
- Create: `pkg/gui/docs/06-agents.md`
- Create: `pkg/gui/docs/07-storage-paths.md`
- Create: `pkg/gui/docs/08-worlds-studio.md`
- Create: `pkg/gui/docs/09-systems-studio.md`
- Create: `pkg/gui/docs/10-codex-yaml.md`

- [x] **Step 1: Create `pkg/gui/docs/01-overview.md`**

```markdown
---
id: 01-overview
title: Architecture & Core Concepts
category: Core Concepts
order: 1
description: Schema-agnostic design, three-tier separation, and how AI agents drive tabletop adventures.
---

# Architecture & Core Concepts

LocalRPG is a local-first, turn-based tabletop RPG client built in Go and TypeScript. It lets you play immersive solo tabletop roleplaying games powered by local or cloud AI models, procedural media generation, and modular game mechanics.

## The Schema-Agnostic Design

Traditional digital RPGs hardcode character classes, hit points, mana bars, and spell slots directly into their engine databases. LocalRPG takes an entirely different approach: **it has zero hardcoded RPG stats or mechanics**.

Instead, LocalRPG is **schema-agnostic**:
- Every character, location, faction, and item is a plain Markdown file with a YAML frontmatter block.
- Game mechanics (dice rolls, stats, inventories, fatigue, spell slots) are defined by sandboxed JavaScript/Wasm rules engines.
- The AI Game Master (GM) and Narrator read these rules and lore directly from structured prompt contexts, adapting to any genre or ruleset.

## Three-Tier On-Disk Separation

LocalRPG cleanly separates mechanics, setting, and play sessions across three distinct directory tiers:

1. **Systems (`systems/<id>/`)**
   The rules of the game:
   - `system.yaml`: Manifest declaring system identity, action modes, and metadata.
   - `mechanics.js`: Sandboxed JavaScript hooks evaluating actions, checks, and state mutations.
   - `prompts/rules.md`: Core rules guidance provided to the GM agent.

2. **Worlds (`worlds/<id>/`)**
   The setting and lore:
   - `world.yaml`: World manifest detailing name, description, genre, and art style tags.
   - `prompts/lore.md`: Foundational background, history, tone, and cultural guidelines.
   - `entities/*.md`: Authored templates for factions, key figures, locations, and starter items.
   - `system_overrides/<system-id>/hooks.js`: Optional world-specific mechanics extensions.

3. **Games (`games/<id>/`)**
   An active campaign instance:
   - `game.yaml`: Campaign configuration linking a specific World and System.
   - `history.jsonl`: The append-only canonical timeline of every turn.
   - `entities/*.md`: Living campaign entities mutated and created as the adventure progresses.
   - `assets/`: Generated character portraits, scene banners, and audio narrations.
   - `cache/index.db`: High-performance SQLite cache disposable and rebuildable from Markdown.

## The Agent Triad

Three specialized AI agent roles collaborate to create each turn:

- **Game Master (`gm`)**: Evaluates player choices against system rules, decides difficulty, determines whether checks are required, and issues directives.
- **Narrator (`narrator`)**: Converts GM decisions and player actions into atmospheric prose, dialogue, and stage directions.
- **Extractor (`extractor`)**: Inspects narrative prose to discover newly introduced NPCs, places, and relationship changes, updating the campaign Codex.
```

- [x] **Step 2: Create `pkg/gui/docs/02-worlds.md`**

```markdown
---
id: 02-worlds
title: Worlds & Lore
category: Core Concepts
order: 2
description: Anatomy of a world, starting location resolution, and system overrides.
---

# Worlds & Lore

A **World** defines the setting, lore, aesthetics, and starting conditions for campaigns. Worlds are completely modular and can be paired with any game system.

## Anatomy of a World

Each world directory (`worlds/<id>/`) contains:

```
worlds/eldoria/
├── world.yaml                  # Identity, tags, and settings
├── prompts/
│   └── lore.md                 # Fundamental lore and tone prompt
├── entities/                   # Starter entity templates
│   ├── the-iron-bastion.md     # Starter location
│   └── lord-aldous.md          # Starter faction/character
└── system_overrides/           # System-specific hooks
    └── classic-d20/
        └── hooks.js
```

### World Manifest (`world.yaml`)

```yaml
id: eldoria
name: The Sunken Reach
description: A mist-shrouded archipelago of submerged ruins and arcane salvage.
genre: nautical-fantasy
tags: [mysterious, grim, salvage, ocean]
settings:
  start_location: the-iron-bastion
  time_progression: turns
```

## Starting Location Resolution

When a new campaign begins, LocalRPG determines the opening scene using a deterministic 4-stage resolution hierarchy:

1. **Pinned Setting (`settings.start_location`)**: If specified in `world.yaml` or `game.yaml`, the engine binds to that specific entity ID.
2. **Player Wikilink Target**: If the player character's YAML frontmatter includes a `location: "[[The Sinking Quay]]"` reference, that location takes precedence.
3. **Any Authored Location**: If no location is pinned or referenced, the engine scans `entities/` for any entity with `type: location`.
4. **Procedural Fallback**: If no locations exist in the world, the engine synthesizes an opening scene note (`games/<id>/entities/opening-scene.md`) derived directly from the world name and description.

## System Overrides

Sometimes a world introduces setting-specific mechanics (e.g. sanity in a Lovecraftian setting or oxygen consumption in hard sci-fi). Worlds can provide custom JavaScript hooks inside `system_overrides/<system-id>/hooks.js`. When a campaign runs with that specific system, these hooks merge with the base system mechanics.
```

- [x] **Step 3: Create `pkg/gui/docs/03-systems.md`**

```markdown
---
id: 03-systems
title: Systems & Mechanics
category: Core Concepts
order: 3
description: Rules prompts, action modes, dice expressions, and the sandboxed JavaScript mechanics engine.
---

# Systems & Mechanics

A **System** defines how actions are resolved, what dice are rolled, how characters progress, and how rules are enforced.

## System Structure

```
systems/classic-d20/
├── system.yaml         # System metadata, modes, and dice definitions
├── mechanics.js        # Sandboxed JavaScript mechanics engine
└── prompts/
    └── rules.md        # Natural language instructions for the GM agent
```

## Action Modes

LocalRPG supports four fundamental action modes configured in `system.yaml`:

- **`do`**: Physical or active interventions ("I leap across the chasm").
- **`say`**: Direct dialogue or social interactions ("I ask the merchant about the lost amulet").
- **`story`**: Narrative establishment or background declarations ("Ten years ago, my guild swore an oath...").
- **`roll`**: Explicit rules checks evaluated against the mechanics engine ("Roll Athletics DC 15").

## Dice Expressions

LocalRPG features a built-in dice evaluation engine supporting standard tabletop notations:
- `1d20 + 5`: Roll a 20-sided die and add 5.
- `2d6`: Roll two six-sided dice and sum the results.
- `4dF`: Fate/Fudge dice (values -1, 0, +1).
- `1d100` / `d%`: Percentile dice.
- `3d6kh2`: Keep highest 2 of 3 six-sided dice.

## Sandboxed JavaScript Mechanics Engine (`mechanics.js`)

Custom systems export JavaScript functions that execute inside an isolated Goja runtime. The engine passes turn state and receives structured mutations:

```javascript
// Example mechanics.js
function resolveAction(ctx) {
  // ctx.player contains player character state and frontmatter
  // ctx.action contains the player's prompt and action mode
  if (ctx.action.mode === 'roll') {
    const roll = rollDice('1d20 + ' + (ctx.player.state.athletics || 0));
    const targetDC = ctx.action.dc || 12;
    const success = roll.total >= targetDC;

    return {
      success: success,
      roll: roll,
      narrative_cue: success ? "Feat succeeded with style." : "Complication arises.",
      state_patch: {
        stamina: (ctx.player.state.stamina || 10) - 1
      }
    };
  }

  return { pass_to_gm: true };
}
```

The mechanics engine exposes hooks including `onTurnStart`, `resolveAction`, and `onTurnEnd`.
```

- [x] **Step 4: Create `pkg/gui/docs/04-campaigns.md`**

```markdown
---
id: 04-campaigns
title: Campaigns & Turns
category: Core Concepts
order: 4
description: The turn lifecycle, canonical history log, non-destructive rewinding, and living-world progression.
---

# Campaigns & Turns

A **Campaign** is an active instance of a World played under a System. It preserves full continuity through an immutable timeline, dynamic relationship graphs, and evolving character sheets.

## The Turn Lifecycle

Every turn passes through a disciplined multi-step pipeline:

```
[Player Action] ──> [GM Directives / Undo Check]
                          │
                          ▼
            [Dice / Mechanics Evaluation]
                          │
                          ▼
            [4-Layer Context Prompt Assembly]
                          │
                          ▼
              [Narrator Prose Generation]
                          │
                          ▼
           [Entity Mention & Dialogue Parsing]
                          │
                          ▼
       [Entity Extraction & Graph Reconciliation]
                          │
                          ▼
        [history.jsonl Append & Index Update]
```

1. **Directive Handling**: The engine detects slash commands like `/undo` or `/gm <directive>` (which injects steering instructions to the GM).
2. **Mechanics Resolution**: If the action requires dice or triggers a JS hook, the outcome is computed before the GM narrates.
3. **Context Layering**: The engine builds a focused prompt assembling:
   - System rules and mechanics cues.
   - World lore and tone constraints.
   - Voice profile catalog.
   - Scene scope: reachable entities from current location via knowledge graph edges.
   - Active narrative arcs and faction clocks.
4. **Narration & Speech**: The narrator produces prose with inline speaker tags and stage directions.
5. **Timeline Recording**: Turn segments are written to `history.jsonl` and mirrored to `cache/index.db`.

## The `history.jsonl` Canonical Log

Campaign history is stored as append-only newline-delimited JSON (`games/<id>/history.jsonl`). Each record captures:
- Sequential turn number.
- Raw player prompt.
- Narrator's rewritten output.
- Turn segments with identified speakers.
- Mentioned entity IDs and newly discovered concepts.
- Dice roll results and mechanics state patches.

## Non-Destructive Rewind (`/undo`)

Because `history.jsonl` is the source of truth, invoking `/undo` rewinds the campaign cleanly:
- The log is truncated back to the chosen turn number.
- `cache/index.db` turn entries are pruned.
- Entity history turn markers are rolled back, while authored entity lore and character sheets remain intact.
```

- [x] **Step 5: Create `pkg/gui/docs/05-providers.md`**

```markdown
---
id: 05-providers
title: AI & Media Providers
category: Configuration & Providers
order: 5
description: Configuring LLMs, voice synthesis, speech recognition, image generation, and spend tracking.
---

# AI & Media Providers

LocalRPG uses a uniform provider configuration architecture for all AI models, voice synthesizers, speech recognition, and image generators.

## Provider Architecture

Every provider in `config.yaml` follows the same schema:

```yaml
providers:
  my-provider-id:
    type: http # builtin | cli | http | mock | disabled
    builtin_name: "" # Used when type is builtin
    base_url: "https://api.openai.com/v1"
    api_key: "env:OPENAI_API_KEY"
    model: "gpt-4o"
```

### Supported Provider Types

- **`builtin`**: Runs directly in the LocalRPG process without external dependencies or GPU requirements.
- **`http`**: Connects via HTTP/REST to local daemons (Ollama, LM Studio, Sherpa, ComfyUI) or cloud APIs (OpenAI, Gemini, Anthropic, ElevenLabs).
- **`cli`**: Spawns command-line binaries (e.g. `whisper.cpp`, `spd-say`, custom scripts).
- **`mock`**: Returns deterministic placeholder responses for offline testing and development.
- **`disabled`**: Explicitly disables the capability.

## LLM Providers

### Ollama (Local)
Run models locally with zero external network access:
```yaml
providers:
  local-llama:
    type: http
    base_url: "http://localhost:11434"
    model: "llama3.1:8b"
```

### Gemini API (Cloud)
```yaml
providers:
  gemini-flash:
    type: http
    base_url: "https://generativelanguage.googleapis.com"
    api_key: "env:GEMINI_API_KEY"
    model: "gemini-2.0-flash"
```

### Narrative Oracle (Built-in)
Zero-setup built-in fallback model that generates narrative choices using procedural oracle tables.

## Media Providers

### Voice (TTS)
- **ElevenLabs**: High-fidelity AI speech (`type: http`, `base_url: https://api.elevenlabs.io`).
- **Gemini Voice**: Multimodal speech synthesis.
- **Native OS (`builtin_name: native-os`)**: Built-in speech using your operating system's native synthesizer (`say` on macOS, `spd-say` on Linux, PowerShell SAPI on Windows).
- **Sherpa / Piper**: High quality local neural speech synthesis.

### Image Generation
- **Procedural Art (`builtin_name: procedural-art`)**: Pure-Go SVG generator creating heraldic banners, landscape silhouettes, and item icons without a GPU.
- **ComfyUI / Automatic1111**: Local Stable Diffusion web APIs.
- **Google Imagen**: Cloud image synthesis.

## Spend Ledger & Rate Limit Handling

LocalRPG protects you from runaway API costs and service interruptions:
- **Rate Limits (HTTP 429)**: The engine automatically applies exponential backoff with jitter.
- **Insufficient Funds (HTTP 402)**: Instantly pauses background generation and displays a warning chip in the interface.
- **Spend Ledger**: Tracks exact token usage, estimated costs, and requests per model, viewable in Global Settings.
```

- [x] **Step 6: Create `pkg/gui/docs/06-agents.md`**

```markdown
---
id: 06-agents
title: AI Agents & Roles
category: Configuration & Providers
order: 6
description: Assigning models to GM, Narrator, and Extractor roles, plus token budgets and fallback chains.
---

# AI Agents & Roles

In LocalRPG, distinct cognitive tasks are assigned to specialized **Agent Roles**. You can map different AI models to each role based on their strengths, speeds, and costs.

## Core Agent Roles

1. **`gm` (Game Master)**
   - Responsible for rules adjudication, difficulty checks, world logic, and pacing.
   - Best suited for reasoning-heavy models (e.g. `claude-3-5-sonnet`, `gemini-2.0-pro`, `gpt-4o`, `qwen2.5:14b`).

2. **`narrator`**
   - Responsible for literary description, evocative dialogue, sensory immersion, and atmospheric stage directions.
   - Best suited for creative writing models (e.g. `gemini-2.0-flash`, `mistral-large`, `llama3.1:8b`).

3. **`extractor`**
   - Runs in the background at turn completion to parse entities, character introductions, inventory changes, and relationship tags into the campaign Codex.
   - Best suited for fast, structured-output models (e.g. `gemini-2.0-flash-lite`, `gpt-4o-mini`, `llama3.2:3b`).

## Role Mapping in Configuration

Configure agent role assignments under `agents.roles` in your `config.yaml`:

```yaml
agents:
  roles:
    gm:
      provider: gemini-pro
      temperature: 0.7
      max_tokens: 1024
      fallback: local-llama
    narrator:
      provider: gemini-flash
      temperature: 0.9
      max_tokens: 1500
    extractor:
      provider: local-llama
      temperature: 0.2
      max_tokens: 512
```

## Token Budgets & Context Management

The orchestrator dynamically fits prompt layers into the configured `context_window` limit:
- Priority is given to system rules and character sheet state.
- Entity memories and living-world notes are prioritized based on proximity in the knowledge graph.
- Historical turns are compressed or truncated using sliding-window recaps when context headroom is low.
```

- [x] **Step 7: Create `pkg/gui/docs/07-storage-paths.md`**

```markdown
---
id: 07-storage-paths
title: Storage & Directories
category: Configuration & Providers
order: 7
description: Standard XDG directories, workspace mode, and disposable SQLite caching.
---

# Storage & Directories

LocalRPG follows modern operating system standards for file organization while supporting portable workspace modes.

## Standard XDG Storage Locations

By default, LocalRPG stores user data and caches in standard OS directories (via XDG specification):

### Linux & BSD
- **Data (`$XDG_DATA_HOME/localrpg` or `~/.local/share/localrpg/`)**:
  - `systems/`: Installed and custom game systems.
  - `worlds/`: Installed and custom worlds.
  - `games/`: Campaign folders, logs, and assets.
- **Cache (`$XDG_CACHE_HOME/localrpg` or `~/.cache/localrpg/`)**:
  - Downloaded models, generated audio cache, and image thumbnails.
- **Config (`$XDG_CONFIG_HOME/localrpg` or `~/.config/localrpg/config.yaml`)**:
  - Global user configuration and provider keys.

### macOS
- Data: `~/Library/Application Support/localrpg/`
- Cache: `~/Library/Caches/localrpg/`
- Config: `~/Library/Application Support/localrpg/config.yaml`

### Windows
- Data: `%LOCALAPPDATA%\localrpg\`
- Cache: `%LOCALAPPDATA%\localrpg\cache\`
- Config: `%APPDATA%\localrpg\config.yaml`

## Workspace & Project Mode

When you launch LocalRPG inside a repository containing `./localrpg.yaml` or pass the `--dir <path>` CLI flag:
- The working directory becomes the project root.
- Relative paths in configuration resolve directly against that root.
- Content is kept isolated, making campaigns and systems fully portable in Git repositories.

## Cache Safety & `cache/index.db`

Each campaign directory contains a SQLite database at `games/<id>/cache/index.db`.
- This database is strictly an index cache for rapid full-text search, graph queries, and turn listing.
- **It is 100% disposable**: If you delete `cache/index.db`, LocalRPG automatically rebuilds it from the Markdown notes and `history.jsonl` upon next launch.
- Never edit `index.db` directly; always edit the Markdown files in `entities/`.
```

- [x] **Step 8: Create `pkg/gui/docs/08-worlds-studio.md`**

```markdown
---
id: 08-worlds-studio
title: Building Worlds Studio Guide
category: Studio Guides
order: 8
description: Walkthrough for creating custom worlds, defining lore prompts, and authoring starter entities.
---

# Building Worlds: Studio Guide

The **Worlds Studio** provides a dedicated visual workspace for authoring immersive settings, creating factions and starter locations, and generating procedural heraldry.

## Step-by-Step: Creating a New World

1. Open the **Worlds Studio** by clicking the Globe icon on the launcher dock or visiting the World tab in Global Settings.
2. Click **Create World** in the top right.
3. Configure World Identity:
   - **ID**: A unique kebab-case slug (e.g. `sunken-citadel`).
   - **Name**: Display title (e.g. `The Sunken Citadel`).
   - **Description**: A 1-2 sentence pitch summarizing the atmosphere and premise.
   - **Genre**: The broad narrative tradition (e.g. `dark-fantasy`, `cyberpunk`, `solarpunk`, `space-western`).
   - **Style Tags**: Visual and tonal tags (e.g. `decay, neon, torrential-rain, ancient-machinery`).

## Authoring the Lore Prompt (`prompts/lore.md`)

The lore prompt is the bedrock for the AI Narrator. Structure your lore with clear headings:

```markdown
# World Lore: The Sunken Citadel

## Core Theme
A vast drowned metropolis where scavengers dive into submerged towers for pre-fall technology.

## Tone & Atmosphere
Grim, humid, mysterious, and cautious. Salt corrodes everything.

## Factions & Powers
- **The Rust Divers**: Hardy scavengers who operate salvage diving bells.
- **The Salt Monks**: Fanatical hermits who worship the rising tides.

## Sensory Guide
Describe the drip of brackish water, the squeal of rusted iron pulleys, and the bioluminescent glow of abyssal flora.
```

## Creating Starter Entities

In the **Entities** panel of the Worlds Studio, add key starter notes:
- **Locations**: Create at least one primary location (e.g. `the-diving-dock.md`) and set `type: location`.
- **Factions**: Create founding groups and allegiances.
- **Key Figures**: Add memorable NPCs with distinctive mannerisms and voice tags.

## Generating Banners and Icons

Click the **Generate Artwork** button on your world card to generate procedural SVG heraldry or trigger an image generation prompt based on your genre tags.
```

- [x] **Step 9: Create `pkg/gui/docs/09-systems-studio.md`**

```markdown
---
id: 09-systems-studio
title: Building Custom Systems Guide
category: Studio Guides
order: 9
description: Authoring system rules prompts, coding mechanics.js hooks, and testing dice expressions.
---

# Building Custom Systems: Studio Guide

The **Systems Studio** enables you to craft tabletop mechanics from scratch or recreate your favorite roleplaying games.

## System Configuration (`system.yaml`)

```yaml
id: grim-survival
name: Grim Survival d6
description: A gritty ruleset focusing on stamina depletion, scarcity, and dangerous skill checks.
version: "1.0.0"
action_modes:
  - id: do
    label: Action
  - id: say
    label: Dialogue
  - id: roll
    label: Skill Check
```

## Crafting the Rules Prompt (`prompts/rules.md`)

The rules prompt guides the GM agent when calling for checks:

```markdown
# Rules System: Grim Survival

## Core Resolution
When the player attempts a risky or uncertain action, call for a d6 check:
- **1-2**: Failure with severe consequence or injury.
- **3-4**: Partial success with a complication or stamina loss.
- **5-6**: Clean success.

## Difficulty Modifiers
- Difficult tasks impose a -1 modifier.
- Prepared equipment or relevant traits grant +1.
```

## Writing `mechanics.js` Hooks

Create custom resolution logic in JavaScript. The sandbox includes helper methods like `rollDice(notation)`:

```javascript
/**
 * resolveAction is invoked whenever a turn requires mechanics evaluation.
 * @param {Object} ctx - The execution context
 * @returns {Object} Resolution outcome and state mutations
 */
function resolveAction(ctx) {
  if (ctx.action.mode === 'roll') {
    const roll = rollDice('1d6');
    let outcome = 'failure';
    let cue = 'Things go terribly wrong.';

    if (roll.total >= 5) {
      outcome = 'success';
      cue = 'You achieve your goal cleanly.';
    } else if (roll.total >= 3) {
      outcome = 'mixed';
      cue = 'You succeed, but pay a price.';
    }

    return {
      success: outcome !== 'failure',
      outcome: outcome,
      roll: roll,
      narrative_cue: cue,
      state_patch: {
        last_roll: roll.total
      }
    };
  }

  return { pass_to_gm: true };
}
```

## Live Studio Testing

Use the built-in **Dice & Rules Tester** at the bottom of the Systems Studio to execute trial actions, verify dice formulas, and inspect returned state patches before deploying your system to a campaign.
```

- [x] **Step 10: Create `pkg/gui/docs/10-codex-yaml.md`**

```markdown
---
id: 10-codex-yaml
title: Codex YAML Frontmatter & Graph Reference
category: Codex & Content Reference
order: 10
description: Complete YAML frontmatter schema, entity types, custom state, and wikilink graph modeling.
---

# Codex YAML Frontmatter & Graph Reference

In LocalRPG, every entity in your campaign (characters, locations, factions, items, and concepts) is represented as a plain Markdown file in `entities/` with YAML frontmatter enclosed in `---` delimiters.

## Full Entity Frontmatter Schema

```yaml
---
id: lady-evelyn
name: Lady Evelyn Vance
type: character # character | location | faction | item | concept
tags: [noble, merchant, ally, secretive]
location: "[[the-iron-bastion]]"
faction: "[[the-gilded-cabal]]"
portrait: "assets/portraits/lady-evelyn.png"
appearance: "A slender woman in midnight-blue velvet holding an engraved silver spyglass."
gender: female
age: "34"
aliases:
  - The Blue Falcon
  - Evelyn Vance
voice:
  provider: elevenlabs
  voice_id: "21m00Tcm4TlvDq8ikWAM"
  pitch: 1.0
  speech_rate: 1.05
state:
  hp: 24
  max_hp: 24
  disposition: friendly
  gold: 140
  skills:
    persuasion: 4
    intrigue: 3
inventory:
  - "[[iron-vault-key]]"
  - "Silver spyglass"
  - "Vial of belladonna"
---

# Lady Evelyn Vance

Evelyn is the second daughter of the Vance merchant dynasty. While outwardly managing shipping manifests, she covertly funds salvage expeditions into the sunken ruins.
```

## Supported Entity Types

- **`character`**: Player characters, NPCs, companions, and adversaries. Supports `voice`, `appearance`, and `inventory`.
- **`location`**: Towns, taverns, dungeons, starships, and regions. Can nest inside parent locations via `location: "[[parent-zone]]"`.
- **`faction`**: Guilds, secret societies, governments, and crews. Can track influence, reputation, and rivalries in `state`.
- **`item`**: Relics, weapons, spellbooks, and keys. Can be carried in entity `inventory` arrays.
- **`concept`**: Prophecies, historical events, cultural taboos, and magical phenomena.

## Graph Modeling with `[[Wikilinks]]`

LocalRPG automatically converts wikilinks into dynamic relationship edges in the campaign knowledge graph:

- **Frontmatter references**:
  ```yaml
  location: "[[sunken-spire]]"
  faction: "[[salvage-guild]]"
  ```
- **Prose references with labels**:
  ```markdown
  Evelyn was apprenticed to [[master-corvus|Arch-Mage Corvus]] before the fall.
  ```

Edges are bidirectional in search queries: when the player visits `[[sunken-spire]]`, the context assembler automatically loads all characters whose `location` points to `sunken-spire`.
```

- [x] **Step 11: Verify markdown files exist**

Run: `ls -la pkg/gui/docs/`
Expected: 10 files listed from `01-overview.md` to `10-codex-yaml.md`.

- [x] **Step 12: Commit Task 1**

```bash
git add pkg/gui/docs/
git commit -m "docs: add 10 embedded help and reference articles"
```

---

### Task 2: Backend Documentation Service, DTOs, and HTTP Handlers

Define DTOs in `pkg/gui/types.go`, embed and parse articles in `pkg/gui/docs.go`, register `/api/docs` and `/api/docs/{id}` in `pkg/gui/server.go`, and write unit tests in `pkg/gui/docs_test.go`.

**Files:**
- Modify: `pkg/gui/types.go`
- Create: `pkg/gui/docs.go`
- Modify: `pkg/gui/server.go`
- Test: `pkg/gui/docs_test.go`

- [x] **Step 1: Write failing unit test in `pkg/gui/docs_test.go`**

```go
package gui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDocsService_GetDocsList(t *testing.T) {
	svc := &Service{}
	docs, err := svc.GetDocsList(context.Background())
	if err != nil {
		t.Fatalf("GetDocsList failed: %v", err)
	}

	if len(docs) < 10 {
		t.Fatalf("expected at least 10 docs, got %d", len(docs))
	}

	// Verify first article is overview
	if docs[0].ID != "01-overview" {
		t.Errorf("expected first article to be 01-overview, got %s", docs[0].ID)
	}

	// Verify categories are populated
	foundCategories := make(map[string]bool)
	for _, doc := range docs {
		if doc.Title == "" {
			t.Errorf("doc %s missing title", doc.ID)
		}
		if doc.Category == "" {
			t.Errorf("doc %s missing category", doc.ID)
		}
		foundCategories[doc.Category] = true
	}

	expectedCategories := []string{
		"Core Concepts",
		"Configuration & Providers",
		"Studio Guides",
		"Codex & Content Reference",
	}
	for _, cat := range expectedCategories {
		if !foundCategories[cat] {
			t.Errorf("missing expected category %q", cat)
		}
	}
}

func TestDocsService_GetDocArticle(t *testing.T) {
	svc := &Service{}
	article, err := svc.GetDocArticle(context.Background(), "01-overview")
	if err != nil {
		t.Fatalf("GetDocArticle failed: %v", err)
	}

	if article.ID != "01-overview" {
		t.Errorf("expected id 01-overview, got %s", article.ID)
	}
	if article.Title != "Architecture & Core Concepts" {
		t.Errorf("unexpected title: %s", article.Title)
	}
	if len(article.Content) == 0 {
		t.Errorf("expected non-empty article content")
	}

	// Test non-existent article returns ErrDocNotFound
	_, err = svc.GetDocArticle(context.Background(), "non-existent-article")
	if err == nil {
		t.Errorf("expected error for non-existent article, got nil")
	}
}

func TestServer_DocsEndpoints(t *testing.T) {
	svc := &Service{}
	server := NewServer(svc, nil)

	// Test GET /api/docs
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/docs returned status %d", w.Code)
	}

	// Test GET /api/docs/01-overview
	req = httptest.NewRequest(http.MethodGet, "/api/docs/01-overview", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/docs/01-overview returned status %d", w.Code)
	}

	// Test GET /api/docs/not-found
	req = httptest.NewRequest(http.MethodGet, "/api/docs/non-existent-doc", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/docs/non-existent-doc expected 404, got %d", w.Code)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestDocs ./pkg/gui/`
Expected: FAIL (compilation errors: undefined `DocArticleSummaryDTO`, `GetDocsList`, etc.)

- [x] **Step 3: Add DTOs to `pkg/gui/types.go`**

Append to `pkg/gui/types.go`:

```go
// DocArticleSummaryDTO describes an article in documentation navigation and search.
type DocArticleSummaryDTO struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Order       int    `json:"order"`
	Description string `json:"description"`
}

// DocArticleDTO contains the full article content including markdown prose.
type DocArticleDTO struct {
	DocArticleSummaryDTO
	Content string `json:"content"`
}
```

- [x] **Step 4: Create `pkg/gui/docs.go`**

```go
package gui

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed docs/*.md
var embeddedDocsFS embed.FS

// ErrDocNotFound is returned when an article ID does not match any embedded guide.
var ErrDocNotFound = errors.New("document not found")

type docFrontmatter struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Category    string `yaml:"category"`
	Order       int    `yaml:"order"`
	Description string `yaml:"description"`
}

// parseDocFile reads frontmatter and body from markdown bytes.
func parseDocFile(data []byte) (*docFrontmatter, string, error) {
	content := string(data)
	normalized := strings.ReplaceAll(content, "\r\n", "\n")

	if !strings.HasPrefix(normalized, "---\n") {
		return nil, "", fmt.Errorf("missing frontmatter delimiter")
	}

	endIdx := strings.Index(normalized[4:], "\n---\n")
	if endIdx == -1 {
		return nil, "", fmt.Errorf("unclosed frontmatter delimiter")
	}

	fmRaw := normalized[4 : 4+endIdx]
	bodyRaw := strings.TrimSpace(normalized[4+endIdx+5:])

	var fm docFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return nil, "", fmt.Errorf("parse frontmatter yaml: %w", err)
	}

	return &fm, bodyRaw, nil
}

// GetDocsList returns metadata for all embedded documentation articles sorted by category and order.
func (s *Service) GetDocsList(_ context.Context) ([]DocArticleSummaryDTO, error) {
	entries, err := fs.ReadDir(embeddedDocsFS, "docs")
	if err != nil {
		return nil, fmt.Errorf("read embedded docs: %w", err)
	}

	var summaries []DocArticleSummaryDTO
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := embeddedDocsFS.ReadFile("docs/" + entry.Name())
		if err != nil {
			continue
		}

		fm, _, err := parseDocFile(data)
		if err != nil {
			continue
		}

		id := fm.ID
		if id == "" {
			id = strings.TrimSuffix(entry.Name(), ".md")
		}

		summaries = append(summaries, DocArticleSummaryDTO{
			ID:          id,
			Title:       fm.Title,
			Category:    fm.Category,
			Order:       fm.Order,
			Description: fm.Description,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Category == summaries[j].Category {
			return summaries[i].Order < summaries[j].Order
		}
		return summaries[i].Category < summaries[j].Category
	})

	return summaries, nil
}

// GetDocArticle retrieves an article by its ID.
func (s *Service) GetDocArticle(_ context.Context, id string) (*DocArticleDTO, error) {
	cleanID := strings.TrimSpace(id)
	cleanID = strings.TrimPrefix(cleanID, "/")
	cleanID = strings.TrimSuffix(cleanID, ".md")

	entries, err := fs.ReadDir(embeddedDocsFS, "docs")
	if err != nil {
		return nil, fmt.Errorf("read embedded docs: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := embeddedDocsFS.ReadFile("docs/" + entry.Name())
		if err != nil {
			continue
		}

		fm, body, err := parseDocFile(data)
		if err != nil {
			continue
		}

		docID := fm.ID
		if docID == "" {
			docID = strings.TrimSuffix(entry.Name(), ".md")
		}

		if docID == cleanID || strings.TrimSuffix(entry.Name(), ".md") == cleanID {
			return &DocArticleDTO{
				DocArticleSummaryDTO: DocArticleSummaryDTO{
					ID:          docID,
					Title:       fm.Title,
					Category:    fm.Category,
					Order:       fm.Order,
					Description: fm.Description,
				},
				Content: body,
			}, nil
		}
	}

	return nil, ErrDocNotFound
}
```

- [x] **Step 5: Register HTTP routes in `pkg/gui/server.go`**

In `pkg/gui/server.go`:
1. In `routeMetricLabel(path string) string`:
```go
	case strings.HasPrefix(path, "/api/docs"):
		return "/api/docs"
```
2. In `registerRoutes()`:
```go
	s.mux.HandleFunc("/api/docs", s.handleDocsRoutes)
	s.mux.HandleFunc("/api/docs/", s.handleDocsRoutes)
```
3. Add handler method:
```go
func (s *Server) handleDocsRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/docs")
	path = strings.Trim(path, "/")

	if path == "" {
		docs, err := s.service.GetDocsList(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, docs)
		return
	}

	article, err := s.service.GetDocArticle(r.Context(), path)
	if err != nil {
		if errors.Is(err, ErrDocNotFound) {
			http.Error(w, "document not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, article)
}
```

- [x] **Step 6: Run tests to verify they pass**

Run: `go test -v -run TestDocs ./pkg/gui/`
Expected: PASS for all tests.

- [x] **Step 7: Run `go vet ./...` to verify clean linter**

Run: `go vet ./pkg/gui/...`
Expected: Clean exit code 0.

- [x] **Step 8: Commit Task 2**

```bash
git add pkg/gui/types.go pkg/gui/docs.go pkg/gui/server.go pkg/gui/docs_test.go
git commit -m "feat(gui): implement embedded docs service and HTTP endpoints"
```

---

### Task 3: Frontend Types and API Client Methods

Expose TypeScript types and client fetchers for documentation.

**Files:**
- Modify: `frontend/src/types.ts`
- Modify: `frontend/src/api/client.ts`

- [x] **Step 1: Add types to `frontend/src/types.ts`**

Add to `frontend/src/types.ts`:

```typescript
export interface DocArticleSummary {
  id: string;
  title: string;
  category: string;
  order: number;
  description: string;
}

export interface DocArticle extends DocArticleSummary {
  content: string;
}
```

- [x] **Step 2: Add API methods to `frontend/src/api/client.ts`**

Import `DocArticleSummary` and `DocArticle` from `../types`.
In `APIClient` class, add static and instance methods:

```typescript
  static async getDocsList(): Promise<DocArticleSummary[]> {
    const res = await fetch('/api/docs');
    if (!res.ok) throw new HTTPError(res.status, `getDocsList: ${res.statusText}`);
    return res.json();
  }

  static async getDocArticle(id: string): Promise<DocArticle> {
    const res = await fetch(`/api/docs/${encodeURIComponent(id)}`);
    if (!res.ok) throw new HTTPError(res.status, `getDocArticle: ${res.statusText}`);
    return res.json();
  }
```

- [x] **Step 3: Verify TypeScript compilation**

Run: `npx tsc --noEmit`
Expected: PASS with 0 errors.

- [x] **Step 4: Commit Task 3**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): add documentation types and APIClient methods"
```

---

### Task 4: Markdown Document Viewer Component

Create `frontend/src/components/MarkdownDocViewer.tsx` to render rich documentation markdown:
- Headings with hierarchy styling.
- Fenced code blocks with language badge and one-click "Copy" button with checkmark confirmation.
- GitHub-style callouts (`> [!NOTE]`, `> [!TIP]`, `> [!IMPORTANT]`, `> [!WARNING]`).
- Tables, blockquotes, bulleted and numbered lists, inline code, bold, italic.

**Files:**
- Create: `frontend/src/components/MarkdownDocViewer.tsx`

- [x] **Step 1: Create `frontend/src/components/MarkdownDocViewer.tsx`**

```tsx
import React, { memo, useState } from 'react';
import { Copy, Check, Info, Lightbulb, AlertTriangle, AlertCircle } from 'lucide-react';

interface MarkdownDocViewerProps {
  content: string;
  className?: string;
}

interface CodeBlockProps {
  code: string;
  language?: string;
}

const CodeBlock: React.FC<CodeBlockProps> = ({ code, language }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy code:', err);
    }
  };

  return (
    <div className="my-4 rounded-xl overflow-hidden border border-white/10 bg-black/60 shadow-lg">
      <div className="flex items-center justify-between px-4 py-1.5 bg-white/[0.04] border-b border-white/10 text-xs font-mono text-stone-400">
        <span className="uppercase tracking-wider font-semibold">{language || 'text'}</span>
        <button
          onClick={handleCopy}
          className="flex items-center gap-1.5 px-2 py-1 rounded-md text-stone-300 hover:text-white hover:bg-white/10 transition-all cursor-pointer"
          title="Copy code to clipboard"
        >
          {copied ? (
            <>
              <Check className="w-3.5 h-3.5 text-emerald-400" />
              <span className="text-emerald-400 font-sans">Copied!</span>
            </>
          ) : (
            <>
              <Copy className="w-3.5 h-3.5" />
              <span className="font-sans">Copy</span>
            </>
          )}
        </button>
      </div>
      <pre className="p-4 overflow-x-auto text-xs sm:text-sm font-mono text-stone-200 leading-relaxed selection:bg-purple-500/30">
        <code>{code}</code>
      </pre>
    </div>
  );
};

const inlinePattern = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*]+\*)|(_[^_]+_)|(\[[^\]]+\]\([^)]+\))/g;

const renderInlineProse = (text: string): React.ReactNode[] => {
  const nodes: React.ReactNode[] = [];
  let lastIndex = 0;
  let key = 0;
  let match: RegExpExecArray | null;

  inlinePattern.lastIndex = 0;
  while ((match = inlinePattern.exec(text)) !== null) {
    if (match.index > lastIndex) {
      nodes.push(text.slice(lastIndex, match.index));
    }

    const token = match[0];
    if (token.startsWith('`')) {
      nodes.push(
        <code key={key++} className="px-1.5 py-0.5 rounded bg-white/[0.08] border border-white/10 font-mono text-[0.88em] text-purple-300">
          {token.slice(1, -1)}
        </code>
      );
    } else if (token.startsWith('**')) {
      nodes.push(
        <strong key={key++} className="font-semibold text-white">
          {token.slice(2, -2)}
        </strong>
      );
    } else if (token.startsWith('*') || token.startsWith('_')) {
      nodes.push(
        <em key={key++} className="italic text-stone-300">
          {token.slice(1, -1)}
        </em>
      );
    } else if (token.startsWith('[')) {
      const linkMatch = token.match(/\[([^\]]+)\]\(([^)]+)\)/);
      if (linkMatch) {
        nodes.push(
          <a
            key={key++}
            href={linkMatch[2]}
            target="_blank"
            rel="noopener noreferrer"
            className="text-purple-400 hover:text-purple-300 underline font-medium"
          >
            {linkMatch[1]}
          </a>
        );
      } else {
        nodes.push(token);
      }
    }

    lastIndex = match.index + token.length;
  }

  if (lastIndex < text.length) {
    nodes.push(text.slice(lastIndex));
  }
  return nodes;
};

export const MarkdownDocViewer: React.FC<MarkdownDocViewerProps> = memo(({ content, className = '' }) => {
  const normalized = (content ?? '').replace(/\r\n/g, '\n');
  if (!normalized.trim()) return null;

  // Split content by fenced code blocks first
  const parts = normalized.split(/(```[\s\S]*?```)/g);

  return (
    <div className={`space-y-4 text-stone-300 font-sans leading-relaxed text-sm sm:text-base ${className}`}>
      {parts.map((part, partIndex) => {
        if (part.startsWith('```') && part.endsWith('```')) {
          const firstLineEnd = part.indexOf('\n');
          const language = part.slice(3, firstLineEnd).trim();
          const code = part.slice(firstLineEnd + 1, -3);
          return <CodeBlock key={partIndex} code={code} language={language} />;
        }

        // Process standard markdown blocks
        const blocks = part.split(/\n{2,}/);
        return blocks.map((block, blockIndex) => {
          const trimmed = block.trim();
          if (!trimmed) return null;

          // Headings
          if (trimmed.startsWith('# ')) {
            return (
              <h1 key={`${partIndex}-${blockIndex}`} className="text-2xl sm:text-3xl font-bold text-white tracking-tight pt-4 pb-2 border-b border-white/10">
                {renderInlineProse(trimmed.slice(2))}
              </h1>
            );
          }
          if (trimmed.startsWith('## ')) {
            return (
              <h2 key={`${partIndex}-${blockIndex}`} className="text-xl sm:text-2xl font-bold text-purple-300 tracking-tight pt-4 pb-1">
                {renderInlineProse(trimmed.slice(3))}
              </h2>
            );
          }
          if (trimmed.startsWith('### ')) {
            return (
              <h3 key={`${partIndex}-${blockIndex}`} className="text-lg font-semibold text-stone-100 tracking-tight pt-2">
                {renderInlineProse(trimmed.slice(4))}
              </h3>
            );
          }
          if (trimmed.startsWith('#### ')) {
            return (
              <h4 key={`${partIndex}-${blockIndex}`} className="text-base font-semibold text-purple-200/90 pt-1">
                {renderInlineProse(trimmed.slice(5))}
              </h4>
            );
          }

          // Horizontal rule
          if (/^(-{3,}|\*{3,}|_{3,})$/.test(trimmed)) {
            return <hr key={`${partIndex}-${blockIndex}`} className="my-6 border-white/10" />;
          }

          // Callouts: > [!NOTE], > [!TIP], > [!IMPORTANT], > [!WARNING]
          if (trimmed.startsWith('>')) {
            const lines = trimmed.split('\n').map((l) => l.replace(/^>\s?/, ''));
            const header = lines[0]?.trim() || '';

            let calloutType: 'note' | 'tip' | 'important' | 'warning' | null = null;
            let title = '';
            let bodyLines = lines;

            if (header.startsWith('[!NOTE]')) {
              calloutType = 'note';
              title = 'Note';
              bodyLines = lines.slice(1);
            } else if (header.startsWith('[!TIP]')) {
              calloutType = 'tip';
              title = 'Tip';
              bodyLines = lines.slice(1);
            } else if (header.startsWith('[!IMPORTANT]')) {
              calloutType = 'important';
              title = 'Important';
              bodyLines = lines.slice(1);
            } else if (header.startsWith('[!WARNING]')) {
              calloutType = 'warning';
              title = 'Warning';
              bodyLines = lines.slice(1);
            }

            if (calloutType) {
              const styles = {
                note: { border: 'border-blue-500/40', bg: 'bg-blue-950/20', text: 'text-blue-300', icon: Info },
                tip: { border: 'border-emerald-500/40', bg: 'bg-emerald-950/20', text: 'text-emerald-300', icon: Lightbulb },
                important: { border: 'border-purple-500/40', bg: 'bg-purple-950/20', text: 'text-purple-300', icon: AlertCircle },
                warning: { border: 'border-amber-500/40', bg: 'bg-amber-950/20', text: 'text-amber-300', icon: AlertTriangle },
              }[calloutType];
              const IconComponent = styles.icon;

              return (
                <div key={`${partIndex}-${blockIndex}`} className={`my-4 p-4 rounded-xl border ${styles.border} ${styles.bg}`}>
                  <div className={`flex items-center gap-2 font-semibold text-sm ${styles.text} mb-1.5`}>
                    <IconComponent className="w-4 h-4" />
                    <span>{title}</span>
                  </div>
                  <div className="text-sm text-stone-300 space-y-1">
                    {bodyLines.map((line, li) => (
                      <p key={li}>{renderInlineProse(line)}</p>
                    ))}
                  </div>
                </div>
              );
            }

            // Standard blockquote
            return (
              <blockquote key={`${partIndex}-${blockIndex}`} className="border-l-2 border-purple-500/50 pl-4 py-1 italic text-stone-300">
                {lines.map((l, li) => (
                  <p key={li}>{renderInlineProse(l)}</p>
                ))}
              </blockquote>
            );
          }

          // Tables
          const lines = trimmed.split('\n');
          if (lines.length >= 2 && lines[0].includes('|') && lines[1].includes('|') && lines[1].includes('-')) {
            const headerCells = lines[0].split('|').map((c) => c.trim()).filter(Boolean);
            const rowLines = lines.slice(2);

            return (
              <div key={`${partIndex}-${blockIndex}`} className="my-4 overflow-x-auto rounded-xl border border-white/10 bg-black/40">
                <table className="w-full text-left border-collapse text-xs sm:text-sm">
                  <thead>
                    <tr className="border-b border-white/10 bg-white/[0.04]">
                      {headerCells.map((cell, ci) => (
                        <th key={ci} className="py-2.5 px-4 font-semibold text-stone-200">
                          {renderInlineProse(cell)}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {rowLines.map((row, ri) => {
                      const cells = row.split('|').map((c) => c.trim()).filter(Boolean);
                      return (
                        <tr key={ri} className="border-b border-white/5 hover:bg-white/[0.02]">
                          {cells.map((cell, ci) => (
                            <td key={ci} className="py-2.5 px-4 text-stone-300">
                              {renderInlineProse(cell)}
                            </td>
                          ))}
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            );
          }

          // Bulleted list
          if (lines.every((line) => /^\s*[-*]\s+/.test(line))) {
            return (
              <ul key={`${partIndex}-${blockIndex}`} className="list-disc list-outside ml-6 space-y-1 text-stone-300">
                {lines.map((line, li) => (
                  <li key={li}>{renderInlineProse(line.replace(/^\s*[-*]\s+/, ''))}</li>
                ))}
              </ul>
            );
          }

          // Numbered list
          if (lines.every((line) => /^\s*\d+\.\s+/.test(line))) {
            return (
              <ol key={`${partIndex}-${blockIndex}`} className="list-decimal list-outside ml-6 space-y-1 text-stone-300">
                {lines.map((line, li) => (
                  <li key={li}>{renderInlineProse(line.replace(/^\s*\d+\.\s+/, ''))}</li>
                ))}
              </ol>
            );
          }

          // Regular paragraph
          return (
            <p key={`${partIndex}-${blockIndex}`} className="leading-relaxed">
              {renderInlineProse(trimmed)}
            </p>
          );
        });
      })}
    </div>
  );
});

MarkdownDocViewer.displayName = 'MarkdownDocViewer';
```

- [x] **Step 2: Verify TypeScript compilation**

Run: `npx tsc --noEmit`
Expected: PASS with 0 errors.

- [x] **Step 3: Commit Task 4**

```bash
git add frontend/src/components/MarkdownDocViewer.tsx
git commit -m "feat(frontend): create MarkdownDocViewer with code blocks and callouts"
```

---

### Task 5: Documentation Modal Component (`DocsModal.tsx`)

Build the two-pane studio modal featuring:
- Search bar with live keyword filtering across title, description, category, and article content.
- Left sidebar with categorized tree navigation and article count badges.
- Right reader pane with breadcrumb bar, full prose rendering, and Previous/Next buttons.
- Deep-linking via `initialArticleID?: string`.
- Escape key listener and backdrop dismiss.

**Files:**
- Create: `frontend/src/components/DocsModal.tsx`

- [x] **Step 1: Create `frontend/src/components/DocsModal.tsx`**

```tsx
import React, { useState, useEffect, useMemo } from 'react';
import {
  X,
  Search,
  BookOpen,
  Folder,
  ChevronRight,
  ArrowLeft,
  ArrowRight,
  Compass,
  Sliders,
  Wrench,
  FileCode,
} from 'lucide-react';
import { APIClient } from '../api/client';
import { DocArticleSummary, DocArticle } from '../types';
import { MarkdownDocViewer } from './MarkdownDocViewer';

interface DocsModalProps {
  isOpen: boolean;
  onClose: () => void;
  initialArticleID?: string;
}

const CATEGORY_ICONS: Record<string, React.FC<{ className?: string }>> = {
  'Core Concepts': Compass,
  'Configuration & Providers': Sliders,
  'Studio Guides': Wrench,
  'Codex & Content Reference': FileCode,
};

export const DocsModal: React.FC<DocsModalProps> = ({
  isOpen,
  onClose,
  initialArticleID,
}) => {
  const [summaries, setSummaries] = useState<DocArticleSummary[]>([]);
  const [selectedArticleID, setSelectedArticleID] = useState<string>('');
  const [currentArticle, setCurrentArticle] = useState<DocArticle | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Load article summaries list on open
  useEffect(() => {
    if (!isOpen) return;

    let isMounted = true;
    APIClient.getDocsList()
      .then((list) => {
        if (!isMounted) return;
        setSummaries(list);

        const targetID =
          initialArticleID && list.some((a) => a.id === initialArticleID)
            ? initialArticleID
            : list[0]?.id || '';

        setSelectedArticleID(targetID);
      })
      .catch((err) => {
        if (!isMounted) return;
        console.error('Failed to load docs list:', err);
        setError('Failed to load documentation catalogue.');
      });

    return () => {
      isMounted = false;
    };
  }, [isOpen, initialArticleID]);

  // Load selected article content
  useEffect(() => {
    if (!selectedArticleID || !isOpen) return;

    let isMounted = true;
    setIsLoading(true);
    setError(null);

    APIClient.getDocArticle(selectedArticleID)
      .then((article) => {
        if (!isMounted) return;
        setCurrentArticle(article);
        setIsLoading(false);
      })
      .catch((err) => {
        if (!isMounted) return;
        console.error(`Failed to load doc ${selectedArticleID}:`, err);
        setError('Failed to load documentation article.');
        setIsLoading(false);
      });

    return () => {
      isMounted = false;
    };
  }, [selectedArticleID, isOpen]);

  // Keyboard shortcut: Escape to close
  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  // Grouped categories
  const categories = useMemo(() => {
    const map = new Map<string, DocArticleSummary[]>();
    for (const doc of summaries) {
      const cat = doc.category || 'General';
      if (!map.has(cat)) {
        map.set(cat, []);
      }
      map.get(cat)!.push(doc);
    }
    return Array.from(map.entries()).map(([name, articles]) => ({
      name,
      articles: articles.sort((a, b) => a.order - b.order),
    }));
  }, [summaries]);

  // Filtered articles when searching
  const filteredArticles = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return null;
    return summaries.filter(
      (a) =>
        a.title.toLowerCase().includes(q) ||
        a.description.toLowerCase().includes(q) ||
        a.category.toLowerCase().includes(q) ||
        a.id.toLowerCase().includes(q)
    );
  }, [summaries, searchQuery]);

  // Previous and Next article navigation
  const currentIndex = summaries.findIndex((a) => a.id === selectedArticleID);
  const prevArticle = currentIndex > 0 ? summaries[currentIndex - 1] : null;
  const nextArticle =
    currentIndex >= 0 && currentIndex < summaries.length - 1
      ? summaries[currentIndex + 1]
      : null;

  if (!isOpen) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Documentation & Reference"
      className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-6 bg-black/80 backdrop-blur-md anim-fade-in"
    >
      <div className="relative w-full max-w-6xl h-[90vh] bg-stone-900/95 border border-white/15 rounded-3xl overflow-hidden shadow-2xl flex flex-col anim-scale-in">
        {/* Top Header */}
        <div className="p-4 px-6 border-b border-white/10 flex items-center justify-between bg-stone-950/70 gap-4">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-xl bg-purple-950/60 border border-purple-500/30 flex items-center justify-center text-purple-400 shadow-md">
              <BookOpen className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-base font-sans font-bold text-white tracking-tight">
                Documentation & Reference
              </h2>
              <p className="text-xs text-stone-400 hidden sm:block">
                Guides, architecture, provider setup, and codex schemas
              </p>
            </div>
          </div>

          {/* Quick Search */}
          <div className="relative flex-1 max-w-md mx-4">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-stone-400 pointer-events-none" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search guides, rules, syntax..."
              className="w-full bg-white/[0.05] border border-white/10 rounded-xl pl-9 pr-8 py-1.5 text-xs sm:text-sm text-stone-200 placeholder-stone-500 focus:outline-none focus:border-purple-500/50 focus:ring-1 focus:ring-purple-500/30 transition-all"
            />
            {searchQuery && (
              <button
                onClick={() => setSearchQuery('')}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-stone-400 hover:text-white text-xs"
              >
                ✕
              </button>
            )}
          </div>

          {/* Close button */}
          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 flex items-center justify-center text-stone-400 hover:text-white transition-all cursor-pointer"
            title="Close (Escape)"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Two-Pane Body */}
        <div className="flex-1 flex min-h-0 overflow-hidden">
          {/* Left Navigation Sidebar */}
          <aside className="w-64 sm:w-80 border-r border-white/10 bg-stone-950/40 flex flex-col min-h-0">
            <div className="flex-1 overflow-y-auto p-3 space-y-4">
              {filteredArticles ? (
                // Search Results View
                <div className="space-y-1">
                  <div className="text-[11px] font-sans font-semibold uppercase tracking-wider text-purple-400 px-3 py-1">
                    Search Results ({filteredArticles.length})
                  </div>
                  {filteredArticles.length === 0 ? (
                    <div className="text-xs text-stone-500 px-3 py-4 text-center">
                      No matching articles found.
                    </div>
                  ) : (
                    filteredArticles.map((article) => {
                      const isSelected = article.id === selectedArticleID;
                      return (
                        <button
                          key={article.id}
                          onClick={() => {
                            setSelectedArticleID(article.id);
                          }}
                          className={`w-full text-left px-3 py-2 rounded-xl transition-all flex flex-col gap-0.5 cursor-pointer ${
                            isSelected
                              ? 'bg-purple-600/30 border border-purple-500/50 text-white shadow-sm'
                              : 'text-stone-300 hover:bg-white/[0.05] hover:text-white'
                          }`}
                        >
                          <div className="text-xs font-semibold">{article.title}</div>
                          <div className="text-[11px] text-stone-400 line-clamp-1">
                            {article.description}
                          </div>
                        </button>
                      );
                    })
                  )}
                </div>
              ) : (
                // Categorized Tree View
                categories.map((category) => {
                  const Icon = CATEGORY_ICONS[category.name] || Folder;
                  return (
                    <div key={category.name} className="space-y-1">
                      <div className="flex items-center gap-2 px-3 py-1 text-[11px] font-sans font-semibold uppercase tracking-wider text-stone-400">
                        <Icon className="w-3.5 h-3.5 text-purple-400" />
                        <span>{category.name}</span>
                        <span className="ml-auto text-[10px] text-stone-500">
                          {category.articles.length}
                        </span>
                      </div>
                      <div className="space-y-0.5">
                        {category.articles.map((article) => {
                          const isSelected = article.id === selectedArticleID;
                          return (
                            <button
                              key={article.id}
                              onClick={() => setSelectedArticleID(article.id)}
                              className={`w-full text-left px-3 py-2 rounded-xl transition-all flex items-center justify-between gap-2 cursor-pointer ${
                                isSelected
                                  ? 'bg-purple-600/30 border border-purple-500/50 text-white font-medium shadow-sm'
                                  : 'text-stone-400 hover:bg-white/[0.04] hover:text-stone-200'
                              }`}
                            >
                              <span className="text-xs truncate">{article.title}</span>
                              {isSelected && (
                                <ChevronRight className="w-3.5 h-3.5 text-purple-400 shrink-0" />
                              )}
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </aside>

          {/* Right Reader Content */}
          <main className="flex-1 flex flex-col min-h-0 bg-stone-900/60 overflow-hidden">
            {/* Breadcrumb Navigation */}
            {currentArticle && (
              <div className="px-6 py-2.5 border-b border-white/10 bg-stone-950/30 flex items-center gap-2 text-xs text-stone-400 shrink-0">
                <span className="hover:text-stone-200">Documentation</span>
                <ChevronRight className="w-3 h-3 text-stone-600" />
                <span className="text-purple-400 font-medium">
                  {currentArticle.category}
                </span>
                <ChevronRight className="w-3 h-3 text-stone-600" />
                <span className="text-stone-200 truncate">{currentArticle.title}</span>
              </div>
            )}

            {/* Scrollable Document Body */}
            <div className="flex-1 overflow-y-auto p-6 sm:p-8 space-y-6">
              {isLoading ? (
                <div className="flex items-center justify-center h-48 text-stone-400 text-sm">
                  Loading documentation...
                </div>
              ) : error ? (
                <div className="p-4 rounded-xl border border-red-500/30 bg-red-950/20 text-red-300 text-sm">
                  {error}
                </div>
              ) : currentArticle ? (
                <>
                  <div className="mb-6">
                    <p className="text-sm text-stone-400 mt-1 italic">
                      {currentArticle.description}
                    </p>
                  </div>
                  <MarkdownDocViewer content={currentArticle.content} />
                </>
              ) : (
                <div className="text-stone-500 text-center py-12">
                  Select an article from the sidebar to begin reading.
                </div>
              )}

              {/* Prev / Next Article Footer */}
              {!isLoading && currentArticle && (prevArticle || nextArticle) && (
                <div className="pt-8 mt-8 border-t border-white/10 flex items-center justify-between gap-4">
                  {prevArticle ? (
                    <button
                      onClick={() => setSelectedArticleID(prevArticle.id)}
                      className="flex items-center gap-2 px-4 py-2.5 rounded-xl border border-white/10 bg-white/[0.04] hover:bg-white/[0.08] text-stone-300 hover:text-white text-xs transition-all cursor-pointer"
                    >
                      <ArrowLeft className="w-3.5 h-3.5 text-purple-400" />
                      <div className="text-left">
                        <div className="text-[10px] text-stone-500 uppercase font-semibold">
                          Previous
                        </div>
                        <div className="font-medium text-stone-200 truncate max-w-[180px]">
                          {prevArticle.title}
                        </div>
                      </div>
                    </button>
                  ) : (
                    <div />
                  )}

                  {nextArticle && (
                    <button
                      onClick={() => setSelectedArticleID(nextArticle.id)}
                      className="flex items-center gap-2 px-4 py-2.5 rounded-xl border border-white/10 bg-white/[0.04] hover:bg-white/[0.08] text-stone-300 hover:text-white text-xs transition-all cursor-pointer ml-auto"
                    >
                      <div className="text-right">
                        <div className="text-[10px] text-stone-500 uppercase font-semibold">
                          Next
                        </div>
                        <div className="font-medium text-stone-200 truncate max-w-[180px]">
                          {nextArticle.title}
                        </div>
                      </div>
                      <ArrowRight className="w-3.5 h-3.5 text-purple-400" />
                    </button>
                  )}
                </div>
              )}
            </div>
          </main>
        </div>
      </div>
    </div>
  );
};
```

- [x] **Step 2: Verify TypeScript compilation**

Run: `npx tsc --noEmit`
Expected: PASS with 0 errors.

- [x] **Step 3: Commit Task 5**

```bash
git add frontend/src/components/DocsModal.tsx
git commit -m "feat(frontend): create DocsModal with category tree, search, and reader"
```

---

### Task 6: Integration into Launcher Dock, Launcher Hub, and Chronicle Header

Wire entry points:
1. `LauncherDock.tsx`: Help/Docs button at bottom left of dock.
2. `LauncherHub.tsx`: Connect `onOpenDocs`, mount `DocsModal`.
3. `App.tsx`: Header Docs button next to Settings, mount `DocsModal`.

**Files:**
- Modify: `frontend/src/components/launcher/LauncherDock.tsx`
- Modify: `frontend/src/components/LauncherHub.tsx`
- Modify: `frontend/src/App.tsx`

- [x] **Step 1: Add `onOpenDocs` to `LauncherDock.tsx`**

In `frontend/src/components/launcher/LauncherDock.tsx`:
1. Import `HelpCircle` from `lucide-react`.
2. Add `onOpenDocs?: () => void;` to `LauncherDockProps`.
3. Add the button after the Settings group (inside `div.flex.flex-col.items-center.gap-2.5.pt-3.border-t.border-white/10`):

```tsx
        <div className="relative group">
          <button
            onClick={onOpenDocs}
            className="w-10 h-10 rounded-xl flex items-center justify-center text-stone-400 hover:text-white hover:bg-white/[0.08] transition-all cursor-pointer"
            title="Help & Documentation"
            aria-label="Help & Documentation"
          >
            <HelpCircle className="w-5 h-5" />
          </button>
          <div className="pointer-events-none absolute left-[64px] top-1/2 -translate-y-1/2 px-2.5 py-1 bg-stone-900 border border-white/15 rounded-lg text-xs font-sans text-stone-200 whitespace-nowrap shadow-xl opacity-0 group-hover:opacity-100 transition-opacity z-50">
            Help & Documentation
          </div>
        </div>
```

- [x] **Step 2: Connect `DocsModal` in `LauncherHub.tsx`**

In `frontend/src/components/LauncherHub.tsx`:
1. Import `DocsModal` from `./DocsModal`.
2. Add state `const [isDocsOpen, setIsDocsOpen] = useState(false);`
3. Pass `onOpenDocs={() => setIsDocsOpen(true)}` to `<LauncherDock />`.
4. Render `<DocsModal isOpen={isDocsOpen} onClose={() => setIsDocsOpen(false)} />` at the bottom of `LauncherHub`.

- [x] **Step 3: Connect `DocsModal` in `App.tsx`**

In `frontend/src/App.tsx`:
1. Import `HelpCircle` from `lucide-react`.
2. Import `DocsModal` from `./components/DocsModal`.
3. Add state `const [isDocsOpen, setIsDocsOpen] = useState(false);`
4. Add header button after `Settings` button:

```tsx
              <button
                onClick={() => setIsDocsOpen(true)}
                className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  isDocsOpen ? 'bg-purple-600 text-white font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
                title="Help & Documentation"
              >
                <HelpCircle className="w-3.5 h-3.5 text-purple-400" />
                <span className="hidden sm:inline">Docs</span>
              </button>
```

5. Render `<DocsModal isOpen={isDocsOpen} onClose={() => setIsDocsOpen(false)} />` alongside other modals at the root of `App.tsx`.

- [x] **Step 4: Verify TypeScript compilation**

Run: `npx tsc --noEmit`
Expected: PASS with 0 errors.

- [x] **Step 5: Commit Task 6**

```bash
git add frontend/src/components/launcher/LauncherDock.tsx frontend/src/components/LauncherHub.tsx frontend/src/App.tsx
git commit -m "feat(frontend): integrate documentation modal into launcher dock and chronicle header"
```

---

### Task 7: Full System Verification

Verify backend unit tests, frontend builds, embedding integrity, and linter gates.

**Files:** None (verification commands)

- [x] **Step 1: Run backend tests**

Run: `go test -v -count=1 ./pkg/gui/...`
Expected: PASS with 0 failures.

- [x] **Step 2: Run all backend tests**

Run: `go test -v -count=1 ./...`
Expected: PASS with 0 failures across all packages.

- [x] **Step 3: Run backend linter**

Run: `go vet ./...`
Expected: Clean exit code 0.

- [x] **Step 4: Run frontend type checks**

Run: `npx tsc --noEmit`
Expected: Clean exit code 0.

- [x] **Step 5: Run full frontend and backend build**

Run: `mise run build`
Expected: Vite build succeeds, touches `pkg/gui/dist/.gitkeep`, Go binary builds `bin/localrpg` cleanly with embedded docs.

- [x] **Step 6: Commit and tag if necessary**

```bash
git status
```
Expected: Clean working tree.
