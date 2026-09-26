import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Turn } from '../types';
import { TurnAudioState, segmentAudioKey } from './TurnSegments';
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
  campaignImage?: string;
  serverPlayback?: boolean;
  onPlayAudio?: (turnNumber: number, segmentIndex?: number, force?: boolean) => void;
  onStopAudio?: () => void;
  segmentAudioStatus?: Record<string, { state: TurnAudioState; message?: string }>;
  onEntityClick?: (entityId: string) => void;
  displayMode?: 'stage_directions' | 'hidden' | 'raw';
}

// BEAT_GAP_MS is the buffer between one voice clip finishing and the next line
// appearing, so the spoken word always leads the text.
const BEAT_GAP_MS = 300;

export const StoryTheater: React.FC<StoryTheaterProps> = ({
  turns,
  isOpen,
  onClose,
  volume = 1,
  gameId,
  playerId,
  playerPortrait: propPlayerPortrait,
  campaignImage,
  serverPlayback = false,
  onPlayAudio,
  onStopAudio,
  segmentAudioStatus = {},
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

  const npcSegment = useMemo(
    () => segments.find((s) => s.kind === 'speech' && !s.player && s.portrait_url),
    [segments]
  );
  const npcPortrait = npcSegment?.portrait_url;

  const playerLabel = useMemo(() => {
    for (const turn of turns) {
      const pSeg = turn.segments?.find((s) => s.player && s.kind === 'speech' && s.speaker);
      if (pSeg?.speaker) return pSeg.speaker;
    }
    return 'You';
  }, [turns]);
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

  const hasAudio = segments.some((segment) => !!segment.audio_url);
  const voiceEnabled = hasAudio;

  const browser = useSegmentPlayback(segments, isPlaying && !serverPlayback && voiceEnabled, volume);

  const beatKey = currentTurn ? segmentAudioKey(currentTurn.turn_number, activeIndex) : '';
  const beatStatus = serverPlayback ? segmentAudioStatus[beatKey] : undefined;
  const beatState: TurnAudioState = beatStatus?.state ?? 'idle';

  const goNext = useCallback(() => {
    if (currentIdx < turns.length - 1) {
      setCurrentIdx((prev) => prev + 1);
    } else {
      setIsPlaying(false);
    }
  }, [currentIdx, turns.length]);

  const advanceBeat = useCallback(() => {
    if (activeIndex < segments.length - 1) {
      setActiveSegment(activeIndex + 1);
    } else {
      goNext();
    }
  }, [activeIndex, segments.length, goNext]);

  // Reset the beat whenever the turn changes.
  useEffect(() => {
    setActiveSegment(0);
  }, [currentIdx]);

  // Native voice: request the current beat's clip once, then wait for the device
  // to report it finished before the next line appears.
  const startedBeatRef = useRef<string | null>(null);
  useEffect(() => {
    if (!isOpen || !isPlaying || !serverPlayback || !voiceEnabled || !currentTurn) return;
    if (startedBeatRef.current === beatKey) return;
    startedBeatRef.current = beatKey;
    onPlayAudio?.(currentTurn.turn_number, activeIndex);
  }, [isOpen, isPlaying, serverPlayback, voiceEnabled, currentTurn, activeIndex, beatKey, onPlayAudio]);

  const previousBeatState = useRef<TurnAudioState>('idle');
  useEffect(() => {
    if (!serverPlayback || !voiceEnabled) return;
    const previous = previousBeatState.current;
    previousBeatState.current = beatState;
    if (previous !== 'playing' || (beatState !== 'idle' && beatState !== 'error') || !isPlaying) return;
    const timer = setTimeout(advanceBeat, BEAT_GAP_MS);
    return () => clearTimeout(timer);
  }, [beatState, serverPlayback, voiceEnabled, isPlaying, advanceBeat]);

  // Browser voice: mirror the clip the browser is actually playing, with the
  // same buffer so the text trails the voice.
  useEffect(() => {
    if (serverPlayback || !voiceEnabled) return;
    if (browser.playingIndex === null) return;
    const timer = setTimeout(() => setActiveSegment(browser.playingIndex as number), BEAT_GAP_MS);
    return () => clearTimeout(timer);
  }, [browser.playingIndex, serverPlayback, voiceEnabled]);

  // Browser voice: the turn is finished once its clips stop playing.
  const browserWasPlaying = useRef(false);
  useEffect(() => {
    if (serverPlayback || !voiceEnabled) return;
    const was = browserWasPlaying.current;
    browserWasPlaying.current = browser.playing;
    if (was && !browser.playing && isPlaying) {
      const timer = setTimeout(goNext, BEAT_GAP_MS);
      return () => clearTimeout(timer);
    }
  }, [browser.playing, serverPlayback, voiceEnabled, isPlaying, goNext]);

  // Without voice, the text paces itself on the recorded reading time.
  useEffect(() => {
    if (!isOpen || !isPlaying || turns.length === 0 || voiceEnabled) return;
    const dwell = Math.max(1200, (active?.duration ?? 0) * 1000) / speed;
    const timer = setTimeout(advanceBeat, dwell);
    return () => clearTimeout(timer);
  }, [isOpen, isPlaying, turns.length, voiceEnabled, active?.duration, speed, advanceBeat]);

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
    if (activeIndex < segments.length - 1) {
      setActiveSegment(activeIndex + 1);
    } else {
      goNext();
    }
  };

  const changeTurn = (delta: number) => {
    onStopAudio?.();
    startedBeatRef.current = null;
    previousBeatState.current = 'idle';
    browserWasPlaying.current = false;
    setActiveSegment(0);
    setCurrentIdx((prev) => Math.min(Math.max(0, prev + delta), turns.length - 1));
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
        backgroundURL={backgroundURL}
        playerPortrait={showPlayerPortrait ? playerPortrait : undefined}
        npcPortrait={showNpcPortrait ? npcPortrait : undefined}
        playerLabel={playerLabel}
        npcLabel={npcLabel}
        playerActive={playerActive}
        npcActive={npcActive}
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
          audioState={beatState}
          audioMessage={beatStatus?.message}
          blocked={browser.blocked && !browser.playing}
          onToggle={togglePlay}
          onPrev={() => changeTurn(-1)}
          onNext={() => changeTurn(1)}
          onCycleSpeed={() => setSpeed((s) => (s === 1 ? 1.5 : s === 1.5 ? 2 : 1))}
          onUnblock={browser.play}
        />
      </div>
    </div>
  );
};
