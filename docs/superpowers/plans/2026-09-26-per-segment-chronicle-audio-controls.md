# Per-Segment Chronicle Audio Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show Play / Stop / Regenerate controls for one segment's TTS when hovering or focusing that segment in the Chronicle, with per-segment status.

**Architecture:** Add a `SegmentAudioControls` component and render it inside a `group relative` wrapper on every narration/speech segment. TurnSegments already owns a generic `onPlayTurn(segmentIndex?, force?)`; add a per-segment status map keyed `${turn}:${index}` and a browser-mode `regenerateFrom` on `useSegmentPlayback`. `App` maintains the segment status map and routes per-segment Play/Stop/Regenerate through the existing force-capable endpoints.

**Tech Stack:** React 19, TypeScript (strict), Tailwind v4, `lucide-react`. Backend endpoints already exist (`POST .../segment/{i}/play?force=1`, `GET .../segment/{i}/audio?force=1`); this is frontend-only.

**Spec:** `docs/superpowers/specs/2026-09-26-per-segment-chronicle-audio-controls-design.md`

## Global Constraints

- TypeScript `strict`, `noUnusedLocals`, `noUnusedParameters`; remove unused imports or the build fails.
- Typecheck gate per task: `mise run test:frontend` (runs `npx tsc --noEmit` in `frontend/`).
- Do not nest buttons; the controls are separate elements overlaying the segment.
- The whole-turn controls must keep working; do not remove them.
- Only narration and speech segments get controls (inline check cards in a later spec do not).
- Do not commit unless the user asks.

---

## File Map

- Modify: `frontend/src/hooks/useSegmentPlayback.ts` — `playingIndex`, `regenerateFrom`, shared `playUrl`.
- Create: `frontend/src/components/SegmentAudioControls.tsx` — the hover control row.
- Modify: `frontend/src/components/TurnSegments.tsx` — `segmentAudioKey` export, new props, wrappers, controls.
- Modify: `frontend/src/components/ChronicleView.tsx` — pass `turnNumber` + `segmentAudioStatus`.
- Modify: `frontend/src/App.tsx` — segment status state and per-segment handler.

---

### Task 1: `useSegmentPlayback` per-segment tracking and regeneration

**Files:**
- Modify: `frontend/src/hooks/useSegmentPlayback.ts`

**Interfaces:**
- Produces: `useSegmentPlayback(segments, autoPlay, volume)` now also returns `playingIndex: number | null` and `regenerateFrom(index: number): void`.

- [x] **Step 1: Add `playingIndex` and a shared `playUrl`**

Replace the body from `const [blocked, ...]` through the end of `playFrom` with a version that tracks the active index and exposes a shared `playUrl`:

```ts
  const [blocked, setBlocked] = useState(false);
  const [playingIndex, setPlayingIndex] = useState<number | null>(null);

  const stop = useCallback(() => {
    audioRef.current?.pause();
    audioRef.current = null;
    setPlaying(false);
    setPlayingIndex(null);
  }, []);

  const playUrl = useCallback(
    (url: string, index: number, onEnded?: () => void) => {
      audioRef.current?.pause();
      const audio = new Audio(url);
      audio.volume = volume;
      audio.onended = () => {
        setPlayingIndex(null);
        onEnded?.();
      };
      audioRef.current = audio;
      setPlaying(true);
      setPlayingIndex(index);
      audio
        .play()
        .then(() => setBlocked(false))
        .catch(() => {
          setPlaying(false);
          setPlayingIndex(null);
          setBlocked(true);
        });
    },
    [volume]
  );

  const playFrom = useCallback(
    (index: number) => {
      const urls = (segments ?? []).map((segment) => segment.audio_url);
      const next = urls.findIndex((url, i) => i >= index && !!url);
      if (next === -1) {
        stop();
        return;
      }

      const following = urls.findIndex((url, i) => i > next && !!url);
      if (following !== -1) {
        const prefetch = new Audio(urls[following] as string);
        prefetch.preload = 'auto';
      }

      playUrl(urls[next] as string, next, () => playFrom(next + 1));
    },
    [segments, stop, playUrl]
  );

  // regenerateFrom re-synthesizes one segment on demand. The server accepts
  // force=1 on the audio GET; the timestamp defeats the browser cache.
  const regenerateFrom = useCallback(
    (index: number) => {
      const url = (segments ?? [])[index]?.audio_url;
      if (!url) return;
      const separator = url.includes('?') ? '&' : '?';
      playUrl(`${url}${separator}force=1&t=${Date.now()}`, index);
    },
    [segments, playUrl]
  );
```

