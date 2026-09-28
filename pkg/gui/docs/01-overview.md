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
