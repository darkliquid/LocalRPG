# Procedural Portraits Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#57 PH-2](https://github.com/darkliquid/LocalRPG/issues/57)
**Epic:** [#21 Placeholder and fallback variety](https://github.com/darkliquid/LocalRPG/issues/21)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §7 (PH-2)
**Scope:** `pkg/media`, `pkg/gui`

---

## 1. Problem

A character with no generated portrait gets a procedural bust. It is the same shape every time:
`GenerateProceduralBustSVG(id, name, gender)` (`pkg/media/procedural_bust.go:9-41`) derives two hues
from an FNV-32a hash of the id and draws a 3/4 bust with a background gradient, shoulders, neck, head,
and jaw.

So a table of twelve NPCs is twelve identical silhouettes in twelve colour pairs. Nothing reflects
that one is an orc warrior and another a human scholar, or that one is wounded. The portrait is the
most character-specific image in the app and the least character-specific in practice.

## 2. Goals

- The bust reflects the entity: its **species**, **archetype**, **palette**, and **expression**, from
  its tags and state.
- More visual variety: face shape, hair, and a small accessory by archetype.
- Deterministic: the same entity yields the same portrait.
- Consistent with the entity's description, so a "grizzled orc mercenary" looks like one.
- Cheap and pure Go, for the no-GPU path and exports.

## 3. Non-goals

- Generated portraits (the image providers do those).
- Portrait animation.
- The scene art (PH-1) and layered scenes (PH-3).

## 4. Design

### 4.1 A structured request

```go
// PortraitRequest carries what a procedural portrait is derived from.
type PortraitRequest struct {
	ID     string
	Name   string
	Gender string
	Tags   []string
	State  map[string]interface{}
}

// GenerateProceduralPortrait draws a bust for a character.
func GenerateProceduralPortrait(req PortraitRequest) []byte
```

The existing `GenerateProceduralBustSVG` is kept as a wrapper that fills only id, name, and gender, so
its callers are unchanged.

### 4.2 Species and archetype from tags

Two small tag→category tables, matched case-insensitively against the entity's tags:

- **Species**: `human` (default), `elf`, `dwarf`, `orc`, `halfling`, `beastfolk`, `undead`,
  `construct`, and a few more. A species sets the face silhouette: ear shape, brow, jaw, and a skin
  hue range.
- **Archetype**: `warrior`, `mage`, `rogue`, `scholar`, `priest`, `ranger`, `noble`, `labourer`. An
  archetype sets the hair, a collar or hood, and one accessory (a sword hilt, a staff tip, a hood, a
  circlet, a satchel strap).

An unmatched tag set defaults to `human` + a seeded archetype, so every character still looks like
someone.

### 4.3 Palette

The palette derives from species (a skin range) and archetype (a garment range), with the id's hash
choosing within the range. This keeps a species recognisable and an archetype readable while giving
each character a distinct pair, replacing the current "two arbitrary hues".

### 4.4 Expression from state

A small state→expression table:

- a low health stat (the system's `HealthSpec.Stat`, when declared) → a haggard or strained
  expression (a drawn brow, a paler skin shift);
- a `mood` or `expression` state value, when present → that expression (`calm`, `angry`, `weary`);
- otherwise a neutral expression.

The state read is best-effort; an absent state is neutral.

### 4.5 Determinism

Everything derives from the id, tags, and state through one seeded RNG, so the same character yields
the same portrait. The GUI caches the portrait by entity and version, as today.

### 4.6 Where it is used

`GetCharacterPortrait` (`pkg/gui/service.go`) returns the procedural bust when a character has no
generated portrait; it passes the entity's tags and state to `GenerateProceduralPortrait`. The export
uses the same function, so a story's characters look the same as in the app.

## 5. Behaviour

| Entity | Portrait |
| --- | --- |
| tags `orc`, `warrior` | an orc silhouette with a warrior's build and an accessory |
| tags `elf`, `mage` | an elf with pointed ears and a mage's collar |
| no tags | a human with a seeded archetype |
| state `health: 1` (low) | a strained expression |
| the same entity twice | the same portrait |
| a character with a generated portrait | the generated one (unchanged) |

## 6. Testing

- `pkg/media`: species and archetype are chosen from tags; the same request is deterministic; a
  different species changes the silhouette; a low health state changes the expression; an empty
  request still yields a bust.
- `pkg/media`: the old `GenerateProceduralBustSVG` wrapper produces the previous output for the same
  inputs (a regression guard).
- `pkg/gui`: a portrait-less character gets a tag-aware portrait; the export uses the same function.
- A golden matrix: (species × archetype) renders without panic and produces distinct hashes.

## 7. Rollout

Additive: a richer generator behind the same serving path. An existing procedural portrait changes
(it is richer), which is the point; a generated portrait is untouched.

## 8. Risks

- **Tag vocabulary.** Tags are free-form; the tables match what they can and default gracefully. A
  world with unusual tags still gets a portrait.
- **Determinism creep.** One seeded RNG, as PH-1; a byte-identity test guards it.
- **Scope.** Portraits are a fallback; keep the shapes simple and the accessories to one. It is not a
  character creator.
