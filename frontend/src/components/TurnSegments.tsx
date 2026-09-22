import React from 'react';
import { TurnSegment } from '../types';
import { useSegmentPlayback } from '../hooks/useSegmentPlayback';
import { MarkdownProse } from './MarkdownProse';

interface TurnSegmentsProps {
  segments?: TurnSegment[];
  fallback: string;
  onEntityClick?: (entityId: string) => void;
  autoPlay?: boolean;
  volume?: number;
}

export const TurnSegments: React.FC<TurnSegmentsProps> = ({
  segments,
  fallback,
  onEntityClick,
  autoPlay = false,
  volume = 1,
}) => {
  const ordered = segments && segments.length > 0 ? segments : [{ kind: 'narration' as const, text: fallback }];
  const hasAudio = (segments ?? []).some((segment) => !!segment.audio_url);
  const { playing, play, playFrom, stop } = useSegmentPlayback(segments, autoPlay && hasAudio, volume);

  return (
    <div className="space-y-3">
      {ordered.map((segment, i) =>
        segment.kind === 'speech' ? (
          <div
            key={i}
            className="bg-glass-card border-l-4 border-amber-500/90 pl-4 py-3 pr-4 rounded-r-xl shadow-lg space-y-2"
          >
            {segment.audio_url ? (
              <button
                onClick={() => playFrom(i)}
                className="text-xs text-amber-400 font-cinzel font-bold tracking-widest hover:text-amber-300 cursor-pointer"
              >
                {segment.speaker || 'UNKNOWN'}
              </button>
            ) : (
              <div className="text-xs text-amber-400 font-cinzel font-bold tracking-widest">
                {segment.speaker || 'UNKNOWN'}
              </div>
            )}
            <MarkdownProse
              text={`\u201c${segment.text}\u201d`}
              onEntityClick={onEntityClick}
              className="text-stone-100 text-lg leading-relaxed italic space-y-2"
            />
          </div>
        ) : (
          <MarkdownProse
            key={i}
            text={segment.text}
            onEntityClick={onEntityClick}
            className="text-stone-200 text-xl leading-relaxed tracking-wide font-serif space-y-4"
          />
        )
      )}
      {hasAudio && (
        <div className="flex items-center gap-2 text-xs text-stone-400">
          <button onClick={playing ? stop : play} className="hover:text-amber-300 cursor-pointer">
            {playing ? 'Pause' : 'Play turn'}
          </button>
        </div>
      )}
    </div>
  );
};
