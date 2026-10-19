---
id: 00-start-here
title: Start Here
category: Start Here
order: 0
description: What LocalRPG is, and the order to read the guides in.
---

# Start Here

LocalRPG is a local-first, turn-based tabletop RPG client: one Go binary that serves a
desktop app, a terminal client and an HTTP API over the same engine. It doesn't need an
account or a network connection, and nothing about a campaign is hidden behind a service.

The engine is schema-agnostic. No hit points, mana or character classes are built in;
systems and worlds are files you author, and the app reads them. Read that first, because
it explains why the guides spend so long on file formats.

## Where to begin

- **[Architecture & Core Concepts](01-overview.md)** - Schema-agnostic design, three-tier separation, and how AI agents drive tabletop adventures.
- **[Campaigns & Turns](04-campaigns.md)** - The turn lifecycle, canonical history log, non-destructive rewinding, and living-world progression.
- **[Building Worlds Studio Guide](08-worlds-studio.md)** - Walkthrough for creating custom worlds, defining lore prompts, and authoring starter entities.
- **[Building Custom Systems Guide](09-systems-studio.md)** - Authoring system rules prompts, coding mechanics.js hooks, and testing dice expressions.
- **[AI & Media Providers](05-providers.md)** - Configuring LLMs, voice synthesis, speech recognition, image generation, and spend tracking.
- **[Local-First and Offline Play](21-local-first.md)** - What runs locally in LocalRPG, what hardware each tier requires, and how to configure a zero-network tabletop stack.

## Every guide

Grouped as the app's own documentation viewer groups them: one section per category, in
the order the categories first appear.

### Core Concepts

- **[Architecture & Core Concepts](01-overview.md)** - Schema-agnostic design, three-tier separation, and how AI agents drive tabletop adventures.
- **[Worlds & Lore](02-worlds.md)** - Anatomy of a world, starting location resolution, and system overrides.
- **[Systems & Mechanics](03-systems.md)** - Rules prompts, action modes, dice expressions, and the sandboxed JavaScript mechanics engine.
- **[Campaigns & Turns](04-campaigns.md)** - The turn lifecycle, canonical history log, non-destructive rewinding, and living-world progression.

### Configuration & Providers

- **[AI & Media Providers](05-providers.md)** - Configuring LLMs, voice synthesis, speech recognition, image generation, and spend tracking.
- **[AI Agents & Roles](06-agents.md)** - Assigning models to GM, Narrator, and Extractor roles, plus token budgets and fallback chains.
- **[Storage & Directories](07-storage-paths.md)** - Standard XDG directories, workspace mode, and disposable SQLite caching.
- **[Usage, Cost & Pricing](11-usage-and-pricing.md)** - Reading the spend ledger and setting the provider prices that turn usage into a cost figure.
- **[Provider & Model Catalogue](12-provider-catalogue.md)** - Every registered provider ID, its presets and models, and the ledger key that prices match against.
- **[Configuration Reference](13-configuration-reference.md)** - Every key LocalRPG reads from config.yaml, with the accepted values for the enumerated ones.
- **[Style Packs](24-style-packs.md)** - Theme the procedural art and the app chrome with a pack, or keep the built-in look.
- **[Trace & Telemetry](29-trace-and-telemetry.md)** - Record what a turn did in a local JSONL trace, or export spans and metrics to an OpenTelemetry collector.

### Studio Guides

- **[Building Worlds Studio Guide](08-worlds-studio.md)** - Walkthrough for creating custom worlds, defining lore prompts, and authoring starter entities.
- **[Building Custom Systems Guide](09-systems-studio.md)** - Authoring system rules prompts, coding mechanics.js hooks, and testing dice expressions.
- **[AI World Generation](23-ai-world-generation.md)** - Draft a world from a sentence, a folder, or a set of pages, then review it before it is committed.

### Codex & Content Reference

- **[Codex YAML Frontmatter & Graph Reference](10-codex-yaml.md)** - Complete YAML frontmatter schema, entity types, custom state, and wikilink graph modeling.
- **[Editing Content Outside the App](22-editing-content.md)** - The on-disk and package formats as the interchange contract, so systems, worlds, and campaigns can be edited in any editor.

### Local AI & Self-Hosting

- **[Setting Up Ollama for Local LLMs](14-local-llm-ollama.md)** - Hardware requirements, native/Docker setup, recommended models for tabletop RPGs, and connecting Ollama to LocalRPG.
- **[Setting Up Kokoro-FastAPI for Neural TTS](15-local-tts-kokoro.md)** - Running Kokoro-FastAPI via Docker (CPU/GPU), voice profile selection, and connecting to LocalRPG.
- **[Setting Up Fish Audio S2 with vLLM-Omni](16-local-tts-fish-audio.md)** - GPU requirements, Docker/vLLM-Omni container setup, voice cloning, and how emotion tags steer the performance.
- **[Setting Up Faster-Whisper for Speech Recognition](17-local-stt-whisper.md)** - OpenAI-compatible speech-to-text server setup via Docker, hardware considerations, and LocalRPG voice input configuration.
- **[Setting Up ComfyUI for Local Image Generation](18-local-image-comfyui.md)** - ComfyUI API configuration, model setup (SDXL/Flux/SD1.5), and connecting LocalRPG for scene and entity illustration.
- **[Fully Local AI Stack with Docker Compose](19-local-stack-docker-compose.md)** - One Docker Compose project that runs Ollama, Kokoro, Faster-Whisper, and ComfyUI together, with recommended models, system requirements, and the matching LocalRPG configuration.
- **[Local Semantic Search with a Built-in Encoder](20-local-embeddings.md)** - How LocalRPG turns entities and turns into searchable vectors, why the default projection ranks unrelated words together, and how to switch on the built-in BGE encoder that separates them.
- **[Local-First and Offline Play](21-local-first.md)** - What runs locally in LocalRPG, what hardware each tier requires, and how to configure a zero-network tabletop stack.

### Command Line & Terminal

- **[Command-Line Reference](25-cli-reference.md)** - Every command, its flags, what it prints, and the exit code it leaves behind.
- **[The Terminal Client](26-terminal-client.md)** - The keys, action modes, and slash commands for playing a campaign in a terminal.

### Using the App

- **[The Desktop App](27-desktop-app.md)** - A tour of the desktop window, from the launcher to the campaign screen, its drawers, and the settings panels.
- **[Story Theatre, Audio & Portraits](28-story-theatre.md)** - Replay a campaign as a visual novel, control the spoken lines, and give characters portraits.
