# Scene Consistency Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#53 IMG-3](https://github.com/darkliquid/LocalRPG/issues/53)
**Epic:** [#20 Scene images from turn content](https://github.com/darkliquid/LocalRPG/issues/20)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §4 (IMG-3)
**Depends on:** [#51 IMG-1](https://github.com/darkliquid/LocalRPG/issues/51)
**Scope:** `pkg/engine`, `pkg/media`

---

## 1. Problem

Each scene image is generated independently. IMG-1 makes the prompt reflect the turn, which is a
large improvement, but two images of the same room, in the same light, a turn apart can look like two
different rooms: different palette, different architecture, a different mood.

For a visual novel, consistency *is* the point. A player should recognise the place, and the
characters in it.

## 2. Goals

- Successive images of one **location** share a look: palette, lighting, and a few fixed descriptors.
- A **scene seed** anchors the generation, so the same scene is reproducible.
- Where the provider supports **image conditioning**, the previous image of the scene is passed as a
  reference.
- Where it does not, the seed and a stable prompt skeleton carry the consistency instead.
- The turn's variation (the action, the outcome) still shows through.

## 3. Non-goals

- Character consistency across different scenes; that is the portrait system's job, and a separate
  problem.
- A full "art direction" system.
- The trigger policy (IMG-2) and the budget (IMG-4).

## 4. Design

### 4.1 The scene seed and the style bible

A **scene** is a location plus a scene identity (the location id, or the act/scene the compiler
derives). It carries:

```go
// SceneStyle is the stable look of one scene.
type SceneStyle struct {
	SceneID   string   // the location id, or a scene key
	Seed      int64    // derived from SceneID + world style
	Palette   string   // a few palette words ("muted ochre, cold blue")
	Lighting  string   // "overcast", "lamplit", "dawn"
	Described string   // the location's appearance, if authored
	Reference string   // the previous image's path, when conditioning is available
}
```

`Seed` and the first three fields are derived from the location and the world style, so they are
stable for a location. `Reference` is the most recent scene image for that location, when one exists.

### 4.2 The prompt skeleton

IMG-1's prompt keeps a **stable prefix** from `SceneStyle` (the place, the palette, the lighting, the
appearance) and a **variable suffix** from the turn (the action, the outcome tone, the cast). The
prefix is identical for two turns in one scene; only the suffix changes. This is the consistency
mechanism for a provider with no conditioning: a stable subject and palette, a changing moment.

### 4.3 Image conditioning, where available

A new optional capability:

```go
// SceneConditioner is implemented by image providers that can take a reference
// image to condition a generation.
type SceneConditioner interface {
	GenerateSceneWithReference(ctx context.Context, req SceneRequest, reference []byte) ([]byte, error)
}
```

The art store asserts it; when the provider implements it and a `Reference` exists, it calls the
conditioned method with the previous scene image. Otherwise it calls `GenerateScene` (IMG-1) with the
stable prefix. The reference is the previous scene image for the same `SceneID`, read from
`assets/scenes/`.

### 4.4 What stays variable

The turn's action, the outcome tone, and the cast remain in the prompt, so a scene evolves: the same
room at dawn and at night, calm and then bloodied. Consistency is the place, not the moment.

### 4.5 Storage

A scene's image already lives at `assets/scenes/turn-<N><ext>` (IMG-2/the scene worker). Consistency
adds no new storage: the `Reference` is the previous turn's scene image for the same location, found
by scanning recent scene images and their location. A small index (turn → location) already exists in
the timeline, so the lookup is cheap.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| two turns in one location, no conditioning | a stable palette and place; the moment varies |
| the same, with conditioning | the second image references the first |
| a location change | a new scene seed; a new look |
| a scene's first image | no reference; the seed and prefix carry it |
| a provider with no conditioning | the stable prefix is the whole mechanism |

## 6. Testing

- `pkg/engine`/`pkg/media`: `SceneStyle` is stable for a location across turns; the prompt's prefix is
  identical and its suffix differs; the seed changes with the location.
- `pkg/media`: a provider implementing `SceneConditioner` is called with the reference; one that does
  not is called with the plain request.
- A regression guard: a scene's first image is unchanged from IMG-1's output.

## 7. Rollout

Additive: a scene style, a stable prompt prefix, and an optional conditioning path. Existing cached
scene images remain valid; a scene re-generates with the new prompt on the next trigger.

## 8. Risks

- **Provider support is uneven.** Conditioning is optional; the seed-and-prefix path is the baseline
  and must stand alone.
- **Reference cost.** Passing a reference image costs tokens/bytes for a cloud provider. Bound it to
  the same scene and the most recent image, and let IMG-4's budget cover it.
- **Consistency vs variation.** Too stable and the scene never changes; the variable suffix is the
  release valve. The split is the design.
