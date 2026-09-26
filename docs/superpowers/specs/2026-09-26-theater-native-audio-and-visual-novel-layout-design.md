# Design Spec: Story Theater Native Audio & Visual-Novel Layout

**Date:** 2026-09-26
**Status:** Proposed
**Target:** `frontend` (`StoryTheater.tsx`, new `components/theater/*`, `TurnSegments.tsx`, `App.tsx`, `types.ts`)

---

## 1. Executive Summary

The in-app Story Theater has two problems:

1. **It always plays browser audio.** `StoryTheater.tsx:153-158` renders `TurnSegments` without `serverPlayback` / `onPlayTurn` / `onStopTurn`, so it falls through to `useSegmentPlayback` (`new Audio(url)`, `useSegmentPlayback.ts:37`). `App.tsx:731` also forces `autoPlay={serverAudio ? false : ...}`, so when the app has a native audio device the theater is silent. Chronicle playback already uses the application's device playback; the theater should too.
2. **It does not look like a visual novel.** The stage (`StoryTheater.tsx:110-160`) is a centered column with two plain sprite images and a scrollable card list of every segment. There is no name plate, no single-line dialogue box, no location/info bar, and only the first speech segment per turn drives sprite focus. Reference VN layouts (persona-style, gacha-style) use: full-bleed background, large left/right character sprites, a speaker name plate attached to the dialogue box, one line of dialogue at a time, and a compact info strip.

This spec wires the theater into the application's native audio when it is available (keeping browser audio as the fallback for web/socket-only sessions) and restructures the stage into a proper visual-novel layout.

---

## 2. Architecture & Data Flow

```
App.tsx
  serverAudio (GET /api/audio/status)
  turnAudioStatus: Record<number, TurnAudioState>
  handlePlayTurnAudio(turnNumber, force?) -> POST /api/game/{id}/turn/{n}/play, poll status
  handleStopAudio()
        |
        v
StoryTheater(props: serverPlayback, onPlayTurnAudio, onStopAudio, turnAudioStatus, volume, autoPlay)
        |
        +-- serverPlayback && serverAudio
        |      no browser Audio; turn audio is the master clock
        |      on turn entry -> onPlayTurnAudio(turnNumber)
        |      advance turn when status transitions playing -> idle
        |
        +-- otherwise
               browser per-segment Audio via useSegmentPlayback (unchanged)
```

---

## 3. Detailed Component Designs

### 3.1 Native audio path

**`App.tsx`** passes the existing server-playback props to `StoryTheater` (`:727-735`), mirroring `ChronicleView` (`:606-624`):
```tsx
<StoryTheater
  ...
  serverPlayback={serverAudio}
  onPlayTurnAudio={handlePlayTurnAudio}
  onStopAudio={handleStopAudio}
  turnAudioStatus={turnAudioStatus}
/>
```
`handlePlayTurnAudio` already polls `GET /api/audio/status` and maintains `turnAudioStatus` (`App.tsx:289-323`), so no new polling is needed.

**`StoryTheater.tsx` props** gain:
```ts
serverPlayback?: boolean;
onPlayTurnAudio?: (turnNumber: number, force?: boolean) => void;
onStopAudio?: () => void;
turnAudioStatus?: Record<number, TurnAudioState>;
```

**Playback behavior when `serverPlayback` is true:**
- The theater never calls `useSegmentPlayback`; it passes `serverPlayback`, `onPlayTurn`, `onStopTurn`, and `turnAudioState` through to `TurnSegments` exactly as Chronicle does.
- On entering a turn while `isPlaying`, call `onPlayTurnAudio(currentTurn.turn_number)` once. Guard with a ref keyed by turn number so re-renders do not replay.
- Turn advancement: when `turnAudioStatus[turnNumber]` has been `'playing'` and then becomes `'idle'`, advance to the next turn if still playing. The existing duration timer (`:66-78`) becomes the fallback only when `serverPlayback` is false.
- Prev/Next and Pause call `onStopAudio()` before navigating/toggling, so the device does not keep narrating a turn that is no longer on screen.
- If server playback fails, `turnAudioStatus` becomes `'error'`; show a small non-blocking error chip (`Speech error: <message>`, using the provider-error-surfacing format) and fall back to the duration timer so the theater keeps advancing.

**When `serverPlayback` is false** (web export/socket-only, or no audio device): behavior is unchanged — `useSegmentPlayback` drives per-segment browser audio, autoplay gesture handling stays.

