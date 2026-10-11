import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Turn, TurnSegment, LimitState, PlaybackEntry } from '../types';
import { TurnAudioState, segmentAudioKey } from './TurnSegments';
import { useSegmentPlayback } from '../hooks/useSegmentPlayback';
import { anySegmentHasAudio, groupLastIndex, groupLeaderIndex } from '../lib/audio';
import { readingDurationMs } from '../lib/pacing';
import { TheaterStage } from './theater/TheaterStage';
import { TheaterDialogue } from './theater/TheaterDialogue';
import { TheaterTransport } from './theater/TheaterTransport';
import { X } from 'lucide-react';
import { useMountTransition } from '../hooks/useMountTransition';
import { LimitChip } from './LimitChip';

interface StoryTheaterProps {
  turns: Turn[];
  isOpen: boolean;
  onClose: () => void;
  volume?: number;
  gameId?: string;
  playerId?: string;
  playerName?: string;
  playerPortrait?: string;
  campaignImage?: string;
  serverPlayback?: boolean;
  onPlayAudio?: (turnNumber: number, segmentIndex?: number, force?: boolean) => void;
  onStopAudio?: () => void;
  segmentAudioStatus?: Record<string, { state: TurnAudioState; message?: string }>;
  onEntityClick?: (entityId: string) => void;
  displayMode?: 'stage_directions' | 'hidden' | 'raw';
  limits?: LimitState[];
  // Clips already heard while the turn streamed, which playback must skip.
  skipAudioKeys?: ReadonlySet<string>;
  // Playback ledger containing resume offsets and completion states.
  playbackLedger?: Record<string, PlaybackEntry>;
  // weather is the current location's weather, which draws the stage's overlay.
  weather?: string;
}

// BEAT_GAP_MS is the buffer between one voice clip finishing and the next line
// appearing, so the spoken word always leads the text.
const BEAT_GAP_MS = 120;

// beatGapMs scales the inter-beat gap by the speed control, so faster playback
// tightens it and slower playback widens it.
export function beatGapMs(speed: number): number {
  const factor = speed > 0 ? speed : 1;
  return Math.max(40, BEAT_GAP_MS / factor);
}

// segmentsOf returns a turn's beats, falling back to its prose as one beat.
function segmentsOf(turn: Turn | undefined): TurnSegment[] {
  if (turn?.segments && turn.segments.length > 0) return turn.segments;
  return [{ kind: 'narration', text: turn?.prose ?? '' }];
}

