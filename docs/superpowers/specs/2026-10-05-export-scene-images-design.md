# Export Scene Illustrations Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#55 IMG-5](https://github.com/darkliquid/LocalRPG/issues/55)
**Epic:** [#20 Scene images from turn content](https://github.com/darkliquid/LocalRPG/issues/20)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §4 (IMG-5)
**Depends on:** [#51 IMG-1](https://github.com/darkliquid/LocalRPG/issues/51)
**Scope:** `pkg/scene`, `pkg/export`

---

## 1. Problem

The app generates two kinds of image: a location backdrop (the cached, description-based art) and a
turn scene illustration (the turn-aware image IMG-1 and IMG-2 produce). The **export uses only the
location backdrop**:

```go
// pkg/export/script.go:492-496
store := media.NewArtStore(...)
```

and `scene.Compile` groups turns into scenes by location and resolves scene art from the location
(`pkg/scene/compile.go:198-200,320`).

So an exported story shows the place but never the moment: the image the app showed for the turn
where the player failed the lock is missing from the export, and the export looks static next to the
app.

## 2. Goals

- An export uses a turn's scene illustration where one exists, and the location backdrop otherwise.
- Both the web and the video export.
- The export's pacing and layout are unchanged; only the image changes.
- The export player and the app show the same image for the same beat.

## 3. Non-goals

- Generating images during an export. The export uses what the campaign already has.
- The prompt (IMG-1), the trigger (IMG-2), consistency (IMG-3), or the budget (IMG-4).
- A new image kind.

## 4. Design

### 4.1 The art resolution order

`scene.Compile` already has an `ArtResolver` (`pkg/scene/compile.go:51-54`). It gains a per-beat
preference:

1. the turn's scene illustration (`assets/scenes/turn-<N>.<ext>`), when present;
2. the scene's location backdrop, as today;
3. the script banner, as today.

The order is per **beat**, not per scene, because an illustration belongs to a turn. A scene with no
illustrations resolves exactly as it does now.

### 4.2 Where the illustration is found

The scene worker writes `assets/scenes/turn-<N><ext>` (the extension from `media.ArtExtension`). The
compiler's art resolver reads the game's assets directory and, for each beat's turn number, probes the
known extensions, exactly as `GetTurnSceneImage` does (`pkg/gui/service.go:2272-2282`). A missing file
falls through to the location.

### 4.3 Web and video parity

Both exports share `scene.Compile`, so the resolution order applies to both:

- **Web**: the bundle embeds the chosen image as a data URI, as it embeds location art today
  (`pkg/export/web.go:113-152`); a missing asset is skipped, never a bundle failure.
- **Video**: the renderer draws the chosen image, using its existing cover-fit and crossfade.

Because both use the compiled script, the two exports cannot diverge on which image a beat shows.

### 4.4 Size

A turn illustration is one image per beat, where before there was one per scene. For a long story
that is many images in the web bundle. The bundle already gzips and the images are content-addressed;
the compiler should reuse an image already embedded for the same turn, and the existing
missing-asset handling covers an over-large bundle. A follow-up could downscale embedded images;
this spec notes it as a risk, not a task.

### 4.5 The player

The export player renders a beat's image from the compiled script, so no player change is needed
beyond it accepting the new per-beat art. `StoryPlayer.tsx` already renders `beat.art`.

## 5. Behaviour

| Beat | Image |
| --- | --- |
| a turn with an illustration | the illustration |
| a turn with none | the location backdrop |
| a turn whose illustration file is missing | the location backdrop |
| a scene with no illustrations | exactly as today |
| the web export | the chosen image embedded |
| the video export | the chosen image rendered |

## 6. Testing

- `pkg/scene`: the resolver prefers a turn illustration and falls back to the location; a missing file
  falls through; a scene with none is unchanged.
- `pkg/export`: the web bundle embeds the turn illustration for a beat that has one; the video render
  plan references it.
- A parity test: the app's `GetTurnSceneImage` and the compiler resolve the same file for a turn.
- A regression guard: an export of a campaign with no illustrations is byte-identical to before.

## 7. Rollout

Additive: a per-beat art preference. A campaign with no illustrations exports exactly as today.

## 8. Risks

- **Bundle size.** Many per-beat images inflate the web bundle. The existing gzip and missing-asset
  handling bound the failure; a downscale pass is the follow-up if it bites.
- **Stale images.** A regenerated illustration changes the export; the export is generated on demand,
  so it picks up the current image.
- **Turn numbering.** The compiler must map a beat to its turn number; it already carries the turn in
  the beat, so the mapping is direct.
