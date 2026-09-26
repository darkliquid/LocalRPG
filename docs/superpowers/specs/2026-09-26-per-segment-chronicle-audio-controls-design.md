# Design Spec: Per-Segment Audio Controls in the Chronicle

**Date:** 2026-09-26
**Status:** Proposed
**Target:** `frontend` (`TurnSegments.tsx`, `hooks/useSegmentPlayback.ts`, `App.tsx`, `types.ts`, `api/client.ts`)

---

## 1. Executive Summary

Chronicle playback is currently turn-level. `TurnSegments` renders one whole-turn control bar (`TurnSegments.tsx:123-211`) with Play turn / Stop / Regenerate; the only per-segment affordance is clicking a speech speaker's name (`TurnSegments.tsx:84-93`), which plays that beat but offers no stop, no regenerate, and no visible state.

The backend already does everything needed per segment:

- `POST /api/game/{id}/turn/{n}/segment/{i}/play?force=1` -> `Service.PlaySegmentAudio` (`pkg/gui/server.go:373-391`, `pkg/gui/service.go:1798-1811`, force param already present).
- `GET /api/game/{id}/turn/{n}/segment/{i}/audio?force=1` -> `Service.GetSegmentAudio` (`service.go:1610`, force already present).
- `APIClient.playSegmentAudio(gameID, turn, index, force)` already passes force (`frontend/src/api/client.ts:265-269`).
- `App.handlePlayTurnAudio(turnNumber, segmentIndex?, force)` already routes to the per-segment call (`App.tsx:289-296`).

What is missing is the UI and the per-segment state model. This spec adds a hover/focus-revealed control row on **every** segment (narration and speech) with Play, Stop, and Regenerate for that individual segment's TTS, plus per-segment generating/playing/error state.

---

## 2. Architecture & Data Flow

```
hover / focus a segment
        |
        v
SegmentAudioControls (Play | Stop | Regenerate)
        |
        +-- serverPlayback:  onPlaySegment(turn, i)           -> App.handlePlayTurnAudio(turn, i)
        |                    onStopSegment()                  -> App.handleStopAudio()
        |                    onRegenerateSegment(turn, i)     -> App.handlePlayTurnAudio(turn, i, force=true)
        |                    status: segmentAudioStatus["turn:i"]
        |
        +-- browser:         playFrom(i) / stop()
                             regenerateFrom(i)  -> GET .../segment/{i}/audio?force=1 -> Audio(url)
                             status: local per-index playback state
```

---

## 3. Detailed Component Designs

### 3.1 Per-segment state

Today `turnAudioStatus` is keyed only by turn number (`App.tsx:68`). Add a parallel map for segment-specific status:

```ts
// key is `${turnNumber}:${segmentIndex}`
const [segmentAudioStatus, setSegmentAudioStatus] =
  useState<Record<string, { state: TurnAudioState; message?: string }>>({});
```

`handlePlayTurnAudio(turnNumber, segmentIndex?, force)` sets the keyed entry:
- `segmentIndex === undefined` -> `turnAudioStatus[turnNumber]` (unchanged).
- `segmentIndex` defined -> `segmentAudioStatus[`${turnNumber}:${segmentIndex}`]`.
- The existing 500 ms `APIClient.audioStatus()` poll (`App.tsx:301-317`) clears the correct key on idle/error. `handleStopAudio` (`App.tsx:325-340`) clears both maps.
- Pass `segmentAudioStatus` down through `ChronicleView` to `TurnSegments`, alongside the existing `turnAudioStatus`.

Keep the turn-level map: the whole-turn controls continue to work and should not be broken by a segment-level action.

### 3.2 `TurnSegments` hover controls

Every segment is wrapped so it can host controls:

- Speech cards (`:60-111`) already have a container; narration (`:112-120`) is a bare `MarkdownProse`. Wrap both in a `group relative` div (the narration wrapper must not change text layout).
- Render `SegmentAudioControls` inside each wrapper, absolutely positioned (e.g. `top-2 right-2`) or as a compact inline row revealed on hover, so showing/hiding causes no layout shift:
  ```
  opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-opacity
  ```
