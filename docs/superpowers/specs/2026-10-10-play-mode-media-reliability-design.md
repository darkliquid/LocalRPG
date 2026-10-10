# Play Mode Media Reliability Design

**Date:** 2026-10-10
**Status:** Proposed
**Issue:** [#130](https://github.com/darkliquid/LocalRPG/issues/130)
**Epic:** [#123 Generation, playback, and roll resilience](https://github.com/darkliquid/LocalRPG/issues/123)
**Depends on:** [Theater Performance Design](2026-10-05-theatre-performance-design.md), [Provider Manager Completion Design](2026-10-06-provider-manager-completion-design.md)
**Scope:** `frontend`, `pkg/gui`, `pkg/media/playback`

---

## 1. Problem

Two play-mode media faults share one root cause: a failure with no path back to the
user.

### 1.1 Broken character and NPC portraits

Every portrait in play mode is a bare `<img>` with no error handling and no placeholder:

| Site | Lines |
| --- | --- |
| `TurnSegments.tsx` speech chip | 188-193 |
| `theater/TheaterStage.tsx` player and NPC | 127-132, 156-161 |
| `theater/TheaterDialogue.tsx` | 79-84 |
| `ImageLightbox.tsx` | 43-47 |
| `ChronicleView.tsx` scene illustration | 128 |

None has an `onError`; a project-wide search finds `onError` only on generation buttons,
on zero `<img>` portrait elements. When the URL is non-empty but the fetch returns a
non-image, the browser paints the broken-image glyph and nothing replaces it.

The URL is fabricated for any speaker. `pkg/gui/service.go:604-621` builds
`/api/game/{id}/character/{refID}/portrait` from the speaker id, the segment's
`SpeakerPortrait`, or `entity.Slugify(speaker)`. The server route
(`pkg/gui/server.go:891-933`) calls `GetCharacterPortrait`
(`pkg/gui/service.go:2752-2862`), which returns a **procedural SVG fallback** for a known
character with no portrait file (`service.go:2859-2861`) but returns an error for an
**unknown** id (`service.go:2778-2780`), which `writeGameError` renders as a plain-text
`404`/`500`. An `<img>` given a text body is a broken image.

The client makes this worse: `App.tsx:319` appends `?t=${Date.now()}` unconditionally, so
the URL changes on every render and a failure reappears intermittently;
`StoryTheater.tsx:103-105` fabricates `/api/game/{gameId}/character/{playerId}/portrait`
even when the player has no portrait; and the async portrait-generation race
(`App.tsx:284-377`) can request before the entity exists.

### 1.2 TTS failures are invisible

A play request (`POST /api/game/{id}/turn/{n}/play`, `client.ts:514`) returns `204`
immediately, and completion arrives on the SSE stream `/api/audio/events`
(`server.go:1764`) as `AudioStatusDTO{available, playing, turn, segment, owner}`
(`pkg/gui/types.go:1161-1170`). **Nothing carries a failure.** Every asynchronous failure
is silent:

- Synthesis errors are swallowed with `continue` inside `emitTurnClips`
  (`pkg/gui/service.go:3290-3293`, `:3306-3308`), so a beat simply goes quiet.
- Grouped synthesis can return no clips and no error (`service.go:3183-3197`), which the
  regenerate endpoint turns into `204 No Content` (`server.go:863-864`).
- The audio device silently skips a clip it cannot decode (`pkg/media/playback/player.go:168-179`,
  `queue.go:95-98`), and `queueStreamer.Err()` returns `nil` unconditionally (`queue.go:155`).
- A full queue drops a clip with no error (`pkg/gui/turn_audio.go:161-177`).
- The streaming pre-synthesizer counts failures but exposes only a count to the client
  (`pkg/gui/streaming_tts.go:151-177`, `:240-254`).

On the client, the failure is then converted to success: `App.tsx:709` sets
`source.onerror = () => finish()`, so any SSE error ends the beat as `idle`, and a
`60_000` ms safety timeout (`App.tsx:712`) does the same if nothing arrives.
`useSegmentPlayback.ts:87-94` discards the `audio.play()` rejection and reports a
blocked/undecodable clip as a generic autoplay "blocked" hint; the regenerate handler
catches to `console.error` only (`:125-137`); `useStreamedSpeech.ts:112-114` calls
`advance(false)` without the reason. `SegmentAudioControls.tsx:69-73` will show an error,
but only when one is handed to it, which never happens for a device-side failure.

## 2. Goals

- A portrait that is missing, unknown, or fails to load renders a deterministic
  placeholder, never a broken-image glyph.
- A TTS failure, wherever it happens, reaches the play UI with a reason and a retry.
- A successful-but-silent beat is distinguishable from a failed one.
- No behaviour change to a healthy turn.

## 3. Non-goals

- Changing the audio pipeline's quality, latency, or provider set.
- Redesigning the theater or the audio controls.
- Changing the synchronous error envelope, which already works.

## 4. Design

### 4.1 Portraits

**A shared `EntityAvatar` component.** `components/EntityAvatar.tsx`:

```ts
interface EntityAvatarProps {
  src?: string;
  name: string;
  className?: string;
  alt?: string;
}
```

It renders the `<img src={src} onError={() => setFailed(true)}>` when `src` is set and
has not failed, and otherwise a deterministic initials avatar: the first letters of the
name over a colour derived from a small FNV-1a hash of the name (the same hash family
`AssignVoiceProfile` uses, so a character's colour is stable across sessions). It is a
pure presentational component with no fetch of its own.

All five sites above switch to `EntityAvatar`. A missing `src` and a failed `src` look
the same, which is what the user wants: a character with no art is still a character.

**Do not fabricate a URL for an unknown entity.** `StoryTheater.tsx:103-105` stops
building a player URL from `playerId` and instead reads the portrait map by id, passing
`undefined` when absent. `App.tsx:317` already gates on `has_portrait`; the
`Date.now()` cache-buster (`App.tsx:319`) becomes the portrait's version from the SSE
`portrait` event (`App.tsx:438-445`), so the URL is stable except when a new portrait is
generated.

**The server always answers with an image.** In `server.go:891-933`, an unknown
character id returns the procedural placeholder SVG with `200 image/svg+xml` (derived
from the id via `media.PortraitRequestFor` on a synthetic entity) instead of a
`writeGameError`; an internal error is logged and also returns the placeholder, so an
`<img>` never receives text. Invalid ids still return `400`. The export player already
degrades to no image (`pkg/export/web.go:344-355`) and is unchanged.

### 4.2 A failure channel for audio

**Extend the status payload.** `AudioStatusDTO` gains two optional fields, which is
backward compatible with the client's existing parse:

```go
type AudioStatusDTO struct {
	Available bool   `json:"available"`
	Playing   bool   `json:"playing"`
	Turn      int    `json:"turn"`
	Segment   int    `json:"segment"`
	Owner     string `json:"owner,omitempty"`
	// Error carries a human-readable reason for a failed or aborted beat. It is
	// empty on success, so a client can distinguish silence from failure.
	Error string `json:"error,omitempty"`
	// Stage names where the failure happened: synthesize, decode, queue, configure.
	Stage string `json:"stage,omitempty"`
}
```

**Broadcast the reason at the source.** `emitTurnClips` stops `continue`-ing on a
synthesis error: it records the first error and calls
`broadcastAudioStatus(AudioStatusDTO{Playing: false, Turn: t, Segment: i, Error: err.Error(), Stage: "synthesize", Owner: ownerDevice})`.
The grouped-synthesis empty result (`service.go:3196`) does the same with
`Stage: "synthesize"` and "the provider produced no audio". `turn_audio.go`'s full-queue
drop broadcasts `Stage: "queue"`. `PlayTurnAudio` gains a preflight: if TTS is disabled
or unconfigured, it broadcasts `Stage: "configure"`, "No TTS provider is configured",
and returns rather than a silent no-op.

**Report a decode failure.** `pkg/media/playback` stops swallowing: `PlayFiles`
(`player.go:168-179`) and `queueStreamer` (`queue.go:95-98`) capture the first
`decodeFile` error, and the playback session that owns the device broadcasts it with
`Stage: "decode"`. `queueStreamer.Err()` returns that captured error instead of always
`nil`. This is the one place the device path gains an error channel, and it is where a
clip that the provider wrote but the host cannot decode is surfaced.

**The streaming path reports its first failure.** `streaming_tts.go`'s progress event
gains a `message` alongside the existing `failed_count`, so the pre-synthesizer's
`provisional_error` trace (`:240-254`) is also visible to the client.

### 4.3 The client surfaces it

- `handlePlayTurnAudio` (`App.tsx:665-723`) parses `error` from each SSE message. On a
  non-empty `error`, it calls `setStatus({ state: 'error', message: data.error })` and
  ends the subscription. A completion (`playing: false`) with no error is the only path
  to `idle`.
- `source.onerror` no longer means success. The handler tracks whether a terminal
  message arrived; if the stream errors first, it sets an error state ("The audio stream
  ended unexpectedly.") and ends. The 60 s safety timeout likewise sets an error state
  ("Audio timed out."), so a stalled beat is visible rather than silently idle.
- `useSegmentPlayback.ts` keeps the failure it currently discards: it exposes
  `{ state: 'error', message }`, mapping a browser autoplay block to the existing
  "browser needs a click" hint and any other `audio.play()` rejection (or a media-element
  `error` event) to its reason. Regenerate failures call an `onError` callback instead of
  `console.error`, and an empty result reports "no audio was produced".
- `useStreamedSpeech.ts` passes the `onerror` reason into `advance(false, reason)` so the
  beat's status carries it.
- `SegmentAudioControls` renders the retry action it already offers plus the message,
  and the theater transport shows the same. No new surface is invented; the error state
  that exists (`SegmentAudioControls.tsx:69-73`, `TurnSegments.tsx:293-298`,
  `TheaterTransport.tsx:125-130`) is finally given content.

## 5. Behaviour

| Action | Before | After |
| --- | --- | --- |
| A speaker with no portrait file | broken glyph | initials avatar |
| A speaker with an unknown id | broken glyph | initials avatar (server placeholder) |
| A portrait that 404s mid-turn | broken glyph, reappears | initials avatar, stable |
| TTS synthesis fails mid-turn | beat goes quiet, then "idle" | "Audio failed: <reason>", with retry |
| A clip the host cannot decode | silently skipped | "Audio failed: unsupported format" |
| The audio stream drops | beat marked done | "The audio stream ended unexpectedly" |
| TTS is unconfigured | silent no-op | "No TTS provider is configured" |
| A browser blocks autoplay | generic "blocked" hint | unchanged, distinguished from a real failure |
| A healthy turn | plays | plays, unchanged |

## 6. Testing

- `frontend`: vitest for `EntityAvatar` (renders `src`, falls back on `onError`, falls
  back with no `src`, colour is stable for a name); `handlePlayTurnAudio` sets an error
  state from an SSE `error` and no longer treats `onerror` as success;
  `useSegmentPlayback` surfaces a `play()` rejection reason.
- `pkg/gui`: a synthesis failure broadcasts an `AudioStatusDTO` with `Error` and
  `Stage: "synthesize"` (extend `turn_audio_test.go`, `streaming_tts_test.go`); an
  unknown character id returns `image/svg+xml` `200` (extend `character_portrait_test.go`);
  a no-TTS-config play request broadcasts `Stage: "configure"`.
- `pkg/media/playback`: `queueStreamer.Err()` reports a captured decode error.
- An end-to-end check with a deliberately broken clip path confirms the message reaches
  the theater.

## 7. Rollout

Two independent halves. Portrait fallback is frontend plus one server branch and can
ship alone. The audio error channel is server-first (the payload and the broadcasts),
then the client, then the player package; the payload change is additive, so an older
client ignores `error` and behaves as it does today.

## 8. Risks

- **`source.onerror` also fires on a normal close.** The design keys on whether a
  terminal message arrived, not on the raw event, so a clean end is not misreported as a
  failure. This is the one subtle change and is covered by a test.
- **A placeholder can mask a real outage.** An unknown id and a server error both render
  the placeholder; the server logs the internal error, and the trace keeps it, so the
  transparency is in the diagnosis path rather than the UI.
- **An error channel can be noisy.** Only the first failure per beat is broadcast, so a
  turn with several bad clips does not spam the client.
- **The device path is shared.** `pkg/media/playback` is used by export and the CLI too;
  capturing `Err()` adds information without changing the skip-and-continue behaviour for
  callers that ignore it.
