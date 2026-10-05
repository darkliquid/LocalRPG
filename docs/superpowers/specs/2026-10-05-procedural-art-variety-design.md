# Procedural Art Variety Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#56 PH-1](https://github.com/darkliquid/LocalRPG/issues/56)
**Epic:** [#21 Placeholder and fallback variety](https://github.com/darkliquid/LocalRPG/issues/21)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §7 (PH-1)
**Scope:** `pkg/media`, `pkg/scene`, `pkg/gui`, `pkg/engine`

---

## 1. Problem

The built-in image provider is the one that always works: no GPU, no server, no key, no network. It
is also the one a player sees most, because it backs every scene that has no generated image, every
missing asset, and the whole zero-GPU path.

Today it renders from a thin vocabulary. `media.GenerateImage` (`pkg/media/procedural_art.go:17-102`)
matches a handful of keywords to about three palettes (`blood`/`fire`/`flame` crimson,
`swamp`/`mire`/`toxic` mire, `ruin`/`catacomb`/`dungeon` ruins) and draws one of three structure
silhouettes over an 800×600 SVG with a fixed sky gradient, a celestial glow, a ridge, and fog. The
portrait generator (`pkg/media/procedural_bust.go:9-41`) derives two hues from an FNV hash of the
entity id.

Three palettes across a campaign reads as a bug, not a style. Nothing varies by genre, mood, time of
day, or weather, so a sunlit meadow and a midnight crypt look the same unless the keyword happens to
match.

## 2. Goals

- More palettes, keyed by **genre** and **mood**, not only by a keyword in the prose.
- More **structures**, so scenes differ in composition, not only colour.
- **Time of day** and **weather** variation.
- Art stays **deterministic** and **cheap**: the same inputs always produce the same image, and it
  runs on the no-GPU path and inside exports.
- The generator can receive structured hints rather than parsing prose.

## 3. Non-goals

- Procedural portraits (that is PH-2) and layered SVG (PH-3).
- Any neural model. This stays pure Go.
- Replacing generated images; this improves the fallback and the offline path.

## 4. Design

### 4.1 A structured hint path

The image client interface is prompt-only (`Generate(ctx, prompt)`), which is why the generator
parses prose. Add an optional capability interface in `pkg/media`, mirroring the existing
capability-interface pattern (`MarkdownAware`, `VoiceCatalog`):

```go
// SceneRequest carries structured hints for a generated scene image.
type SceneRequest struct {
	Prompt    string
	Genre     string // world genre, e.g. "fantasy", "cyberpunk"
	Mood      string // e.g. "tense", "serene", "grim"
	TimeOfDay string // dawn | day | dusk | night
	Weather   string // clear | rain | fog | snow
	Seed      string // stable per scene; hashed into the layout
}

// SceneHintProvider is implemented by image providers that can use structured
// hints rather than only the prose prompt.
type SceneHintProvider interface {
	GenerateScene(ctx context.Context, req SceneRequest) ([]byte, error)
}
```

`media.ArtStore`/`ImagePipeline` assert the interface when calling the provider: if it implements
`SceneHintProvider`, call `GenerateScene` with the hints; otherwise call `Generate(prompt)` exactly
as today. No existing provider changes behaviour.

### 4.2 Where the hints come from

- **Genre** from the world manifest (`WorldManifest.Genre`, `Tags`; `pkg/core/types.go:43-52`),
  already available to the art store as the world style.
- **Mood** from the location's tags and state, or, for a turn scene illustration, the turn outcome
  tone (success → brighter, failure → darker).
- **TimeOfDay** and **Weather** from the location's state when the system tracks them (for example
  `state.time_of_day`), else derived deterministically from the seed so a location is consistent.
- **Seed** from the location id plus its appearance hash, which the pipeline already computes for the
  art cache key (`AppearanceHash`, `pkg/media/image.go:61-82`), so a scene's look is stable and the
  cache key is unchanged.

The hints are best-effort: an absent hint falls back to a deterministic derivation, never to an
error.

### 4.3 A richer generator

`pkg/media/procedural_art.go` is restructured into three small tables and a composer:

- **Palettes** (`palettes.go`): a `Palette` has sky top/bottom, ground, ridge, fog, celestial, and
  two accent colours. Indexed by genre with mood as a modifier (for example `fantasy` + `grim`
  desaturates and darkens). At least eight palettes across the genres the presets ship.
- **Structures** (`structures.go`): `ridge`, `forest`, `city`, `coast`, `interior`, `dungeon`,
  `ruins`, `sky`. Each is a small SVG fragment drawn relative to the horizon. Selected by location
  tags, then by genre default, then by a seeded choice.
- **Modifiers**: time of day shifts the sky gradient and the celestial position; weather adds a
  layer (rain streaks, fog bands, snow dots) drawn over the structure.

`GenerateImage(prompt)` keeps working by deriving hints from the prose (the current keyword match
becomes the fallback that fills absent hints), so nothing that calls it today breaks.

The output stays an 800×600 SVG with the same outer shape, so `pkg/scene`'s rasteriser
(`pkg/scene/art.go:110-126`) and the export path are unchanged.

### 4.4 Determinism and caching

Everything is derived from `Seed` plus the resolved hints, through a single seeded RNG. Two calls
with the same `SceneRequest` produce byte-identical SVG, so the art cache key
(`ComputeArtCacheKey`, `pkg/media/image.go:37`) stays correct and no extra cache invalidation is
needed. A test asserts byte-identity for repeated calls and difference for a changed seed.

## 5. Behaviour

| Scene | Result |
| --- | --- |
| forest location, fantasy, day, clear | forest structure, daylight palette |
| same location, night | same structure, night palette, stars |
| crypt location, grim mood | dungeon structure, desaturated palette |
| location with `state.weather: rain` | rain overlay |
| unknown genre | a default palette, chosen deterministically by seed |
| a provider that is not a `SceneHintProvider` | unchanged `Generate(prompt)` path |

## 6. Testing

- `pkg/media`: `GenerateScene` is deterministic for a fixed `SceneRequest`; a changed seed changes
  the bytes; every palette and structure renders without panic; a missing hint falls back.
- `pkg/media`: the `SceneHintProvider` assertion path is used when implemented and the prose path
  otherwise (a fake provider for each).
- `pkg/scene`: the rasteriser consumes the new SVGs unchanged (an existing-art smoke test).
- A golden test: a small matrix of (genre × structure × time × weather) rendered to a hash, so a
  palette regression is caught.

## 7. Rollout

No configuration and no cache migration: the art cache key does not change, and existing cached
images remain valid. New scenes render with the richer generator.

## 8. Risks

- **Palette sprawl.** Eight palettes across five families is a lot of hand-tuned colour. Keep them
  small and systematic (a base hue per genre, a lightness/saturation shift per mood) rather than
  hand-picked per combination.
- **Determinism creep.** Any use of `math/rand`'s global source or `time.Now()` breaks the cache
  contract. The single-seeded-RNG rule and the byte-identity test guard it.
- **Scope.** It is tempting to make this a full generative system. It is a fallback; keep the
  structures simple and the composition flat.
