import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { TheaterStage } from '../theater/TheaterStage';
import { TheaterDialogue } from '../theater/TheaterDialogue';
import { TheaterTransport } from '../theater/TheaterTransport';
import { Story, StoryBeat, StoryScene } from './types';

// The reveal takes the share of a beat the app's exported player already used, and
// BEAT_GAP_MS is the buffer the theatre keeps between a line and the next.
const TYPEWRITER_FRACTION = 0.6;
const BEAT_GAP_MS = 300;
const REVEAL_TICK_MS = 40;

interface QueuedBeat {
  scene: StoryScene;
  sceneIndex: number;
  beat: StoryBeat;
  index: number;
}

// StoryPlayer plays an exported story the way the in-app theatre plays a campaign:
// the same stage, portraits, name plate, dialogue panel, and transport, driving a
// scripted sequence instead of a live campaign. The text is revealed as it goes,
// a beat waits for its clips when it has them, and a beat without clips holds for
// the reading time it was compiled with.
export const StoryPlayer: React.FC<{ story: Story }> = ({ story }) => {
  const beats = useMemo<QueuedBeat[]>(() => {
    const queue: QueuedBeat[] = [];
    story.scenes.forEach((scene, sceneIndex) => {
      scene.beats.forEach((beat) => queue.push({ scene, sceneIndex, beat, index: queue.length }));
    });
    return queue;
  }, [story]);

  const [index, setIndex] = useState(0);
  // A story never starts itself. The play button is the gesture a browser needs before
  // it will play audio, so one control does both jobs.
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState(1);
  const [reveal, setReveal] = useState(1);
  // beatProgress drives the stage's Ken Burns, and runs over the whole beat.
  const [beatProgress, setBeatProgress] = useState(0);
  // Clips the browser would not play. A player cannot fix a clip the browser refuses, but
  // it can say so instead of leaving a line silently missing.
  const [unplayable, setUnplayable] = useState(0);

  const audioRef = useRef<HTMLAudioElement | null>(null);
  const generationRef = useRef(0);
  const elapsedRef = useRef(0);
  const beatDoneRef = useRef(false);

  const total = beats.length;
  const current = total > 0 ? beats[Math.min(index, total - 1)] : undefined;
  const reducedMotion = useMemo(
    () => typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches,
    []
  );

  const stopAudio = useCallback(() => {
    generationRef.current++;
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current = null;
    }
  }, []);

  const next = useCallback(() => setIndex((prev) => Math.min(prev + 1, Math.max(0, total - 1))), [total]);
  const prev = useCallback(() => setIndex((prev) => Math.max(prev - 1, 0)), []);

  // Show the current beat: reveal its text, play its clips, and hold it for as long as
  // it needs. A beat is never shorter than its compiled pace or the time a viewer needs
  // to read it, whichever is longer, plus the buffer between beats; a clip that cannot
  // play (a browser that wants a gesture) therefore leaves the reading time rather than
  // skipping the line.
  useEffect(() => {
    if (!current || !playing) return;

    stopAudio();
    const generation = generationRef.current;
    elapsedRef.current = 0;
    beatDoneRef.current = false;
    setReveal(reducedMotion ? 1 : 0);
    setBeatProgress(reducedMotion ? 1 : 0);

    const beat = current.beat;
    const clips = beat.audio ?? [];
    const revealMs = Math.max(1, beat.duration * 1000 * TYPEWRITER_FRACTION);
    const holdMs = Math.max(beat.duration * 1000, (beat.reading ?? 0) * 1000 + BEAT_GAP_MS);

    const advance = () => {
      if (generation !== generationRef.current || beatDoneRef.current) return;
      beatDoneRef.current = true;
      next();
    };

    // The reveal is a prefix of the text, paced by the beat's own reading time, and the
    // same clock holds the beat: the buffer is inside the hold, so nothing is counted
    // twice and no beat can flash past.
    let clipsDone = clips.length === 0;
    const tick = window.setInterval(() => {
      elapsedRef.current += REVEAL_TICK_MS * speed;
      if (!reducedMotion) {
        setReveal(Math.min(1, elapsedRef.current / revealMs));
        setBeatProgress(Math.min(1, elapsedRef.current / Math.max(1, holdMs)));
      }
      if (clipsDone && elapsedRef.current >= holdMs) advance();
    }, REVEAL_TICK_MS);

    if (clips.length > 0) {
      let clipIndex = 0;
      const step = () => {
        if (generation !== generationRef.current) return;
        if (clipIndex >= clips.length) {
          clipsDone = true;
          return;
        }
        const audio = new Audio(clips[clipIndex++]);
        audioRef.current = audio;
        audio.onended = step;
        audio.onerror = () => {
          setUnplayable((prev) => prev + 1);
          step();
        };
        audio.play().catch((err: unknown) => {
          // Audio the browser refuses to start must not shorten the beat: the line is
          // still held for its reading time. A refusal is counted, because a line that
          // plays nothing is otherwise indistinguishable from one with no clip.
          if (err instanceof Error && err.name !== 'NotAllowedError') {
            setUnplayable((prev) => prev + 1);
          }
          clipsDone = true;
        });
      };
      step();
    }

    return () => {
      window.clearInterval(tick);
      stopAudio();
    };
  }, [current, playing, speed, reducedMotion, next, stopAudio]);

  useEffect(() => stopAudio, [stopAudio]);

  const togglePlay = useCallback(() => {
    setPlaying((prev) => {
      if (prev) stopAudio();
      return !prev;
    });
  }, [stopAudio]);

  if (!current) {
    return (
      <div className="fixed inset-0 flex items-center justify-center bg-stone-950 text-stone-400 font-sans">
        This story has no scenes to play.
      </div>
    );
  }

  const scene = current.scene;
  const beat = current.beat;
  const isSpeech = beat.kind === 'speech';
  const isPlayer = isSpeech && !!beat.player;
  const isCard = beat.kind === 'scene_card';

  // The protagonist stays on the left for the whole story; the speaker's face takes
  // the right while they are the one talking, as the theatre shows them.
  const npcPortrait = isSpeech && !isPlayer ? beat.portrait : undefined;
  const npcLabel = isSpeech && !isPlayer ? beat.speaker ?? 'Unknown' : undefined;

  // The theatre falls back to the campaign's own image rather than to nothing.
  const backgroundURL = beat.art || scene.art || story.banner;
  const progress = total > 0 ? ((index + 1) / total) * 100 : 0;

  return (
    <div className="fixed inset-0 overflow-hidden select-none bg-stone-950 text-stone-100">
      <TheaterStage
        backgroundURL={backgroundURL}
        playerPortrait={story.player_portrait}
        playerLabel={story.player_name}
        npcPortrait={npcPortrait}
        npcLabel={npcLabel}
        playerActive={isPlayer}
        npcActive={!!npcPortrait}
        progress={beatProgress}
        seed={current.index}
        outcome={beat.outcome}
        weather={scene.weather}
        reducedMotion={reducedMotion}
      />

      <header className="absolute top-0 inset-x-0 z-20 px-6 py-4 flex flex-wrap items-center gap-3 bg-gradient-to-b from-black/80 to-transparent pointer-events-none">
        <span className="font-sans text-purple-300 font-bold tracking-[0.2em] text-sm">
          {story.game_name.toUpperCase()}
        </span>
        {scene.location && (
          <span className="text-xs font-sans text-stone-300 bg-black/50 px-2.5 py-1 rounded-full border border-white/10">
            {scene.location}
          </span>
        )}
        <span className="text-xs font-mono text-stone-400 bg-black/50 px-2 py-1 rounded border border-white/10">
          Scene {current.sceneIndex + 1} of {story.scenes.length}
        </span>
        {unplayable > 0 && (
          <span className="text-xs font-sans text-amber-300 bg-amber-950/60 px-2 py-1 rounded border border-amber-500/40">
            {unplayable} {unplayable === 1 ? 'line' : 'lines'} could not play
          </span>
        )}
      </header>

      <div className="absolute inset-x-0 bottom-0 z-20 flex flex-col items-center gap-3 pb-5 px-4">
        {isCard ? (
          // The theatre has no title card: a location change is a background change
          // there. A bundle keeps the card, so a new scene is introduced rather than
          // read as a line of narration.
          <div className="relative w-full max-w-4xl mx-auto min-h-[20vh] flex items-center justify-center">
            <span className="font-serif text-3xl tracking-[0.2em] uppercase text-amber-300 drop-shadow-lg text-center">
              {beat.text.slice(0, Math.max(1, Math.round(beat.text.length * reveal)))}
            </span>
          </div>
        ) : (
          <TheaterDialogue
            segment={{
              kind: isSpeech ? 'speech' : 'narration',
              speaker: beat.speaker,
              text: beat.text,
              portrait_url: beat.portrait,
              player: isPlayer,
              duration: beat.duration,
            }}
            fallback={beat.text}
            isPlayer={isPlayer}
            displayMode={story.display_mode}
            reveal={reveal}
            onAdvance={next}
          />
        )}

        <TheaterTransport
          progress={progress}
          isPlaying={playing}
          speed={speed}
          audioState="idle"
          labels={{ prev: 'Previous line', next: 'Next line' }}
          onToggle={togglePlay}
          onPrev={prev}
          onNext={next}
          onCycleSpeed={() => setSpeed((prev) => (prev === 1 ? 1.5 : prev === 1.5 ? 2 : 1))}
        />
      </div>
    </div>
  );
};
