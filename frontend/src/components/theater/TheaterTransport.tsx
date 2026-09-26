import React from 'react';
import { Loader2, Pause, Play, SkipBack, SkipForward, Volume2 } from 'lucide-react';
import { TurnAudioState } from '../TurnSegments';

interface TheaterTransportProps {
  progress: number;
  isPlaying: boolean;
  speed: number;
  audioState: TurnAudioState;
  audioMessage?: string;
  blocked?: boolean;
  onToggle: () => void;
  onPrev: () => void;
  onNext: () => void;
  onCycleSpeed: () => void;
  onUnblock: () => void;
}

// TheaterTransport is deliberately just transport: the single play/pause button
// owns playback. Per-line controls live in the chronicle, not here.
export const TheaterTransport: React.FC<TheaterTransportProps> = ({
  progress,
  isPlaying,
  speed,
  audioState,
  audioMessage,
  blocked = false,
  onToggle,
  onPrev,
  onNext,
  onCycleSpeed,
  onUnblock,
}) => {
  const generating = audioState === 'generating';

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

        {generating && (
          <span className="inline-flex items-center gap-1 px-2 py-1 rounded-full bg-purple-900/50 border border-purple-500/40 text-purple-300 text-xs">
            <Loader2 className="w-3 h-3 animate-spin" />
            <span>Rendering speech…</span>
          </span>
        )}

        {blocked && (
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
          <span
            className="inline-flex items-center gap-1 px-2 py-1 rounded-full bg-rose-900/40 border border-rose-500/40 text-rose-300 text-xs"
            title={audioMessage}
          >
            Error: {audioMessage}
          </span>
        )}
      </div>
    </div>
  );
};