- [x] **Step 2: Return the new values**

Change the return to:

```ts
  return { playing, blocked, playingIndex, play, playFrom, regenerateFrom, stop };
```

- [x] **Step 3: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 2: `SegmentAudioControls` component

**Files:**
- Create: `frontend/src/components/SegmentAudioControls.tsx`

**Interfaces:**
- Consumes: `TurnAudioState` (type-only) from `./TurnSegments`.
- Produces: `<SegmentAudioControls state message? onPlay onStop onRegenerate />`.

- [x] **Step 1: Create the component**

```tsx
import React from 'react';
import { Loader2, Play, RotateCw, Square } from 'lucide-react';
import type { TurnAudioState } from './TurnSegments';

interface SegmentAudioControlsProps {
  state: TurnAudioState;
  message?: string;
  onPlay: () => void;
  onStop: () => void;
  onRegenerate: () => void;
}

export const SegmentAudioControls: React.FC<SegmentAudioControlsProps> = ({
  state,
  message,
  onPlay,
  onStop,
  onRegenerate,
}) => {
  const generating = state === 'generating';
  const playing = state === 'playing';

  return (
    <div className="absolute top-2 right-2 z-10 flex items-center gap-1 rounded-lg border border-white/10 bg-stone-950/85 backdrop-blur px-1 py-0.5 shadow-lg opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-opacity">
      <button
        type="button"
        onClick={onPlay}
        disabled={generating || playing}
        className={`p-1 rounded transition-colors ${
          generating || playing
            ? 'text-stone-600 cursor-not-allowed'
            : 'text-stone-300 hover:text-purple-300 cursor-pointer'
        }`}
        title="Play this line"
        aria-label="Play this line"
      >
        {generating ? <Loader2 className="w-3 h-3 animate-spin" /> : <Play className="w-3 h-3" />}
      </button>
      <button
        type="button"
        onClick={onStop}
        disabled={!playing}
        className={`p-1 rounded transition-colors ${
          playing ? 'text-rose-400 hover:bg-rose-500/20 cursor-pointer' : 'text-stone-600 cursor-not-allowed'
        }`}
        title="Stop this line"
        aria-label="Stop this line"
      >
        <Square className="w-3 h-3" />
      </button>
      <button
        type="button"
        onClick={onRegenerate}
        disabled={generating || playing}
        className={`p-1 rounded transition-colors ${
          generating || playing
            ? 'text-stone-600 cursor-not-allowed'
            : 'text-stone-300 hover:text-amber-300 cursor-pointer'
        }`}
        title="Regenerate this line"
        aria-label="Regenerate this line"
      >
        <RotateCw className="w-3 h-3" />
      </button>
      {state === 'error' && message && (
        <span className="max-w-[140px] truncate text-[10px] text-rose-300" title={message}>
          Error: {message}
        </span>
      )}
    </div>
  );
};
```

- [x] **Step 2: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 3: Wire controls into `TurnSegments`

**Files:**
- Modify: `frontend/src/components/TurnSegments.tsx`

**Interfaces:**
- Produces: `segmentAudioKey(turnNumber: number, segmentIndex: number): string`.
- Consumes: `SegmentAudioControls`, `useSegmentPlayback`'s `playingIndex`/`regenerateFrom`.

- [x] **Step 1: Add the import and key helper**

After the `lucide-react` import:

```tsx
import { SegmentAudioControls } from './SegmentAudioControls';

export const segmentAudioKey = (turnNumber: number, segmentIndex: number): string =>
  `${turnNumber}:${segmentIndex}`;
```

- [x] **Step 2: Add props**

Add to `TurnSegmentsProps` after `turnAudioMessage`:

```tsx
  turnNumber?: number;
  segmentAudioStatus?: Record<string, { state: TurnAudioState; message?: string }>;
```

Add to the destructure after `turnAudioMessage`:

```tsx
  turnNumber,
  segmentAudioStatus,
```

- [x] **Step 3: Pull the new hook values**

Change the hook destructure to:

```tsx
  const { playing, blocked, playingIndex, play, playFrom, regenerateFrom, stop } = useSegmentPlayback(
    segments,
    autoPlay && hasAudio && !serverPlayback,
    volume
  );
```

- [x] **Step 4: Add the per-segment controls builder**

After the `isError` line:

```tsx
  const segmentState = (index: number): { state: TurnAudioState; message?: string } => {
    if (serverPlayback) {
      const key = turnNumber !== undefined ? segmentAudioKey(turnNumber, index) : '';
      return segmentAudioStatus?.[key] ?? { state: 'idle' };
    }
    return playingIndex === index ? { state: 'playing' } : { state: 'idle' };
  };

  const segmentControls = (index: number) => {
    if (!ordered[index]?.audio_url) return null;
    const status = segmentState(index);
    return (
      <SegmentAudioControls
        state={status.state}
        message={status.message}
        onPlay={() => (serverPlayback ? startServerPlayback(index) : playFrom(index))}
        onStop={() => (serverPlayback ? stopServerPlayback() : stop())}
        onRegenerate={() => (serverPlayback ? startServerPlayback(index, true) : regenerateFrom(index))}
      />
    );
  };
```

- [x] **Step 5: Add `group relative` and render controls in the speech card**

Change the speech card opening div className to start with `group relative `:

```tsx
            className={`group relative bg-glass-card border-l-4 pl-4 py-3 pr-4 rounded-r-xl shadow-lg space-y-2 ${
              segment.player ? 'border-sky-400/90' : 'border-purple-500/90'
            }`}
```

Immediately after that opening div, render the controls:

```tsx
            {segmentControls(i)}
```

- [x] **Step 6: Wrap the narration and render controls**

Replace the narration branch:

```tsx
        ) : (
          <div key={i} className="group relative">
            {segmentControls(i)}
            <MarkdownProse
              text={segment.text}
              onEntityClick={onEntityClick}
              displayMode={displayMode}
              className="text-stone-200 text-xl leading-relaxed tracking-wide font-serif space-y-4"
            />
          </div>
        )
```

- [x] **Step 7: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 4: Pass-through in `ChronicleView`

**Files:**
- Modify: `frontend/src/components/ChronicleView.tsx`

- [x] **Step 1: Add the prop**

Add to `ChronicleViewProps` after `turnAudioStatus`:

```tsx
  segmentAudioStatus?: Record<string, { state: TurnAudioState; message?: string }>;
```

Add to the destructure after `turnAudioStatus = {}`:

```tsx
  segmentAudioStatus = {},
```

- [x] **Step 2: Forward to TurnSegments**

In the `TurnSegments` render, add:

```tsx
                turnNumber={turn.turn_number}
                segmentAudioStatus={segmentAudioStatus}
```

- [x] **Step 3: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 5: `App` per-segment state and handler

**Files:**
- Modify: `frontend/src/App.tsx`

- [x] **Step 1: Import the key helper**

Change the existing import to:

```tsx
import { TurnAudioState, segmentAudioKey } from './components/TurnSegments';
```

- [x] **Step 2: Add the segment status state and keyed polling ref**

After `turnAudioStatus`:

```tsx
  // Per-segment audio status, keyed `${turn}:${index}`.
  const [segmentAudioStatus, setSegmentAudioStatus] = useState<Record<string, { state: TurnAudioState; message?: string }>>({});
```

Change `audioPollingRef` to a string key:

