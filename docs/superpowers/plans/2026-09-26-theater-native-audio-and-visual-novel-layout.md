# Theater Native Audio & Visual-Novel Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Play the Story Theater through the application's audio device when one exists, and restructure the stage into a visual-novel layout that shows one line at a time.

**Architecture:** `App` passes its existing server-playback props to `StoryTheater`. The theater starts turn audio on the device (watching `turnAudioStatus` to advance) and keeps browser playback as the fallback. The stage is split into `theater/TheaterStage`, `theater/TheaterDialogue`, and `theater/TheaterTransport`, orchestrated by `StoryTheater`.

**Tech Stack:** React 19, TypeScript (strict), Tailwind v4, `lucide-react`.

**Spec:** `docs/superpowers/specs/2026-09-26-theater-native-audio-and-visual-novel-layout-design.md`

## Global Constraints

- TypeScript `strict`, `noUnusedLocals`, `noUnusedParameters`.
- No browser audio when `serverPlayback` is true and a device is available; the browser path is the fallback only.
- The standalone web export viewer is out of scope and unchanged.
- Do not nest interactive elements (a click-to-advance panel uses `role="button"`, not `<button>`, because wikilinks render controls inside it).
- Gate: `mise run test:frontend` and `mise run build:frontend`.
- Do not commit unless the user asks.

---

## File Map

- Create: `frontend/src/components/theater/TheaterStage.tsx`
- Create: `frontend/src/components/theater/TheaterDialogue.tsx`
- Create: `frontend/src/components/theater/TheaterTransport.tsx`
- Rewrite: `frontend/src/components/StoryTheater.tsx`
- Modify: `frontend/src/App.tsx` (theater props)

---

### Task 1: `TheaterStage` component

**Files:**
- Create: `frontend/src/components/theater/TheaterStage.tsx`

- [x] **Step 1: Create the component**

```tsx
import React from 'react';

interface TheaterStageProps {
  backgroundURL?: string;
  playerPortrait?: string;
  npcPortrait?: string;
  playerActive?: boolean;
  npcActive?: boolean;
}

// TheaterStage is the visual-novel backdrop: a full-bleed scene with the
// protagonist on the left and the current NPC mirrored on the right. Only the
// active speaker is lit; the other is dimmed.
export const TheaterStage: React.FC<TheaterStageProps> = ({
  backgroundURL,
  playerPortrait,
  npcPortrait,
  playerActive = false,
  npcActive = false,
}) => (
  <>
    <div
      className="absolute inset-0 bg-cover bg-center transition-all duration-700 pointer-events-none"
      style={{
        backgroundImage: backgroundURL
          ? `url(${backgroundURL})`
          : 'radial-gradient(ellipse at center, #261e1b 0%, #0c0a09 100%)',
      }}
    />
    <div className="absolute inset-0 bg-gradient-to-t from-black/90 via-black/45 to-black/60 pointer-events-none" />

    <div className="absolute inset-x-0 top-16 bottom-[30vh] z-10 flex items-end justify-between px-[5vw] pointer-events-none">
      {playerPortrait ? (
        <img
          src={playerPortrait}
          alt=""
          aria-hidden="true"
          className={`max-h-[64vh] w-auto object-contain origin-bottom transition-all duration-500 ${
            playerActive
              ? 'opacity-100 scale-105 drop-shadow-[0_10px_25px_rgba(56,189,248,0.35)]'
              : 'opacity-40 brightness-75 scale-95'
          }`}
        />
      ) : (
        <span />
      )}
      {npcPortrait ? (
        <img
          src={npcPortrait}
          alt=""
          aria-hidden="true"
          className={`max-h-[64vh] w-auto object-contain origin-bottom scale-x-[-1] transition-all duration-500 ${
            npcActive
              ? 'opacity-100 scale-105 drop-shadow-[0_10px_25px_rgba(168,85,247,0.35)]'
              : 'opacity-40 brightness-75 scale-95'
          }`}
        />
      ) : (
        <span />
      )}
    </div>
  </>
);
```

