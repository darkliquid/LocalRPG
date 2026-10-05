# Theatre Effects Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#62 TH-2](https://github.com/darkliquid/LocalRPG/issues/62)
**Epic:** [#22 Theatre and export efficiency](https://github.com/darkliquid/LocalRPG/issues/22)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §10 (TH-2)
**Depends on:** [#58 PH-3](https://github.com/darkliquid/LocalRPG/issues/58), [#61 TH-1](https://github.com/darkliquid/LocalRPG/issues/61)
**Scope:** `frontend`, `pkg/scene`

---

## 1. Problem

The theatre is a slideshow. A beat shows its image, the dialogue types, and the next beat crossfades
in. The only motion is the crossfade (`pkg/scene/theater.go` mirrors it for video). A still image sits
completely still for as long as its line takes to read or play, which reads as a static screen rather
than a scene.

The export has the same limitation, so a story's video is a sequence of stills with a crossfade.

## 2. Goals

- **Ken Burns** on a still: a slow, subtle zoom and pan across the beat, scaled to its duration.
- **Parallax** on a layered procedural scene (PH-3): the background moves less than the foreground.
- A **mood tint** derived from the turn's outcome, so a failure scene reads colder than a success.
- Optional **weather overlays** (rain, snow, fog) as a light layer.
- The app theatre and the export renderer produce the same effect, so a story looks the same in both.

## 3. Non-goals

- New art. The effects animate what exists.
- Character animation; portraits stay still (a follow-up could breathe them).
- The pacing and the missing-audio handling (TH-1, TH-3).

## 4. Design

### 4.1 Ken Burns

A beat's image is drawn with a slow transform over the beat's duration:

- a scale from 1.0 to about 1.08 (or the reverse, alternating), and
- a small translation (a few percent) toward a point derived from the beat's seed.

The transform is a function of the beat's progress, so it is deterministic and the same in both
renderers. The app applies it with CSS (`transform` with a transition or an animation); the export
renderer applies it by compositing the image at an interpolated scale and offset per frame.

The magnitude is small on purpose: a Ken Burns that is noticed is too strong. The direction alternates
by beat so consecutive beats do not all drift the same way.

### 4.2 Parallax

When a scene has layers (PH-3), each layer is drawn with a different translation for the same beat
progress: the background moves least, the foreground most. The depth is a per-layer constant the
procedural generator assigns. A single-layer image has no parallax.

### 4.3 Mood tint

The turn's outcome maps to a tint:

- success/strong → a warm, slightly brighter tint;
- weak/partial → a neutral tint;
- miss/fail → a cool, slightly darker tint.

The tint is a colour overlay at a low opacity, drawn over the image and under the scrim, so it does
not affect text legibility. The mapping is the same one IMG-1 uses for prompt tone, so the image and
its treatment agree.

### 4.4 Weather overlays

When the scene's weather (from the location state, as IMG-1 reads it) is rain, snow, or fog, a light
overlay is drawn:

- rain: thin, slightly angled streaks, animated;
- snow: slow drifting dots;
- fog: a soft horizontal band with a slow drift.

The overlay is a small, procedurally drawn layer (CSS animation in the app, a drawn layer in the
export), bounded so it never obscures the text.

### 4.5 Parity

The app and the export must agree. The effects are defined by **pure functions of beat progress**
(scale, offset, tint opacity, overlay position), and each renderer implements the same functions. A
test asserts the two produce the same transform values for a set of progresses, so a divergence is
caught.

### 4.6 Performance

The effects are cheap: a CSS transform and an overlay in the app; a scaled composite and a drawn
overlay in the export. The export's existing frame-reuse optimisation (a repeat frame re-encodes the
previous image) must not be defeated by a per-frame transform; the renderer should treat an
animated beat as a fresh frame, which it already does for a crossfade.

## 5. Behaviour

| Beat | Effect |
| --- | --- |
| a still image | Ken Burns over its duration |
| a layered scene | parallax plus Ken Burns |
| a failed check | a cool tint |
| a scene with rain | a rain overlay |
| reduced motion (a preference) | no Ken Burns, no parallax, no overlay animation |
| the export | the same effects, frame by frame |

## 6. Testing

- `frontend`: the transform is applied and is a function of progress; the tint follows the outcome;
  the overlay follows the weather; a reduced-motion preference disables the motion.
- `pkg/scene`: the export renders the same transform and tint for the same beat and progress.
- A parity test: the app's and the export's transform functions agree on a set of progresses.
- A regression guard: a beat with no layers, no outcome, and no weather renders as a plain still.

## 7. Rollout

Additive, and gated by a reduced-motion preference (which the app may already read). No export
format change.

## 8. Risks

- **Motion sickness.** Ken Burns and parallax can discomfort some users. The reduced-motion preference
  is the guard, and the magnitudes are small.
- **Export cost.** A per-frame transform means more distinct frames to encode. The renderer already
  handles a crossfade's distinct frames; the magnitude is small, so the added cost is bounded. Measure
  it.
- **Legibility.** Tints and overlays must not reduce text contrast. They are drawn under the scrim and
  bounded; the existing scrim's opacity is unchanged.