export const StoryTheater: React.FC<StoryTheaterProps> = ({
  turns,
  isOpen,
  onClose,
  volume = 1,
  gameId,
  playerId,
  playerName: propPlayerName,
  playerPortrait: propPlayerPortrait,
  campaignImage,
  serverPlayback = false,
  onPlayAudio,
  onStopAudio,
  segmentAudioStatus = {},
  onEntityClick,
  displayMode,
  limits,
  skipAudioKeys,
  playbackLedger,
  weather,
}) => {
  const [currentIdx, setCurrentIdx] = useState(0);
  const [activeSegment, setActiveSegment] = useState(0);
  const [isPlaying, setIsPlaying] = useState(true);
  const [speed, setSpeed] = useState<number>(1);
  const [beatProgress, setBeatProgress] = useState(0);
  // Captions are off by default so they do not duplicate the dialogue for a
  // hearing viewer.
  const [captions, setCaptions] = useState(false);

  const currentTurn = turns[currentIdx];
  const segments = useMemo(() => segmentsOf(currentTurn), [currentTurn]);

  const activeIndex = Math.min(activeSegment, segments.length - 1);
  const active = segments[activeIndex];
  // The roll this beat narrates, when it has one. A check is an annotation on its
  // beat, not a beat of its own: playback and audio are indexed by segment.
  const activeCheck = useMemo(
    () =>
      active?.check_ref
        ? (currentTurn?.checks ?? []).find((check) => check.check_id === active.check_ref)
        : undefined,
    [active, currentTurn],
  );
  // A clip group shares one audio clip, so playback is keyed on the group's
  // leader: stepping within a group keeps the audio playing, and only entering a
  // new group switches it.
  const groupLeader = groupLeaderIndex(segments, activeIndex);
  const groupLast = groupLastIndex(segments, activeIndex);

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

  const npcSegment = useMemo(
    () => segments.find((s) => s.kind === 'speech' && !s.player && s.portrait_url),
    [segments]
  );
  const npcPortrait = npcSegment?.portrait_url;

  const playerLabel = useMemo(() => {
    if (propPlayerName && propPlayerName.trim()) return propPlayerName;
    for (const turn of turns) {
      const pSeg = turn.segments?.find((s) => s.player && s.kind === 'speech' && s.speaker);
      if (pSeg?.speaker) return pSeg.speaker;
    }
    return 'You';
  }, [propPlayerName, turns]);
  const npcLabel = npcSegment?.speaker || 'Unknown';

  const playerSpeaking = active?.kind === 'speech' && !!active.player;
  const npcSpeaking = active?.kind === 'speech' && !active.player;

  // Two copies of one image must never share the stage: the right sprite is a
  // mirror of the left, so when both resolve to the same source only the left
  // is drawn and it carries the active-speaker emphasis.
  const normalizeImage = (url?: string) => (url ? url.split('?')[0] : '');
  const sharedPortrait =
    !!playerPortrait && !!npcPortrait && normalizeImage(playerPortrait) === normalizeImage(npcPortrait);
  const showPlayerPortrait = !!playerPortrait;
  const showNpcPortrait = !!npcPortrait && !sharedPortrait;
  const playerActive = playerSpeaking || (npcSpeaking && !showNpcPortrait);
  const npcActive = npcSpeaking && showNpcPortrait;

  // The scene's own art wins; otherwise the campaign banner fills the stage.
  const backgroundURL = currentTurn?.image_url || campaignImage;

  const hasAudio = anySegmentHasAudio(segments);
  const voiceEnabled = hasAudio;

  const reducedMotion = useMemo(
    () =>
      typeof window !== 'undefined' &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches,
    []
  );

  // The stage's Ken Burns drifts over the beat, so progress is sampled while the
  // beat plays. Reduced motion, a paused theatre, and a beat with no image all
  // leave the stage still.
  useEffect(() => {
    if (!isOpen || !isPlaying || reducedMotion || !backgroundURL) {
      setBeatProgress(0);
      return;
    }
    const baseMs =
      active?.duration && active.duration > 0 ? active.duration * 1000 : readingDurationMs(active?.text ?? '');
    const spanMs = Math.max(400, baseMs / (speed > 0 ? speed : 1));
    const started = performance.now();
    let frame = 0;
    const tick = () => {
      const value = Math.min(1, (performance.now() - started) / spanMs);
      setBeatProgress(value);
      if (value < 1) frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [isOpen, isPlaying, reducedMotion, backgroundURL, active, speed]);

  const browser = useSegmentPlayback(segments, {
    autoPlay: isPlaying && !serverPlayback && voiceEnabled,
    volume,
    gameId,
    turnNumber: currentTurn?.turn_number,
    skipKeys: skipAudioKeys,
    ledger: playbackLedger,
  });

  const beatKey = currentTurn ? segmentAudioKey(currentTurn.turn_number, groupLeader) : '';
  const beatStatus = serverPlayback ? segmentAudioStatus[beatKey] : undefined;
  const beatState: TurnAudioState = beatStatus?.state ?? 'idle';
  // A group the policy left without a clip is visibly silent rather than looking
  // like a stall, unless its clip is still being synthesized.
  const beatNoAudio =
    voiceEnabled && (segments[groupLeader]?.audio_urls?.length ?? 0) === 0 && beatState !== 'generating';

  const goNext = useCallback(() => {
    if (currentIdx < turns.length - 1) {
      setCurrentIdx((prev) => prev + 1);
    } else {
      setIsPlaying(false);
    }
  }, [currentIdx, turns.length]);

  // stepSegment walks the beats one at a time, crossing a turn boundary at either
  // end. It is what the transport and the advance caret do, and it never touches
  // playback: the audio follows the clip group, not the beat.
  const stepSegment = useCallback(
    (delta: number) => {
      const next = activeIndex + delta;
      if (next >= 0 && next < segments.length) {
        setActiveSegment(next);
        return;
      }
      const nextTurn = currentIdx + (delta > 0 ? 1 : -1);
      if (nextTurn < 0 || nextTurn >= turns.length) return;
      const nextSegments = segmentsOf(turns[nextTurn]);
      setCurrentIdx(nextTurn);
      setActiveSegment(delta > 0 ? 0 : nextSegments.length - 1);
    },
    [activeIndex, segments.length, currentIdx, turns],
  );

  // advanceGroup moves to the next clip group once the current one has played, so
  // a merged run is followed by the next group rather than by a silent beat.
  const advanceGroup = useCallback(() => {
    const next = groupLast + 1;
    if (next < segments.length) {
      setActiveSegment(next);
    } else {
      goNext();
    }
  }, [groupLast, segments.length, goNext]);

  // Reset the beat whenever the turn changes.
  useEffect(() => {
    setActiveSegment(0);
  }, [currentIdx]);

  // Native voice: request the current group's clip once, then wait for the device
  // to report it finished before the next group appears. Stepping within the group
  // leaves the audio untouched.
  const startedBeatRef = useRef<string | null>(null);
  useEffect(() => {
    if (!isOpen || !isPlaying || !serverPlayback || !voiceEnabled || !currentTurn) return;
    if (startedBeatRef.current === beatKey) return;
    startedBeatRef.current = beatKey;
    onPlayAudio?.(currentTurn.turn_number, groupLeader);
  }, [isOpen, isPlaying, serverPlayback, voiceEnabled, currentTurn, groupLeader, beatKey, onPlayAudio]);

  // Prefetch the next clip group's audio and preload its image, so the next
  // group does not wait on the network. Bounded to a single group.
  const prefetchedRef = useRef<string | null>(null);
  useEffect(() => {
    if (!isOpen || !isPlaying) return;
    const nextWithinTurn = groupLast + 1 < segments.length;
    const nextSegment = nextWithinTurn ? segments[groupLast + 1] : undefined;
    const nextTurn = nextWithinTurn ? currentTurn : turns[currentIdx + 1];
    if (!nextTurn) return;
    const nextIndex = nextWithinTurn ? groupLast + 1 : 0;
    const key = `${nextTurn.turn_number}:${nextIndex}`;
    if (prefetchedRef.current === key) return;
    prefetchedRef.current = key;
    for (const url of nextSegment?.audio_urls ?? nextTurn.segments?.[0]?.audio_urls ?? []) {
      fetch(url).catch(() => {
        // Prefetch is best effort; playback still works without it.
      });
    }
    const nextImage = nextTurn.image_url;
    if (nextImage && nextImage !== backgroundURL) {
      const image = new Image();
      image.src = nextImage;
    }
  }, [isOpen, isPlaying, groupLast, currentIdx, segments, currentTurn, turns, backgroundURL]);

  const previousBeatState = useRef<TurnAudioState>('idle');
  useEffect(() => {
    if (!serverPlayback || !voiceEnabled) return;
    const previous = previousBeatState.current;
    previousBeatState.current = beatState;
    if (previous !== 'playing' || (beatState !== 'idle' && beatState !== 'error') || !isPlaying) return;
    const timer = setTimeout(advanceGroup, beatGapMs(speed));
    return () => clearTimeout(timer);
  }, [beatState, serverPlayback, voiceEnabled, isPlaying, advanceGroup]);

  // While a group's audio plays, walk the text through the group's beats at a
  // reading pace, so a merged run is read line by line rather than jumped over.
  // Stepping within a group never re-requests the audio.
  useEffect(() => {
    if (!isOpen || !isPlaying || !voiceEnabled) return;
    const playing = serverPlayback ? beatState === 'playing' : browser.playing;
    if (!playing || activeIndex >= groupLast) return;
    const dwell = readingDurationMs(active?.text ?? '') / (speed > 0 ? speed : 1);
    const timer = setTimeout(() => setActiveSegment(activeIndex + 1), dwell);
    return () => clearTimeout(timer);
  }, [isOpen, isPlaying, voiceEnabled, serverPlayback, beatState, browser.playing, activeIndex, groupLast, active?.text, speed]);

  // Browser voice: mirror the clip the browser is actually playing, jumping only
  // when the group changes so the text does not restart mid-group.
  useEffect(() => {
    if (serverPlayback || !voiceEnabled) return;
    if (browser.playingIndex === null) return;
    const timer = setTimeout(() => {
      setActiveSegment((prev) =>
        groupLeaderIndex(segments, prev) === browser.playingIndex ? prev : (browser.playingIndex as number),
      );
    }, beatGapMs(speed));
    return () => clearTimeout(timer);
  }, [browser.playingIndex, serverPlayback, voiceEnabled, segments]);

  // Browser voice: the turn is finished once its clips stop playing.
  const browserWasPlaying = useRef(false);
  useEffect(() => {
    if (serverPlayback || !voiceEnabled) return;
    const was = browserWasPlaying.current;
    browserWasPlaying.current = browser.playing;
    if (was && !browser.playing && isPlaying) {
      const timer = setTimeout(advanceGroup, beatGapMs(speed));
      return () => clearTimeout(timer);
    }
  }, [browser.playing, serverPlayback, voiceEnabled, isPlaying, advanceGroup]);

  // Without voice, the text paces itself on the reading estimate, scaled by the
  // speed control, so a silent beat lingers as long as it takes to read.
  useEffect(() => {
    if (!isOpen || !isPlaying || turns.length === 0 || voiceEnabled) return;
    const dwell = readingDurationMs(active?.text ?? '') / (speed > 0 ? speed : 1);
    const timer = setTimeout(() => stepSegment(1), dwell);
    return () => clearTimeout(timer);
  }, [isOpen, isPlaying, turns.length, voiceEnabled, active?.text, speed, stepSegment]);

  const togglePlay = useCallback(() => {
    const next = !isPlaying;
    setIsPlaying(next);
    if (!next) {
      onStopAudio?.();
      startedBeatRef.current = null;
      previousBeatState.current = 'idle';
      browserWasPlaying.current = false;
    }
  }, [isPlaying, onStopAudio]);

  const advanceDialogue = () => {
    stepSegment(1);
  };

  const { mounted, state } = useMountTransition(isOpen && turns.length > 0, 250);

  if (!mounted) return null;
  const progress = ((currentIdx + (activeIndex + 1) / segments.length) / turns.length) * 100;

  return (
    <div
      data-state={state}
      className={`fixed inset-0 z-50 overflow-hidden bg-stone-950 text-stone-100 ${
        state === 'enter' ? 'anim-fade-in' : 'anim-fade-out pointer-events-none'
      }`}
      style={{ '--anim-dur': '250ms' } as React.CSSProperties}
      role="dialog"
      aria-label="Story theater"
    >
      <TheaterStage
        backgroundURL={backgroundURL}
        playerPortrait={showPlayerPortrait ? playerPortrait : undefined}
        npcPortrait={showNpcPortrait ? npcPortrait : undefined}
        playerLabel={playerLabel}
        npcLabel={npcLabel}
        playerActive={playerActive}
        npcActive={npcActive}
        progress={beatProgress}
        seed={currentTurn?.turn_number ?? 0}
        outcome={currentTurn?.outcome}
        weather={weather}
        reducedMotion={reducedMotion}
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
          {limits?.map((block, idx) => (
            <LimitChip key={idx} block={block} />
          ))}
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
          noAudio={beatNoAudio}
          caption={captions}
          check={activeCheck}
          onAdvance={advanceDialogue}
        />
        <TheaterTransport
          progress={progress}
          isPlaying={isPlaying}
          speed={speed}
          labels={{ prev: 'Previous line', next: 'Next line' }}
          audioState={beatState}
          audioMessage={beatStatus?.message}
          blocked={browser.blocked && !browser.playing}
          captions={captions}
          onToggleCaptions={() => setCaptions((value) => !value)}
          onToggle={togglePlay}
          onPrev={() => stepSegment(-1)}
          onNext={() => stepSegment(1)}
          onCycleSpeed={() => setSpeed((s) => (s === 1 ? 1.5 : s === 1.5 ? 2 : 1))}
          onUnblock={browser.play}
        />
      </div>
    </div>
  );
};
