# Missing Audio and Pacing Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#63 TH-3](https://github.com/darkliquid/LocalRPG/issues/63)
**Epic:** [#22 Theatre and export efficiency](https://github.com/darkliquid/LocalRPG/issues/22)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §10 (TH-3)
**Depends on:** [#61 TH-1](https://github.com/darkliquid/LocalRPG/issues/61)
**Scope:** `frontend`, `pkg/scene`

---

## 1. Problem

A beat with no audio is invisible. The theatre advances on a timer when a beat has no clip
(`frontend/src/components/StoryTheater.tsx:193-199`), so a missing clip looks exactly like a beat
that is playing: the prose shows, the progress moves, and nothing says the audio is gone. A user
whose TTS provider failed, or whose clip was dropped by a queue overflow, sees a silent theatre and
no explanation.

The pacing is also fixed. A silent beat advances after a constant, whether it is four words or forty,
so a long silent line is cut off before it can be read and a short one lingers.

## 2. Goals

- A beat with no audio **says so**, visibly, per beat.
- A silent beat is paced by how long it takes to read, not by a constant.
- The same logic serves the theatre and the export player, so a silent export paces the same way.
- Nothing changes for a beat that has audio.

## 3. Non-goals

- Fixing why the audio is missing (a provider failure is surfaced elsewhere).
- Subtitles and captions (TH-4).
- The audio pipeline's own retries.

## 4. Design

### 4.1 A per-beat audio state

Each beat already knows whether it has a clip (`segment.audio_urls`, and the clip-group leader
logic). The theatre derives a beat audio state:

- `audio` — a clip exists and plays;
- `silent` — no clip, because the provider was disabled, failed, or the segment had no speakable
  text;
- `pending` — a clip is being synthesized (`audio_progress` stages already report this).

The state drives both the affordance and the pacing.

### 4.2 The affordance

A `silent` beat shows a small, quiet chip on the beat (a muted-speaker glyph and "no audio"), in the
same place the per-beat audio controls sit. It is informational, not an error: a campaign with TTS
disabled is not broken.

A `pending` beat shows the existing synthesizing indicator (already present for segments without a
clip, `frontend/src/components/TurnSegments.tsx:92-104`), reused in the theatre.

### 4.3 Reading-speed pacing

The pacing function is shared (frontend and the Go export renderer):

```ts
// readingDurationMs estimates how long a line takes to read, in milliseconds.
export function readingDurationMs(text: string, wpm = 200, minMs = 900): number
```

- word count divided by words-per-minute, floored at a minimum so a short line is not flashed;
- scaled by the theatre's speed control.

A `silent` beat advances after `readingDurationMs(text)`. An `audio` beat advances on completion
(TH-1) and, if the completion is lost, after `max(readingDurationMs(text), expected clip length)`
rather than a fixed bound.

### 4.4 The export renderer

`pkg/scene` compiles beats with a duration derived from the clip length and, for a silent beat, from
a reading estimate (the existing `groupBeatShares`/`silence` logic at
`pkg/scene/compile.go:130-162,385-397`). It adopts the same reading-speed rule so a silent export
paces like the app, and a golden test pins the estimate for a fixed line.

### 4.5 What does not change

A beat with audio plays and advances exactly as before. The change is confined to silent and pending
beats.

## 5. Behaviour

| Beat | Affordance | Advance |
| --- | --- | --- |
| has audio | the normal controls | on completion (TH-1) |
| silent, short line | "no audio" chip | after the reading minimum |
| silent, long line | "no audio" chip | after the reading estimate |
| pending | synthesizing indicator | when the clip arrives |
| TTS disabled for the campaign | every beat shows "no audio" | by reading speed |

## 6. Testing

- `frontend`: `readingDurationMs` scales with length and speed and floors at the minimum; a silent
  beat shows the chip and advances by the estimate; a beat with audio is unchanged; a pending beat
  shows the indicator.
- `pkg/scene`: the compiler uses the same reading estimate for a silent beat; a golden test pins it.
- A regression guard: a fully-audio turn's timing is unchanged.

## 7. Rollout

Frontend plus a shared pacing rule in the export renderer. No migration.

## 8. Risks

- **Reading speed is a guess.** 200 wpm is a common average; it is a constant, tunable later, and the
  floor prevents an unreadable flash. Better a reasonable guess than a fixed gap that ignores length.
- **A chip on every beat when TTS is off.** That is honest but repetitive. Consider showing the chip
  once per turn when the whole turn is silent, and per beat only when the silence is mixed. The spec
  prefers per beat for truth; the turn-level variant is a presentation choice.
- **Parity drift.** The frontend and the Go renderer must use the same rule; a shared constant and a
  golden test keep them aligned.
