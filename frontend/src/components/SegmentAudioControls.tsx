import React from 'react';
import { Loader2, Play, RotateCw, Square } from 'lucide-react';
import type { TurnAudioState } from './TurnSegments';

interface SegmentAudioControlsProps {
  state: TurnAudioState;
  message?: string;
  // grouped marks a control that covers a whole clip group rather than one line.
  grouped?: boolean;
  onPlay: () => void;
  onStop: () => void;
  onRegenerate: () => void;
}

export const SegmentAudioControls: React.FC<SegmentAudioControlsProps> = ({
  state,
  message,
  grouped = false,
  onPlay,
  onStop,
  onRegenerate,
}) => {
  const generating = state === 'generating';
  const playing = state === 'playing';
  const what = grouped ? 'this group' : 'this line';

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
        title={`Play ${what}`}
        aria-label={`Play ${what}`}
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
        title={`Stop ${what}`}
        aria-label={`Stop ${what}`}
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
        title={`Regenerate ${what}`}
        aria-label={`Regenerate ${what}`}
      >
        <RotateCw className="w-3 h-3" />
      </button>
      {state === 'error' && message && (
        <span className="max-w-[140px] truncate text-xs text-rose-300" title={message}>
          Error: {message}
        </span>
      )}
    </div>
  );
};