```tsx
  const audioPollingRef = useRef<Record<string, ReturnType<typeof setInterval>>>({});
```

- [x] **Step 3: Route per-segment status in `handlePlayTurnAudio`**

Replace `handlePlayTurnAudio` with:

```tsx
  const handlePlayTurnAudio = (turnNumber: number, segmentIndex?: number, force = false) => {
    if (!activeGameID) return;
    const key = segmentIndex === undefined ? null : segmentAudioKey(turnNumber, segmentIndex);
    const setStatus = (entry: { state: TurnAudioState; message?: string }) => {
      if (key === null) {
        setTurnAudioStatus((prev) => ({ ...prev, [turnNumber]: entry }));
      } else {
        setSegmentAudioStatus((prev) => ({ ...prev, [key]: entry }));
      }
    };

    setStatus({ state: 'generating' });

    const call = segmentIndex === undefined
      ? APIClient.playTurnAudio(activeGameID, turnNumber, force)
      : APIClient.playSegmentAudio(activeGameID, turnNumber, segmentIndex, force);

    const pollKey = key ?? `turn:${turnNumber}`;

    call
      .then(() => {
        setStatus({ state: 'playing' });
        const intervalId = setInterval(() => {
          APIClient.audioStatus()
            .then((status) => {
              if (!status.playing) {
                clearInterval(intervalId);
                delete audioPollingRef.current[pollKey];
                setStatus({ state: 'idle' });
              }
            })
            .catch(() => {
              clearInterval(intervalId);
              delete audioPollingRef.current[pollKey];
              setStatus({ state: 'idle' });
            });
        }, 500);
        audioPollingRef.current[pollKey] = intervalId;
      })
      .catch((err: unknown) => {
        const message = err instanceof Error ? err.message : String(err);
        setStatus({ state: 'error', message });
      });
  };
```

- [x] **Step 4: Clear segment status on stop**

Replace `handleStopAudio` with:

```tsx
  const handleStopAudio = () => {
    APIClient.stopAudio().catch(console.error);
    for (const [key, intervalId] of Object.entries(audioPollingRef.current)) {
      clearInterval(intervalId);
      delete audioPollingRef.current[key];
    }
    setTurnAudioStatus((prev) => {
      const next = { ...prev };
      for (const key of Object.keys(next)) {
        if (next[Number(key)].state === 'playing' || next[Number(key)].state === 'generating') {
          next[Number(key)] = { state: 'idle' };
        }
      }
      return next;
    });
    setSegmentAudioStatus({});
  };
```

- [x] **Step 5: Pass the map to ChronicleView**

Add to the `ChronicleView` props:

```tsx
                    segmentAudioStatus={segmentAudioStatus}
```

- [x] **Step 6: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 6: Verification

- [x] **Step 1: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

- [x] **Step 2: Build**

Run: `mise run build:frontend`
Expected: `tsc` + `vite build` succeed; `.gitkeep` preserved.

- [x] **Step 3: Manual checklist**

With a native audio device: hover a narration and a speech segment, Play plays only that line, Stop halts, Regenerate re-synthesizes, spinner shows while generating, and the whole-turn controls still work. With no device (browser mode): the same actions work through `useSegmentPlayback`, and segment Play/Stop reflect the active line. Tab focus reveals the controls.

---

## Self-Review

**Spec coverage:** hover controls (Task 3 + 2), per-segment status map (Task 5 + 3), server force path reused via `onPlayTurn(i, true)` (Tasks 3/5), browser `regenerateFrom` + cache-bust (Task 1), pass-through (Task 4), no controls on check cards (Task 3 gates on `audio_url`). Whole-turn controls preserved.

**Placeholder scan:** none.

**Type consistency:** `segmentAudioKey` is defined in Task 3 Step 1 and used in Tasks 3 and 5; `playingIndex`/`regenerateFrom` names match between Task 1 and Task 3; `SegmentAudioControls` prop names match Task 2 and Task 3; `segmentAudioStatus` shape is identical across Tasks 3, 4, and 5.
