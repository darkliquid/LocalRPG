# Theatre Performance Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#61 TH-1](https://github.com/darkliquid/LocalRPG/issues/61)
**Epic:** [#22 Theatre and export efficiency](https://github.com/darkliquid/LocalRPG/issues/22)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §10 (TH-1)
**Scope:** `frontend`, `pkg/gui`

---

## 1. Problem

The theatre paces itself with a poll and a fixed gap. `StoryTheater` advances a beat when the server's
playback status leaves `playing`, and it learns that by polling `/api/audio/status` every 500 ms
(the turn-latency research cites `frontend/src/App.tsx:317-331`). Each beat also waits a fixed
`BEAT_GAP_MS = 300` (`frontend/src/components/StoryTheater.tsx:36`), and the next beat's audio is
requested only after the current one ends (`StoryTheater.tsx:142-157`).

The result is up to half a second of dead air per beat, a 300 ms pause on top, and a serial
audio/image fetch chain. For a ten-beat turn that is several seconds of avoidable latency, and it is
the theatre's dominant responsiveness cost.

## 2. Goals

- Advance a beat on a real completion event, not a poll.
- Fetch the next beat's audio and image **while** the current beat plays.
- Preload images so a scene change is instant.
- Retune the inter-beat gap to what is actually needed.
- No change to what plays or in what order.

## 3. Non-goals

- The export player's pacing, which is offline and has no server.
- Missing-audio handling (TH-3) and effects (TH-2).
- The audio pipeline's own latency (the turn-latency work).

## 4. Design

### 4.1 Event-driven beat advance

The server already streams playback progress over SSE for other purposes (the export progress and
audio-progress channels). Add a **playback completion event**: when the server's queue streamer
finishes a clip, it emits `{turn, segment, state: "ended"}` on a subscription the theatre holds.

The theatre subscribes once per session and advances the current beat when its completion arrives,
replacing the poll. The poll is removed. A safety timeout remains (a beat that never reports ending
advances after a generous bound, so a dropped event cannot stall playback).

The server side reuses the existing `playback.Player` completion signal (`pkg/media/playback`) and
the SSE plumbing already used for `audio_progress` (`pkg/gui/types.go`, `streaming_tts.go`).

### 4.2 Prefetch

When beat N starts playing, the theatre requests beat N+1's audio clip and image URL. Both are
content-addressed (`/api/audio/clip/<key>` and the scene-image URL), so a prefetch is just a fetch
the browser caches; the `Cache-Control` change from the latency work (`ETag`/immutable) makes the
prefetch actually useful.

Prefetch is bounded to one beat ahead, so a long turn does not fetch everything at once.

### 4.3 Image preload

Scene and portrait images are preloaded with `new Image()` (or a hidden `<img>`) for the next beat's
art when it differs from the current, so a scene change is a swap, not a load.

### 4.4 Retuned gap

With event-driven advance, the fixed `BEAT_GAP_MS` no longer compensates for poll latency. Reduce it
to a small, deliberate beat (for example 120 ms) so consecutive lines do not run together, and make
it a constant that the theatre's speed control scales.

### 4.5 What does not change

The order of beats, the audio that plays, the images shown, and the controls are unchanged. This is a
timing and fetching change only.

## 5. Behaviour

| Situation | Before | After |
| --- | --- | --- |
| a beat ends | advance within up to 500 ms | advance on the event |
| beat N plays | beat N+1 fetched after | beat N+1 fetched during |
| a scene change | image loads on show | image preloaded |
| between beats | 300 ms fixed | ~120 ms, scaled by speed |
| a dropped completion event | n/a | advances after the safety timeout |

## 6. Testing

- `frontend`: a completion event advances the beat; the poll is gone; the next beat is prefetched
  while the current plays; the safety timeout advances a stuck beat; the gap constant is applied and
  scaled by speed.
- `pkg/gui`: the server emits a completion event when a clip finishes and the subscription delivers
  it.
- A regression guard: the beat order and the audio keys are unchanged for a fixed turn.

## 7. Rollout

Frontend plus one additive SSE event. The poll removal is a behaviour change but not a data change.
No migration.

## 8. Risks

- **Lost events.** SSE can drop; the safety timeout is the guard, and it should be generous enough to
  never cut a real beat short.
- **Prefetch cost.** Prefetching a metered provider's clip that is never played wastes a synthesis.
  Prefetch only the **next** beat, and only its clip if it is already cached (the server can report
  cache state in the event, or the prefetch can be a HEAD/GET that the server serves from cache).
- **Browser cache.** Prefetch is only useful with the immutable cache headers from the latency work;
  if those have not landed, prefetch adds requests without saving time. Sequence TH-1 after them, or
  land the headers as part of it.