- [x] **Step 2: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 2: `TheaterDialogue` component

**Files:**
- Create: `frontend/src/components/theater/TheaterDialogue.tsx`

- [x] **Step 1: Create the component**

```tsx
import React from 'react';
import { TurnSegment } from '../../types';
import { MarkdownProse } from '../MarkdownProse';

interface TheaterDialogueProps {
  segment?: TurnSegment;
  fallback: string;
  speaker?: string;
  isPlayer?: boolean;
  onEntityClick?: (entityId: string) => void;
  displayMode?: 'stage_directions' | 'hidden' | 'raw';
  onAdvance: () => void;
}

// TheaterDialogue shows one beat at a time: a speaker name plate, an optional
// portrait, and the line. Clicking anywhere on the panel advances.
export const TheaterDialogue: React.FC<TheaterDialogueProps> = ({
  segment,
  fallback,
  speaker,
  isPlayer = false,
  onEntityClick,
  displayMode,
  onAdvance,
}) => {
  const text = segment?.text ?? fallback;
  const isSpeech = segment?.kind === 'speech';
  const name = isSpeech ? segment?.speaker || speaker || 'Unknown' : 'Narrator';

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onAdvance}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onAdvance();
        }
      }}
      className="relative w-full max-w-4xl mx-auto text-left pointer-events-auto cursor-pointer"
      title="Click to continue"
      aria-label={isSpeech ? `${name}: ${text}` : text}
    >
      <div className="flex items-end gap-3">
        {isSpeech && segment?.portrait_url && (
          <div
            className={`w-16 h-16 rounded-lg overflow-hidden shrink-0 border-2 shadow-2xl bg-black/40 ${
              isPlayer ? 'border-sky-400/80' : 'border-purple-400/80'
            }`}
          >
            <img src={segment.portrait_url} alt="" aria-hidden="true" className="w-full h-full object-cover" />
          </div>
        )}
        <div
          className={`relative w-full bg-stone-950/90 backdrop-blur-xl rounded-2xl border shadow-2xl px-6 py-5 min-h-[20vh] ${
            isPlayer ? 'border-sky-400/50' : isSpeech ? 'border-purple-400/50' : 'border-white/15'
          }`}
        >
          <span
            className={`absolute -top-3.5 left-6 px-3 py-1 rounded-md text-sm font-sans font-extrabold tracking-wide shadow-lg ${
              isPlayer ? 'bg-sky-600 text-white' : isSpeech ? 'bg-purple-600 text-white' : 'bg-stone-700 text-stone-100'
            }`}
          >
            {name}
          </span>
          <MarkdownProse
            text={isSpeech ? `\u201c${text}\u201d` : text}
            onEntityClick={onEntityClick}
            displayMode={displayMode}
            className={
              isSpeech
                ? 'text-stone-50 text-xl leading-relaxed italic space-y-2'
                : 'text-stone-200 text-xl leading-relaxed tracking-wide font-serif space-y-3'
            }
          />
          <span className="absolute bottom-2 right-3 text-purple-300/70 animate-pulse text-xs" aria-hidden="true">
            &#9662;
          </span>
        </div>
      </div>
    </div>
  );
};
```

- [x] **Step 2: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 3: `TheaterTransport` component

**Files:**
- Create: `frontend/src/components/theater/TheaterTransport.tsx`

- [x] **Step 1: Create the component**

