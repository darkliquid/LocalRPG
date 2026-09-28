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
