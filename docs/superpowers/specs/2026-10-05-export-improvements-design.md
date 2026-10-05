# Export Improvements Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#65 TH-5](https://github.com/darkliquid/LocalRPG/issues/65)
**Epic:** [#22 Theatre and export efficiency](https://github.com/darkliquid/LocalRPG/issues/22)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §10 (TH-5)
**Depends on:** [#55 IMG-5](https://github.com/darkliquid/LocalRPG/issues/55), [#64 TH-4](https://github.com/darkliquid/LocalRPG/issues/64)
**Scope:** `pkg/export`, `pkg/scene`, `frontend`

---

## 1. Problem

Two changes in this phase grow the export and one leaves it thin:

- **IMG-5** puts a per-beat scene illustration in the export, so a long story embeds many more images.
  The single-file web bundle is already about 1.6 MB gzipped for a modest campaign
  (`docs/superpowers/specs/2026-09-30-single-file-web-bundle-design.md` §2); per-beat images could
  multiply that.
- **TH-4** adds a subtitle track; a long story's VTT is not free either.
- The export has **no chapters**: a viewer cannot jump to a scene, and the video has no chapter
  metadata.

The export is the artefact a user shares, so its size and its navigation matter.

## 2. Goals

- **Chapters**: scene boundaries exposed in the web player and as WebM chapter metadata in the video,
  so a viewer can navigate.
- **Smaller output**: downscale embedded images to what the player shows, and re-measure the bundle.
- **Video transitions**: scene-level transitions that reuse TH-2's effects, so a scene change reads as
  a change.
- Record the measured sizes, so a regression is visible.

## 3. Non-goals

- The subtitle track itself (TH-4) and the per-beat art (IMG-5).
- A new export format.
- Streaming or partial exports.

## 4. Design

### 4.1 Chapters

A chapter is a scene (the compiler already groups turns into scenes by location,
`pkg/scene/compile.go:198-200`). Each chapter carries a title (the scene's location name) and a start
time (its first beat's start).

- **Web**: the player renders a chapter list, and a `chapters` WebVTT track (WebVTT has a chapter
  cue format) lets the browser's native controls show chapters where supported.
- **Video**: WebM supports chapter metadata; the muxer writes a chapters element from the same list.
  If the muxer cannot, a sidecar `.chapters.txt` (the ffmpeg chapters format) is written instead, and
  the spec notes the limitation.

The chapters come from the compiled script, so the web and the video agree.

### 4.2 Smaller output

- **Image downscale**: an embedded image is scaled to the export's display size (the player's canvas,
  for example 1280×720) before embedding, using the standard library's image scaling
  (`golang.org/x/image/draw`, already a dependency). A 2 MB illustration becomes a fraction of that.
  The original stays in the campaign; only the export's copy is downscaled.
- **Image format**: keep the source format; where an image is already a large PNG, re-encode as JPEG
  or WebP at a quality that is visually lossless at the display size. The scene package already
  decodes SVGs and rasters (`pkg/scene/art.go`), so the pipeline exists.
- **Gzip level**: the bundle uses gzip; ensure the highest level is used for the final artefact, since
  it is written once.
- **Measure**: a benchmark exports the `website/demo` fixture and records the bundle size; the spec
  pins a target (for example, no larger than 1.5× the pre-IMG-5 size for the same story) and the
  benchmark fails if it is exceeded, so a regression is caught.

### 4.3 Video transitions

TH-2 gives per-beat effects. TH-5 adds **scene transitions**: at a scene boundary, a longer, distinct
transition (for example a slower fade or a brief dip to black) so a location change reads differently
from a beat change. The transition duration is a small constant; the renderer already forces a
keyframe at a scene crossfade (`pkg/export/video.go:296-311`).

### 4.4 The web player

The web player gains a chapter list beside the transport, and clicking a chapter seeks. The player
already has a scrubber; the chapters are markers on it plus a list.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| a multi-scene story, web | a chapter per scene; seeking works |
| the same, video | chapter metadata or a sidecar |
| a large illustration | downscaled in the export, not in the campaign |
| the demo fixture | the bundle size is within the pinned target |
| a scene boundary | a distinct transition |
| a single-scene story | one chapter; no boundary transition |

## 6. Testing

- `pkg/scene`/`pkg/export`: chapters are derived from scenes with the right titles and starts; the web
  player lists them; the video writes chapter metadata or a sidecar.
- `pkg/export`: an embedded image is downscaled to the display size; the benchmark records the size
  and fails above the target.
- A regression guard: a story with no illustrations and one scene is unchanged apart from chapters.

## 7. Rollout

Additive: chapters, a downscale pass, and a scene transition. The measured size is recorded; the
target is a guard, not a hard limit that blocks an export.

## 8. Risks

- **Downscale quality.** A downscaled illustration is smaller but softer. The display size is what the
  player shows, so at native size it is indistinguishable; the original remains for the app.
- **Chapter metadata support.** Not every player reads WebM chapters; the sidecar is the fallback. The
  web player reads its own list regardless.
- **Size target churn.** A target that is too tight blocks a legitimate export. The benchmark warns
  and records; the target is revisited if it proves wrong.