```tsx
import React from 'react';
import { Loader2, Pause, Play, RotateCw, SkipBack, SkipForward, Square, Volume2 } from 'lucide-react';
import { TurnAudioState } from '../TurnSegments';

interface TheaterTransportProps {
  progress: number;
  isPlaying: boolean;
  speed: number;
  serverPlayback: boolean;
  audioState: TurnAudioState;
  audioMessage?: string;
  blocked?: boolean;
  onToggle: () => void;
  onPrev: () => void;
  onNext: () => void;
  onCycleSpeed: () => void;
  onRegenerate: () => void;
  onStop: () => void;
  onUnblock: () => void;
}

export const TheaterTransport: React.FC<TheaterTransportProps> = ({
  progress,
  isPlaying,
  speed,
  serverPlayback,
  audioState,
  audioMessage,
  blocked = false,
  onToggle,
  onPrev,
  onNext,
  onCycleSpeed,
  onRegenerate,
  onStop,
  onUnblock,
}) => {
  const generating = audioState === 'generating';
  const playing = audioState === 'playing';

  return (
    <div className="w-full max-w-4xl mx-auto flex flex-col items-center gap-2 pointer-events-auto">
      <div className="w-full h-1.5 bg-stone-900 rounded-full overflow-hidden border border-white/5">
        <div className="h-full bg-purple-500 transition-all duration-300" style={{ width: `${Math.min(100, progress)}%` }} />
      </div>

      <div className="flex flex-wrap items-center justify-center gap-3">
        <button onClick={onPrev} className="p-2 rounded-full hover:bg-white/10 text-stone-300 cursor-pointer" title="Previous turn">
          <SkipBack className="w-5 h-5" />
        </button>
        <button
          onClick={onToggle}
          className="p-3.5 rounded-full bg-purple-600 hover:bg-purple-500 text-white font-bold shadow-lg transition-transform hover:scale-105 cursor-pointer"
          title={isPlaying ? 'Pause' : 'Play'}
        >
          {isPlaying ? <Pause className="w-6 h-6" /> : <Play className="w-6 h-6 ml-0.5" />}
        </button>
        <button onClick={onNext} className="p-2 rounded-full hover:bg-white/10 text-stone-300 cursor-pointer" title="Next turn">
          <SkipForward className="w-5 h-5" />
        </button>
        <button
          onClick={onCycleSpeed}
          className="px-3 py-1 rounded-lg bg-stone-900/80 border border-white/10 text-xs font-mono font-bold text-purple-300 hover:bg-stone-800 cursor-pointer"
          title="Playback speed"
        >
          {speed}x
        </button>

        {serverPlayback && (
          <>
            <button
              onClick={onStop}
              disabled={!playing}
              className={`inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border text-xs transition-colors ${
                playing
                  ? 'text-rose-300 border-rose-500/50 hover:bg-rose-500/20 cursor-pointer bg-stone-900/80'
                  : 'opacity-40 cursor-not-allowed bg-stone-900/80 border-stone-700 text-stone-400'
              }`}
              title="Stop narration"
            >
              <Square className="w-3 h-3" />
              <span>Stop</span>
            </button>
            <button
              onClick={onRegenerate}
              disabled={generating || playing}
              className={`inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border text-xs transition-colors ${
                generating || playing
                  ? 'opacity-40 cursor-not-allowed bg-stone-900/80 border-stone-700 text-stone-400'
                  : 'bg-stone-900/80 border-stone-700 text-stone-300 hover:text-amber-300 hover:border-amber-500/40 cursor-pointer'
              }`}
              title="Regenerate this turn's speech"
            >
              <RotateCw className="w-3 h-3" />
              <span>Regenerate</span>
            </button>
            {generating && (
              <span className="inline-flex items-center gap-1 px-2 py-1 rounded-full bg-purple-900/50 border border-purple-500/40 text-purple-300 text-xs">
                <Loader2 className="w-3 h-3 animate-spin" />
                <span>Rendering speech…</span>
              </span>
            )}
          </>
        )}

        {!serverPlayback && blocked && (
          <button
            onClick={onUnblock}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-purple-600 hover:bg-purple-500 text-white text-xs font-sans font-bold cursor-pointer"
            title="The browser needs a click before it will play audio"
          >
            <Volume2 className="w-3.5 h-3.5" />
            <span>Enable audio</span>
          </button>
        )}

        {audioState === 'error' && audioMessage && (
          <span className="inline-flex items-center gap-1 px-2 py-1 rounded-full bg-rose-900/40 border border-rose-500/40 text-rose-300 text-xs" title={audioMessage}>
            Error: {audioMessage}
          </span>
        )}
      </div>
    </div>
  );
};
```