- Controls appear only when the segment has audio (`!!segment.audio_url`, i.e. TTS enabled). Segments without audio keep no controls, matching the current `hasAudio` gating.

`SegmentAudioControls` (new small component, or an inline block):
| Button | Icon | Server action | Browser action |
|---|---|---|---|
| Play | `Play` | `onPlaySegment?.(i)` | `playFrom(i)` |
| Stop | `Square` | `onStopTurn?.()` | `stop()` |
| Regenerate | `RotateCw` | `onPlaySegment?.(i, true)` | `regenerateFrom(i)` |

- While the segment is `generating`: disable all three, show `Loader2` on the Play slot.
- While the segment is `playing`: Stop enabled (rose styling, as the turn-level Stop), Play disabled, Regenerate disabled. (Because device playback is global, only one segment/whole-turn is "playing" at a time; opening another segment's Play implicitly stops the previous one through `PlaySegmentAudio` replacing the queue.)
- On `error`: Play becomes "Retry", Regenerate enabled, and the message is shown in the existing `Speech error: <message>` chip vocabulary (see the provider-error-surfacing spec).
- `title`/`aria-label` name the action and the segment (`"Play this line"`, `"Regenerate this line"`).

The existing speaker-name click-to-play (`:84-93`) can stay as a convenience shortcut; it should reuse the same handler so its state is driven by the same per-segment status.

### 3.3 App wiring

New props on `TurnSegments` (Chronicle already holds the handlers via `ChronicleView`):
```ts
onPlaySegment?: (segmentIndex: number, force?: boolean) => void;
onStopSegment?: () => void;
segmentAudioStatus?: Record<string, { state: TurnAudioState; message?: string }>;
turnNumber?: number; // needed to key segmentAudioStatus
```
`App.tsx` passes `onPlaySegment={(i, force) => handlePlayTurnAudio(turnNumber, i, force)}` and `onStopSegment={handleStopAudio}` through `ChronicleView`.

Alternatively, reuse the existing generic `onPlayTurn(segmentIndex?, force?)` prop and only add the status map + turn number; that is the smaller change. The plan should prefer reusing `onPlayTurn`.

### 3.4 Browser-mode regenerate

`useSegmentPlayback` (`frontend/src/hooks/useSegmentPlayback.ts`) plays `segment.audio_url` clips directly. For per-segment regenerate in browser mode it needs:
- `regenerateFrom(index)`: `fetch(`/api/game/${gameId}/turn/${turn}/segment/${index}/audio?force=1`)`, then play the returned blob/URL for that index.
- A cache-bust for the non-force paths after a regenerate: accept an optional `versions: Record<number, number>` and append `&t=${versions[i]}` to the URL so a regenerated clip is not served from the browser cache. The server already sets the content-addressed cache and returns fresh bytes when `force=1`.

If adding these to the shared hook is disproportionate, the segment Regenerate button may always route through the server force endpoint and then `audioStatus`/blob playback; but the hook extension keeps browser and server behavior consistent.

### 3.5 Interaction with inline check cards

The inline `DiceCheckCard` (see the chronicle-inline-check-results spec) is not TTS-able and gets no hover controls. Only `narration` and `speech` segments do.

---

## 4. Non-Goals

- No queuing/auto-advance of per-segment playback (that is turn-level / Story Theater behavior).
- No per-segment voice override or prompt editing.
- No change to the `history.jsonl` or index formats; audio is derived from the existing segment text.
- No backend changes: the force-capable endpoints already exist. This spec is frontend-only unless the browser regenerate hook extension is deemed in scope.

---

## 5. Test Strategy

1. **Frontend build gate**: `npm --prefix frontend run build` passes (strict TS).
2. **Manual — native device**: hover a narration and a speech segment, Play plays only that line, Stop halts it, Regenerate re-synthesizes that line; the spinner shows while generating; the whole-turn controls still work and reflect the same global playing state.
3. **Manual — browser mode**: the same three actions work via `useSegmentPlayback`, and a regenerated line is not served from the browser cache.
4. **Manual — states**: error case shows the provider message and Retry; only one segment shows "playing" at a time.
5. **Accessibility**: Tab focus reveals the controls (`:focus-within`), and each button has an accessible label naming the action and line.
