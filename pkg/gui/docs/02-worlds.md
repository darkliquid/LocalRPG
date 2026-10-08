---
id: 02-worlds
title: Worlds & Lore
category: Core Concepts
order: 2
description: Anatomy of a world, starting location resolution, and system overrides.
---

# Worlds & Lore

A **World** defines the setting, lore, aesthetics, and starting conditions for campaigns. Worlds are modular and can be paired with any game system.

## Anatomy of a World

Each world directory (`worlds/<id>/`) contains:

```text
worlds/eldoria/
├── world.yaml                  # Identity, genre, art style, and dependencies
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
version: 1.0.0
description: A mist-shrouded archipelago of submerged ruins and arcane salvage.
genre: nautical-fantasy
art_style: ink-wash, muted teal, low horizon
default_system: classic-d20
tags: [mysterious, grim, salvage, ocean]
requires:
  - type: system
    id: classic-d20
    version: ">=1.0.0 <2.0.0"
```

The pinned opening location is a campaign setting rather than a world setting: the engine resolves it once at creation and records it in `game.yaml`. See [Editing Content Outside the App](22-editing-content) for the field list of each manifest.

## Starting Location Resolution

When a new campaign begins, LocalRPG determines the opening scene using a deterministic 4-stage resolution hierarchy:

1. **Pinned Setting (`settings.start_location`)**: If `game.yaml` pins a location, the engine binds to that specific entity ID.
2. **Player Location Reference**: If the player character's note carries a `location: "[[The Sinking Quay]]"`, or a wikilink to a location anywhere in the body, that location takes precedence.
3. **Any Authored Location**: If nothing pins or references a location, the engine takes the first indexed entity with `type: location`.
4. **Procedural Fallback**: If the world defines no locations, the engine creates an opening scene note (`games/<id>/entities/opening-scene.md`) from the world name and description.

## System Overrides

Sometimes a world introduces setting-specific mechanics (such as sanity in a Lovecraftian setting or oxygen consumption in hard sci-fi). Worlds can provide custom JavaScript hooks inside `system_overrides/<system-id>/hooks.js`. When a campaign runs with that specific system, these hooks merge with the base system mechanics.