- [x] **Step 2: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 4: Rewrite `StoryTheater` with native audio and one-line VN staging

**Files:**
- Rewrite: `frontend/src/components/StoryTheater.tsx`

**Interfaces:**
- Consumes: `TheaterStage`, `TheaterDialogue`, `TheaterTransport`, `useSegmentPlayback`, `TurnAudioState`.
- Props gain: `serverPlayback`, `onPlayTurnAudio`, `onStopAudio`, `turnAudioStatus`, `onEntityClick`, `displayMode`. `autoPlay` is removed (the theater always starts playing).

- [x] **Step 1: Replace the file**

```tsx
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Turn } from '../types';
import { TurnAudioState } from './TurnSegments';
import { useSegmentPlayback } from '../hooks/useSegmentPlayback';
import { TheaterStage } from './theater/TheaterStage';
import { TheaterDialogue } from './theater/TheaterDialogue';
import { TheaterTransport } from './theater/TheaterTransport';
import { X } from 'lucide-react';

interface StoryTheaterProps {
  turns: Turn[];
  isOpen: boolean;
  onClose: () => void;
  volume?: number;
  gameId?: string;
  playerId?: string;
  playerPortrait?: string;
  serverPlayback?: boolean;
  onPlayTurnAudio?: (turnNumber: number, force?: boolean) => void;
  onStopAudio?: () => void;
  turnAudioStatus?: Record<number, { state: TurnAudioState; message?: string }>;
  onEntityClick?: (entityId: string) => void;
  displayMode?: 'stage_directions' | 'hidden' | 'raw';
}

export const StoryTheater: React.FC<StoryTheaterProps> = ({
  turns,
  isOpen,
  onClose,
  volume = 1,
  gameId,
  playerId,
  playerPortrait: propPlayerPortrait,
  serverPlayback = false,
  onPlayTurnAudio,
  onStopAudio,
  turnAudioStatus = {},
  onEntityClick,
  displayMode,
}) => {
  const [currentIdx, setCurrentIdx] = useState(0);
  const [activeSegment, setActiveSegment] = useState(0);
  const [isPlaying, setIsPlaying] = useState(true);
  const [speed, setSpeed] = useState<number>(1);

  const currentTurn = turns[currentIdx];
  const segments = useMemo(() => {
    if (currentTurn?.segments && currentTurn.segments.length > 0) return currentTurn.segments;
    return [{ kind: 'narration' as const, text: currentTurn?.prose ?? '' }];
  }, [currentTurn]);

  const activeIndex = Math.min(activeSegment, segments.length - 1);
  const active = segments[activeIndex];

  const playerPortrait = useMemo(() => {
    if (propPlayerPortrait) return propPlayerPortrait;
    for (const turn of turns) {
      const pSeg = turn.segments?.find((s) => s.player && s.portrait_url);
      if (pSeg?.portrait_url) return pSeg.portrait_url;
    }
    if (gameId && playerId) {
      return `/api/game/${gameId}/character/${playerId}/portrait`;
    }
    return undefined;
  }, [propPlayerPortrait, turns, gameId, playerId]);

  const npcPortrait = useMemo(
    () => segments.find((s) => s.kind === 'speech' && !s.player && s.portrait_url)?.portrait_url,
    [segments]
  );

  const playerSpeaking = active?.kind === 'speech' && !!active.player;
  const npcSpeaking = active?.kind === 'speech' && !active.player;

  const hasAudio = segments.some((segment) => !!segment.audio_url);
  const browser = useSegmentPlayback(segments, isPlaying && !serverPlayback && hasAudio, volume);

  const audioStatus = currentTurn ? turnAudioStatus[currentTurn.turn_number] : undefined;
  const audioState: TurnAudioState = audioStatus?.state ?? 'idle';

  // Reset the beat whenever the turn changes.
  useEffect(() => {
    setActiveSegment(0);
  }, [currentIdx]);

  const goNext = useCallback(() => {
    if (currentIdx < turns.length - 1) {
      setCurrentIdx((prev) => prev + 1);
    } else {
      setIsPlaying(false);
    }
  }, [currentIdx, turns.length]);

  // Native playback: start the turn's audio once per turn while playing.
  const startedTurnRef = useRef<number | null>(null);
  useEffect(() => {
    if (!isOpen || !isPlaying || !serverPlayback || !currentTurn) return;
    if (startedTurnRef.current === currentTurn.turn_number) return;
    startedTurnRef.current = currentTurn.turn_number;
    onPlayTurnAudio?.(currentTurn.turn_number);
  }, [isOpen, isPlaying, serverPlayback, currentTurn, onPlayTurnAudio]);

  // Advance when the device reports the turn's audio has finished.
  const previousAudioState = useRef<TurnAudioState>('idle');
  useEffect(() => {
    if (!serverPlayback) return;
    const previous = previousAudioState.current;
    previousAudioState.current = audioState;
    if (previous === 'playing' && audioState === 'idle' && isPlaying) {
      goNext();
    }
  }, [audioState, serverPlayback, isPlaying, goNext]);

  // Per-beat pacing. With native audio the last beat is held until the audio ends.
  useEffect(() => {
    if (!isOpen || !isPlaying || turns.length === 0) return;
    const dwell = Math.max(1200, (active?.duration ?? 0) * 1000) / speed;
    const timer = setTimeout(() => {
      if (activeIndex < segments.length - 1) {
        setActiveSegment(activeIndex + 1);
      } else if (!serverPlayback) {
        goNext();
      }
    }, dwell);
    return () => clearTimeout(timer);
  }, [isOpen, isPlaying, activeIndex, segments.length, active?.duration, speed, serverPlayback, goNext, turns.length]);

  const togglePlay = useCallback(() => {
    setIsPlaying((playing) => {
      const next = !playing;
      if (!next) {
        onStopAudio?.();
        startedTurnRef.current = null;
      }
      return next;
    });
  }, [onStopAudio]);

  const advanceDialogue = () => {
    if (activeIndex < segments.length - 1) {
      setActiveSegment(activeIndex + 1);
    } else {
      goNext();
    }
  };

  const changeTurn = (delta: number) => {
    onStopAudio?.();
    startedTurnRef.current = null;
    setCurrentIdx((prev) => Math.min(Math.max(0, prev + delta), turns.length - 1));
  };

  const regenerate = () => {
    if (!currentTurn) return;
    onPlayTurnAudio?.(currentTurn.turn_number, true);
    startedTurnRef.current = currentTurn.turn_number;
  };

  if (!isOpen || turns.length === 0) return null;

  const progress = ((currentIdx + (activeIndex + 1) / segments.length) / turns.length) * 100;

  return (
    <div
      className="fixed inset-0 z-50 overflow-hidden select-none bg-stone-950 text-stone-100"
      role="dialog"
      aria-label="Story theater"
    >
      <TheaterStage
        backgroundURL={currentTurn?.image_url}
        playerPortrait={playerPortrait}
        npcPortrait={npcPortrait}
        playerActive={playerSpeaking}
        npcActive={npcSpeaking}
      />

      <header className="absolute top-0 inset-x-0 z-20 px-6 py-4 flex items-center justify-between bg-gradient-to-b from-black/80 to-transparent">
        <div className="flex flex-wrap items-center gap-3">
          <span className="font-sans text-purple-300 font-bold tracking-[0.2em] text-sm">STORY THEATER</span>
          {currentTurn?.location_name && (
            <span className="text-xs font-sans text-stone-300 bg-black/50 px-2.5 py-1 rounded-full border border-white/10">
              {currentTurn.location_name}
            </span>
          )}
          <span className="text-xs font-mono text-stone-400 bg-black/50 px-2 py-1 rounded border border-white/10">
            Turn {currentIdx + 1} of {turns.length}
          </span>
        </div>
        <button
          onClick={onClose}
          className="p-2 rounded-full hover:bg-white/10 text-stone-300 hover:text-white transition-colors cursor-pointer"
          aria-label="Close theater"
        >
          <X className="w-6 h-6" />
        </button>
      </header>

      <div className="absolute inset-x-0 bottom-0 z-20 flex flex-col items-center gap-3 pb-5 px-4">
        <TheaterDialogue
          segment={active}
          fallback={currentTurn?.prose ?? ''}
          speaker={active?.speaker}
          isPlayer={playerSpeaking}
          onEntityClick={onEntityClick}
          displayMode={displayMode}
          onAdvance={advanceDialogue}
        />
        <TheaterTransport
          progress={progress}
          isPlaying={isPlaying}
          speed={speed}
          serverPlayback={serverPlayback}
          audioState={audioState}
          audioMessage={audioStatus?.message}
          blocked={browser.blocked && !browser.playing}
          onToggle={togglePlay}
          onPrev={() => changeTurn(-1)}
          onNext={() => changeTurn(1)}
          onCycleSpeed={() => setSpeed((s) => (s === 1 ? 1.5 : s === 1.5 ? 2 : 1))}
          onRegenerate={regenerate}
          onStop={() => onStopAudio?.()}
          onUnblock={browser.play}
        />
      </div>
    </div>
  );
};
```

