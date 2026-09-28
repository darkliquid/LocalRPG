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

```text
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
