# Layered Scenes Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#58 PH-3](https://github.com/darkliquid/Projects/LocalRPG/issues/58)
**Epic:** [#21 Placeholder and fallback variety](https://github.com/darkliquid/Projects/LocalRPG/issues/21)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §7 (PH-3)
**Depends on:** [#56 PH-1](https://github.com/darkliquid/LocalRPG/issues/56)
**Scope:** `pkg/media`, `pkg/scene`, `frontend`

---

## 1. Problem

Every scene image is one flat picture. PH-1 gives the procedural generator real variety, and TH-2 adds
parallax, but parallax needs **layers** and there is only one. A generated image is also one flat
layer, so the theatre's motion is limited to a Ken Burns over the whole picture.

A layered procedural scene would let the background move behind the midground and the foreground,
which is the difference between a slideshow and a scene.

## 2. Goals

- The procedural generator emits a scene as **layers** (background, midground, foreground) rather than
  one flat SVG.
- Each layer carries a **depth**, so the theatre and the export can parallax them.
- A flat image (a generated illustration) remains valid as a single layer.
- Layers are deterministic, like PH-1's scenes.
- The rasteriser and the export consume layers unchanged in shape from today's single image.

## 3. Non-goals

- Generated images becoming layered; only the procedural generator layers.
- The parallax itself (TH-2) or the effects; this produces what they animate.
- New structures; PH-1's structures become layered.

## 4. Design

### 4.1 The layered scene

```go
// Layer is one depth of a scene, drawn back to front.
type Layer struct {
	Depth float64 // 0 = background, 1 = foreground
	SVG   []byte  // a self-contained SVG fragment, transparent where empty
}

// LayeredScene is a scene as one or more layers.
type LayeredScene struct {
	Width, Height int
	Layers        []Layer // ordered back to front
}
```

A `LayeredScene` with one layer at depth 0 is exactly a flat image; the existing single-image path
becomes the one-layer case, so nothing that consumes a single image breaks.

### 4.2 The generator

PH-1's composer already builds the scene from parts: a sky and ridge, a structure, and weather/fog.
Those become layers:

- **background** (depth 0): the sky gradient, the celestial, the ridge;
- **midground** (depth ~0.5): the structure (forest, city, coast, …);
- **foreground** (depth 1): the fog band, particles, and the weather overlay.

Each layer is a self-contained SVG with a transparent background, sized to the scene, so it can be
drawn over the others. The composed single SVG (PH-1's output) is the layers flattened, kept for
callers that want one image.

### 4.3 Depth and parallax

The depth is a constant per layer kind, so the background moves least and the foreground most. TH-2's
parallax reads it. A generated illustration has one layer at depth 0, so it gets no parallax, which is
correct: it is already a composed picture.

### 4.4 Serving and storage

The procedural provider's output is cached as a single image today (the art cache stores bytes and
names the file by content). For layers, the cache stores the composed SVG (the flattened image) for
the image path, and the layers are regenerated deterministically when a caller needs them (the
theatre). Because generation is cheap and deterministic, regenerating is fine; a caller that needs
layers calls `GenerateLayeredScene` directly.

This avoids changing the cache's shape (one file per image) while giving the theatre layers.

### 4.5 Consumers

- **Theatre**: when the scene art is procedural (or a layered scene is available), it draws the layers
  with parallax; a flat image is drawn as today.
- **Export**: the renderer draws layers with parallax, frame by frame; a flat image is drawn as today.
- **Fallback**: the procedural generator is the fallback, so the layered path is the one most scenes
  use when no generated image exists.

## 5. Behaviour

| Scene art | Theatre | Export |
| --- | --- | --- |
| a procedural scene | three layers with parallax | the same, per frame |
| a generated illustration | one layer, Ken Burns only | the same |
| a location backdrop (flat) | one layer, Ken Burns only | the same |
| the same request twice | identical layers | identical |

## 6. Testing

- `pkg/media`: `GenerateLayeredScene` returns background, midground, and foreground layers with
  increasing depth; each layer is valid SVG; the flattened output equals PH-1's single-image output
  for the same request; it is deterministic.
- `pkg/scene`/`frontend`: a layered scene draws with parallax; a one-layer scene draws flat.
- A regression guard: a flat image path is unchanged.

## 7. Rollout

Additive: the procedural generator gains a layered output alongside its single-image output. The
single-image cache and the flat path are unchanged.

## 8. Risks

- **SVG cost.** Three SVGs per scene, regenerated for the theatre, cost more CPU than one. They are
  small and the generation is cheap; measure it, and cache the layers in memory per scene if needed.
- **Layer seams.** Overlaying layers can show a seam where a layer's transparency meets another. Each
  layer must fully cover its area with its own content and use transparency only where the layer below
  should show.
- **Parallax over-application.** A layered scene with strong parallax can look gimmicky. TH-2's small
  magnitudes apply; the depth is a hint, not a large offset.
