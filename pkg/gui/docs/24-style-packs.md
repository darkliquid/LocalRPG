---
id: 24-style-packs
title: Style Packs
category: Configuration & Providers
order: 8
description: Theme the procedural art and the app chrome with a pack, or keep the built-in look.
---

# Style Packs

An image with no generated artwork is drawn by the built-in procedural
generator: a scene behind a location, and a bust for a character with no
portrait. A **style pack** changes that look without a code change.

## Pack sections

- **Scene palettes**: the sky, ground, ridge, fog, and accent colours of a genre.
- **Scene structures**: the structure a genre falls back to, such as `forest` or `city`.
- **Portrait categories**: extra species and archetypes, selected by a tag.
- **Genre palettes**: the colours the app chrome and the export use for a genre.

Every section is optional. A pack that sets one colour keeps the built-in value
for every other field, and one that sets a genre leaves the other genres alone.

## The file

A pack is a YAML file in the styles folder beside your configuration file:

```yaml
id: ashen
label: Ashen House Style
genres:
  fantasy:
    from: "#1a1f2b"
    to: "#2f3a4a"
    accent: "#c8a24a"
scene_palettes:
  fantasy:
    sky_top: "#101418"
    sky_bottom: "#2a3440"
    ground: "#14181c"
    ridge: "#1e242a"
    fog: "#8a9aa8"
    celestial: "#ffe6b0"
    accent_a: "#6a8f5a"
    accent_b: "#9aa4b0"
scene_structures:
  fantasy: forest
portrait_species:
  mycelian:
    ear_shape: frond
    jaw: narrow
    skin: ["#8a9a7a", "#6a7a5a"]
portrait_archetypes:
  warden:
    hair: hooded
    collar: high
    accessory: lantern
    garment: ["#3a4a3a", "#2a3a2a"]
```

The `id` names the pack in the configuration and in the art cache, and is
required.

## Selecting a pack

```yaml
styles:
  pack: ashen   # empty keeps the built-in look
```

The **Style Pack** picker on the Settings **Preferences** tab lists the packs in
the styles folder. A pack that does not load is listed with the reason and is
ignored.

The art cache key includes the pack id. Switching packs regenerates the
procedural art rather than serving the previous look.

## From the command line

```bash
localrpg styles list              # the packs in the config directory
localrpg styles validate pack.yaml
```

## Colour values

Colours are `#rrggbb`. A pack with a malformed colour, an unknown structure, or an
unknown portrait value is reported and ignored in full, and a typo cannot
half-apply.