**TurnSegments in the theater**: the VN layout no longer needs the full speech-card list, but `TurnSegments` is the component that owns the play/stop/regenerate controls and the audio state chip. Either keep a compact `TurnSegments` inside the new dialogue box (receiving the server props), or lift its controls into the new transport. The plan should prefer the former to avoid duplicating audio control logic.

### 3.2 Visual-novel layout

Replace the single `StoryTheater.tsx` stage with focused subcomponents under `frontend/src/components/theater/`:

- **`TheaterStage.tsx`** — full-bleed background plus sprites.
  - Background: `currentTurn.image_url` cover with the existing radial-gradient fallback and a scrim; optional per-turn cross-fade.
  - Sprites: protagonist stage-left (facing right), NPC stage-right mirrored (`scale-x-[-1]`), preserving the existing active-speaker emphasis (bright + scale-105 for the speaker, dimmed/brightness-75 for the listener).
  - Sprites are derived from the **active segment's** `portrait_url`, not the first speech segment of the turn, so a mid-turn speaker change swaps the right sprite.
  - Support 2–3 visible characters: if a turn's active narration involves an additional NPC with a portrait, allow a third, smaller sprite between the two, otherwise keep two.
- **`DialogueBox.tsx`** — one line of dialogue at a time.
  - Speaker name plate as a tab attached to the top of the box, tinted by speaker identity (player vs NPC accent colours already used in `TurnSegments`).
  - Small circular speaker portrait in the box (reuse `segment.portrait_url`), 40–56px.
  - Dialogue text uses `MarkdownProse`; narration renders with a neutral plate ("Narrator" or none) and both sprites dimmed.
  - Fixed glass box (`bg-stone-950/85 backdrop-blur-2xl`) at the bottom, height bounded (`min-h-[22vh] max-h-[34vh]`) rather than a scrolling list.
  - Click anywhere on the box (or a "next" chevron) advances to the next segment.
- **`TheaterInfoBar.tsx`** — top strip: location name (`currentTurn.location_name`) and `Turn N of M`, plus the close button. Optionally a small scene/time label if the system provides one.
- **`TheaterTransport.tsx`** — progress bar, prev / play-pause / next, speed, and the audio controls from `TurnSegments` (play/stop/regen) when server playback is active.

**`StoryTheater.tsx`** becomes the orchestrator: current turn index, active segment index, playback state, and composition of the four subcomponents.

#### Segment advancement

- New `useTheaterAdvance` hook (or inline state): holds `activeSegmentIndex` within the current turn.
- When playing, advance the segment after its `duration` (or a minimum readable dwell when duration is 0); advance to the next turn when segments are exhausted.
- Under server playback the turn-level audio is the master clock; segment timing still uses `duration` but is paused while `turnAudioStatus` is not `'playing'`.
- Manual prev/next moves between turns (stopping audio first); clicking the dialogue box advances segments within the turn.

#### Accessibility and reduced motion

- Dialogue advances are operable by keyboard (Enter/Space on the box, focusable transport buttons).
- Respect `prefers-reduced-motion`: disable sprite scale/fade transitions.
- Sprites are decorative (`alt=""`); the speaker name and text carry the meaning.

### 3.3 Non-goals / unchanged

- The standalone web export viewer (`pkg/export/web.go`) keeps browser HTML5 audio and its simple beat layout; it is out of scope. The export payload is unchanged.
- No new AI image generation; sprites are existing portrait assets.
- No typewriter effect requirement (a plain reveal is acceptable in v1; typewriter can be a follow-up).
- No changes to the English/upright orientation rules or the portrait prompt.

---

## 4. Test Strategy

1. **Frontend build gate**: `npm --prefix frontend run build` passes (strict TS).
2. **Manual — native audio**: with a native audio device, open the theater and confirm narration plays through the application (not the browser), Pause/Prev/Next stop audio before navigating, and the turn advances when `audioStatus` returns to idle.
3. **Manual — fallback**: with no audio device (or web export), the theater still plays browser audio and advances as before.
4. **Manual — layout**: verify full-bleed background, left/right sprites that swap when a mid-turn speaker changes, a name plate that matches the active speaker, one line shown at a time, click-to-advance, and the info bar location/turn label across several turns.
5. **Regression**: Chronicle playback is unaffected; the theater close button and Escape behavior are unchanged.