- [x] **Step 2: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 5: Wire App

**Files:**
- Modify: `frontend/src/App.tsx`

- [x] **Step 1: Pass the theater props**

Replace the `StoryTheater` render:

```tsx
          {/* Full-Screen Visual Novel Story Theater */}
          <StoryTheater
            turns={chronicle}
            isOpen={isTheaterOpen}
            onClose={() => setIsTheaterOpen(false)}
            volume={config?.media.tts.master_volume ?? 1}
            gameId={activeGameID ?? undefined}
            playerId={gameState?.player?.id}
            serverPlayback={serverAudio}
            onPlayTurnAudio={(turnNumber, force) => handlePlayTurnAudio(turnNumber, undefined, force)}
            onStopAudio={handleStopAudio}
            turnAudioStatus={turnAudioStatus}
            onEntityClick={handleOpenWikilink}
            displayMode={config?.media.tts.speech_cues?.display_mode}
          />
```

- [x] **Step 2: Typecheck and build**

Run: `mise run test:frontend && mise run build:frontend`
Expected: exit 0.

---

### Task 6: Verification

- [x] **Step 1: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

- [x] **Step 2: Full backend suite (unchanged, sanity)**

Run: `mise run test:backend`
Expected: PASS.

- [x] **Step 3: Manual**

With a device: theater narration plays through the app, not the browser; Pause stops audio; Prev/Next stop audio before moving; the turn advances when the audio ends. Without a device: the "Enable audio" button appears when the browser blocks autoplay. Layout: full-bleed scene, left/right sprites that dim/brighten with the active speaker, a name plate, one line at a time, click-to-advance.

---

## Self-Review

**Spec coverage:** native audio (Task 4 + 5), fallback (Task 4), layout split into stage/dialogue/transport (Tasks 1-3), one-line advancement and sprite swap (Task 4), info bar location/turn (Task 4), transport controls (Task 3).

**Placeholder scan:** none.

**Type consistency:** `TurnAudioState` and `useSegmentPlayback` come from existing modules; `TheaterTransport` prop names match Task 4's render; `StoryTheater` props match Task 5.
