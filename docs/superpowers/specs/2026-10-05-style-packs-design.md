# Style Packs Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#60 PH-5](https://github.com/darkliquid/Projects/LocalRPG/issues/60)
**Epic:** [#21 Placeholder and fallback variety](https://github.com/darkliquid/Projects/LocalRPG/issues/21)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §7 (PH-5)
**Depends on:** [#56 PH-1](https://github.com/darkliquid/Projects/LocalRPG/issues/56), [#57 PH-2](https://github.com/darkliquid/Projects/LocalRPG/issues/57)
**Scope:** `pkg/media`, `pkg/config`, `pkg/gui`, `frontend`

---

## 1. Problem

PH-1, PH-2, and PH-3 give the procedural art real variety, and PH-4 gives the chrome a genre palette.
All of it is compiled in: a user who wants a different look, or a specific house style, cannot change
it without editing Go and rebuilding.

The procedural art is a **fallback that is always seen**, so its look is part of the product's feel.
Making it themeable is cheap and lets a community shape it.

## 2. Goals

- A **style pack**: a file that overrides the procedural palettes, structure choices, and seed
  parameters, plus the genre palette (PH-4) and portrait categories (PH-2).
- Loaded from a directory, selected by config, with the built-in look as the default pack.
- No code change to theme the output; a pack is data.
- A pack that omits a section inherits the built-in value for it, so a small pack is possible.
- Invalid packs degrade to the built-in look rather than failing a render.

## 3. Non-goals

- A pack editor UI; a pack is a text file.
- Generated images; a pack themes the procedural generator and the chrome.
- Sharing or fetching packs; the registry (PKG-4) could carry them later.

## 4. Design

### 4.1 The pack format

A YAML file:

```yaml
id: ashen
label: Ashen House Style
# Genre palettes (PH-4). A missing genre inherits the built-in.
genres:
  fantasy:
    from: "#1a1f2b"
    to: "#2f3a4a"
    accent: "#c8a24a"
# Scene palettes (PH-1). Keys match the built-in palette names.
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
# Structure preferences (PH-1): a genre's default structure.
scene_structures:
  fantasy: forest
# Portrait categories (PH-2): extra species/archetype tags and their shapes.
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

Every section is optional. A pack with only `genres` themes the chrome and leaves the art alone.

### 4.2 Loading

`pkg/media` gains a loader:

```go
// StylePack is a user-supplied override of the procedural look.
type StylePack struct {
	ID               string
	Label            string
	Genres           map[string]GenrePalette
	ScenePalettes    map[string]Palette
	SceneStructures  map[string]string
	PortraitSpecies  map[string]Species
	PortraitArchetypes map[string]Archetype
}

// LoadStylePack reads a pack from a file, validating it.
func LoadStylePack(path string) (StylePack, error)
```

Packs live under the config directory (`<config>/styles/<id>.yaml`). The config selects one:

```yaml
styles:
  pack: ashen   # empty = the built-in look
```

`pkg/gui` loads the selected pack once (keyed on the config revision, like the provider registries) and
passes it to the procedural generator and the chrome.

### 4.3 Resolution

A pack **overrides** the built-in tables by key: a genre palette in the pack replaces the built-in one
for that genre; a genre absent from the pack uses the built-in. The resolution is a merge, so a small
pack is useful.

The generator functions (PH-1's `paletteFor`, PH-2's `speciesFor`/`archetypeFor`, PH-3's layers) take
the pack as an argument (or a package-level "active pack" set once at startup), so a render uses the
pack's values.

### 4.4 Validation and degradation

A pack is validated on load:

- colour values are `#rrggbb`;
- a scene palette's keys are the known ones;
- a structure name is a known structure;
- a species' shapes are known values.

An invalid pack is **reported and ignored** (the built-in look is used), with a `style.pack_invalid`
trace and a warning in the settings. A render never fails because of a pack.

### 4.5 Surfaces

- **Config**: `styles.pack` selects a pack.
- **Settings UI**: a picker listing the packs found, with "built-in" as the default, and the
  validation warnings.
- **CLI**: `localrpg styles list` and `localrpg styles validate <path>`.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| no pack | the built-in look |
| a pack overriding `genres.fantasy` | fantasy chrome uses the pack; others are built-in |
| a pack with an unknown structure | the structure is ignored; the rest applies |
| an invalid colour | the pack is reported and ignored |
| a pack changing species | portraits use the new species |
| the export | the same pack applies |

## 6. Testing

- `pkg/media`: a pack merges over the built-in tables by key; an absent key inherits; an invalid value
  is reported and ignored; a full pack changes the output deterministically.
- `pkg/config`: `styles.pack` round-trips.
- `pkg/gui`: the selected pack reaches the generator and the chrome; an invalid pack warns.
- `frontend`: the picker lists the packs and the warnings.
- A regression guard: no pack yields the built-in output byte-for-byte.

## 7. Rollout

Additive: a loader, a config key, and a picker. No pack means the built-in look, exactly as today.

## 8. Risks

- **Pack complexity.** A full pack has five sections; a user can ignore four. The merge makes a small
  pack valid, and the docs show a minimal example.
- **Colour accessibility.** A pack can choose low-contrast colours. The spec validates the format, not
  the aesthetics; the docs note the contrast expectation, and the app's text has its own scrim.
- **Determinism.** A pack changes the seed inputs; a given pack plus a given scene is still
  deterministic, so the cache key must incorporate the pack id. Add the pack id to the art cache key.
