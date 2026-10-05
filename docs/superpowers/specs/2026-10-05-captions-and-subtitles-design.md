# Captions and Subtitles Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#64 TH-4](https://github.com/darkliquid/LocalRPG/issues/64)
**Epic:** [#22 Theatre and export efficiency](https://github.com/darkliquid/LocalRPG/issues/22)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §10 (TH-4)
**Depends on:** [#63 TH-3](https://github.com/darkliquid/LocalRPG/issues/63)
**Scope:** `frontend`, `pkg/scene`, `pkg/export`

---

## 1. Problem

The theatre shows a line as it is spoken, but only as prose in the beat; there is no caption track for
a viewer who is deaf or hard of hearing, or watching without sound. The export has no subtitles at
all: the web player and the video are silent-prose-only, and a viewer who cannot hear the audio gets
nothing from the spoken lines beyond the narration text.

Accessibility is the primary driver; a caption track is also the honest answer to a missing-audio
beat (TH-3), which otherwise just advances.

## 2. Goals

- A **caption** in the theatre: the current spoken line, as an overlay, optional and toggled.
- A **WebVTT subtitle track** in both exports, from the same segments.
- Captions and subtitles come from the **same source** (the turn segments), so they cannot diverge.
- The timing comes from the audio (the clip duration) where it exists, and from the reading estimate
  (TH-3) where it does not.
- A preference to turn captions on.

## 3. Non-goals

- Speech recognition; the segments are already text.
- Translated subtitles.
- The visual effects (TH-2) and the pacing (TH-3), which this uses.

## 4. Design

### 4.1 The source

A turn's **segments** (`entity.TurnSegment`: kind, speaker, text) are the text. The **timing** comes
from the compiled script: each beat's start and duration are already computed
(`pkg/scene/compile.go`, the beat durations), from the clip length or TH-3's reading estimate.

So a caption cue is `{start, end, speaker, text}` derived from a beat, and the theatre's current
caption is the current beat's.

### 4.2 The theatre caption

An overlay at the bottom of the stage showing the current spoken line, in a legible style (a scrim
behind it, as the dialogue already uses). It is:

- **on** when a preference (`captions: on`) is set, or when the system prefers captions
  (`prefers-reduced-motion` is not the signal; there is no standard "prefers captions" media query, so
  a user preference is the control);
- **off** by default, so it does not duplicate the dialogue for a hearing viewer;
- toggled by a control in the transport.

When a beat has no audio and no caption, the caption is the only text, so the beat still reads.

### 4.3 The WebVTT track

`pkg/scene` (or `pkg/export`) gains a function that renders the compiled beats as WebVTT:

```go
// Captions renders a campaign's compiled beats as a WebVTT document.
func Captions(beats []Beat) string
```

- one cue per spoken beat, `start --> end`, the speaker as a `<v Speaker>` voice tag (WebVTT's
  standard), and the line as the cue text;
- narration beats are **not** captions (the narration is the prose, not speech); a spoken line is.

The web export embeds the `.vtt` (as a data URI or a blob the player loads) and enables the player's
caption track. The video export attaches the `.vtt` as a sidecar file beside the `.webm` (a muxed
subtitle track in WebM is possible but a sidecar is simpler and plays in every player), and names it in
the export summary.

### 4.4 Parity

Because both the theatre and the exports derive captions from the same segments and the same compiled
timings, the caption at time *t* in the theatre and the cue at *t* in the export agree. A test asserts
the WebVTT cue for a beat matches the theatre's caption for that beat.

### 4.5 Accessibility detail

- The cue text is the spoken line exactly, including the speaker's name, so a viewer knows who speaks.
- The caption overlay has sufficient contrast (the existing scrim plus a solid backdrop).
- The theatre's toggle is reachable by keyboard and labelled.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| a spoken beat, captions on | the line is shown as a caption |
| a spoken beat, captions off | no caption (the dialogue shows) |
| a silent beat | the caption is the only text |
| the web export | the player offers the `.vtt` track |
| the video export | a `.vtt` sidecar beside the `.webm` |
| a narration beat | no cue |
| the same beat in the app and the export | the same text and timing |

## 6. Testing

- `pkg/scene`/`pkg/export`: `Captions` emits one cue per spoken beat with the right start/end and a
  voice tag; narration produces no cue; the VTT is well-formed.
- `frontend`: the caption overlay shows the current spoken line when enabled; the toggle works; a
  silent beat still shows a caption.
- A parity test: the WebVTT cue for a beat matches the theatre's caption.
- A regression guard: captions off changes nothing.

## 7. Rollout

Additive: an optional overlay, a generated track, and a preference. Captions are off by default.

## 8. Risks

- **Duplication.** A hearing viewer sees the dialogue and the caption. Off by default; the toggle is
  explicit.
- **Timing drift.** Captions use the compiled timings, the same ones the export plays; a divergence is
  caught by the parity test.
- **Video subtitle support.** A `.vtt` sidecar is not muxed; a player must load it. That is a known
  limitation, documented; muxing is a follow-up if it matters.
