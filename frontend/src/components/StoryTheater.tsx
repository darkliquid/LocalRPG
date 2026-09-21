import React, { useState, useEffect } from 'react';
import { Turn } from '../types';
import { TurnSegments } from './TurnSegments';
import { Play, Pause, SkipBack, SkipForward, X } from 'lucide-react';

interface StoryTheaterProps {
  turns: Turn[];
  isOpen: boolean;
  onClose: () => void;
  autoPlay?: boolean;
  volume?: number;
}

export const StoryTheater: React.FC<StoryTheaterProps> = ({
  turns,
  isOpen,
  onClose,
  autoPlay = false,
  volume = 1,
}) => {
  const [currentIdx, setCurrentIdx] = useState(0);
  const [isPlaying, setIsPlaying] = useState(true);
  const [speed, setSpeed] = useState<number>(1);

  const currentTurn = turns[currentIdx];

  // A turn is held for the reading time its segments report, so the in-app player
  // and a rendered bundle hold a line for the same length of time. The fixed span
  // remains as the fallback for a turn recorded before durations existed.
  const reportedMs = (currentTurn?.segments ?? []).reduce(
    (total, segment) => total + (segment.duration ?? 0) * 1000,
    0
  );
  const turnDurationMs = reportedMs > 0 ? reportedMs : 4000;

  useEffect(() => {
    if (!isOpen || !isPlaying || turns.length === 0) return;

    const interval = setTimeout(() => {
      if (currentIdx < turns.length - 1) {
        setCurrentIdx((prev) => prev + 1);
      } else {
        setIsPlaying(false);
      }
    }, turnDurationMs / speed);

    return () => clearTimeout(interval);
  }, [isOpen, isPlaying, currentIdx, turns.length, speed, turnDurationMs]);

  if (!isOpen || turns.length === 0) return null;

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-stone-950 text-stone-100 overflow-hidden select-none">
      {/* Dynamic Background */}
      <div
        className="absolute inset-0 bg-cover bg-center transition-all duration-700 pointer-events-none"
        style={{
          backgroundImage: currentTurn?.image_url ? `url(${currentTurn.image_url})` : 'radial-gradient(ellipse at center, #261e1b 0%, #0c0a09 100%)',
        }}
      />
      <div className="absolute inset-0 bg-radial-[circle_at_center] from-black/40 via-black/70 to-black/95 pointer-events-none" />

      {/* Top Controls */}
      <header className="relative z-10 p-6 flex justify-between items-center bg-gradient-to-b from-black/80 to-transparent">
        <div className="flex items-center gap-3">
          <span className="font-cinzel text-amber-400 font-bold tracking-widest text-lg">STORY THEATER</span>
          <span className="text-xs font-mono text-stone-400 bg-stone-900/60 px-2 py-1 rounded border border-white/10">
            Turn {currentIdx + 1} of {turns.length}
          </span>
        </div>
        <button
          onClick={onClose}
          className="p-2 rounded-full hover:bg-white/10 text-stone-400 hover:text-white transition-colors cursor-pointer"
        >
          <X className="w-6 h-6" />
        </button>
      </header>

      {/* Main Dialogue Card */}
      <main className="relative z-10 flex-1 flex items-center justify-center p-8">
        <div className="max-w-3xl w-full bg-glass-card rounded-2xl p-8 shadow-2xl border border-white/10 space-y-4">
          <TurnSegments
            segments={currentTurn?.segments}
            fallback={currentTurn?.prose ?? ''}
            autoPlay={autoPlay}
            volume={volume}
          />
        </div>
      </main>

      {/* Bottom Transport Controls */}
      <footer className="relative z-10 p-6 bg-gradient-to-t from-black/90 to-transparent flex flex-col items-center gap-4">
        {/* Progress Bar */}
        <div className="w-full max-w-2xl h-1.5 bg-stone-900 rounded-full overflow-hidden border border-white/5">
          <div
            className="h-full bg-amber-500 transition-all duration-300"
            style={{ width: `${((currentIdx + 1) / turns.length) * 100}%` }}
          />
        </div>

        {/* Buttons */}
        <div className="flex items-center gap-4">
          <button
            onClick={() => setCurrentIdx((p) => Math.max(0, p - 1))}
            disabled={currentIdx === 0}
            className="p-2.5 rounded-full hover:bg-white/10 disabled:opacity-30 cursor-pointer"
          >
            <SkipBack className="w-5 h-5" />
          </button>

          <button
            onClick={() => setIsPlaying(!isPlaying)}
            className="p-4 rounded-full bg-amber-600 hover:bg-amber-500 text-stone-950 font-bold shadow-lg transition-transform hover:scale-105 cursor-pointer"
          >
            {isPlaying ? <Pause className="w-6 h-6" /> : <Play className="w-6 h-6 ml-0.5" />}
          </button>

          <button
            onClick={() => setCurrentIdx((p) => Math.min(turns.length - 1, p + 1))}
            disabled={currentIdx === turns.length - 1}
            className="p-2.5 rounded-full hover:bg-white/10 disabled:opacity-30 cursor-pointer"
          >
            <SkipForward className="w-5 h-5" />
          </button>

          <button
            onClick={() => setSpeed((s) => (s === 1 ? 1.5 : s === 1.5 ? 2 : 1))}
            className="px-3 py-1 rounded-lg bg-stone-900 border border-white/10 text-xs font-mono font-bold text-amber-400 hover:bg-stone-800"
          >
            {speed}x
          </button>
        </div>
      </footer>
    </div>
  );
};
