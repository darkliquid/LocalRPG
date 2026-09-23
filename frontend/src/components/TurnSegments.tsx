import React from 'react';
import { TurnSegment } from '../types';
import { useSegmentPlayback } from '../hooks/useSegmentPlayback';
import { MarkdownProse } from './MarkdownProse';
import { Play, Square } from 'lucide-react';

interface TurnSegmentsProps {
  segments?: TurnSegment[];
  fallback: string;
  onEntityClick?: (entityId: string) => void;
  autoPlay?: boolean;
  volume?: number;
  // When set, playback is the application's job: the browser never starts audio,
  // so nothing depends on an autoplay gesture.
  serverPlayback?: boolean;
  onPlayTurn?: (segmentIndex?: number) => void;
  onStopTurn?: () => void;
  displayMode?: 'stage_directions' | 'hidden' | 'raw';
}

export const TurnSegments: React.FC<TurnSegmentsProps> = ({
  segments,
  fallback,
  onEntityClick,
  autoPlay = false,
  volume = 1,
  serverPlayback = false,
  onPlayTurn,
  onStopTurn,
  displayMode = 'stage_directions',
}) => {
  const ordered = segments && segments.length > 0 ? segments : [{ kind: 'narration' as const, text: fallback }];
  const hasAudio = (segments ?? []).some((segment) => !!segment.audio_url);
  const { playing, blocked, play, playFrom, stop } = useSegmentPlayback(
    segments,
    autoPlay && hasAudio && !serverPlayback,
    volume
  );

  const startServerPlayback = (segmentIndex?: number) => onPlayTurn?.(segmentIndex);
  const stopServerPlayback = () => onStopTurn?.();

  return (
    <div className="space-y-3">
      {ordered.map((segment, i) =>
        segment.kind === 'speech' ? (
          <div
            key={i}
            className={`bg-glass-card border-l-4 pl-4 py-3 pr-4 rounded-r-xl shadow-lg space-y-2 ${
              segment.player ? 'border-sky-400/90' : 'border-purple-500/90'
            }`}
          >
            {hasAudio ? (
              <button
                onClick={() => (serverPlayback ? startServerPlayback(i) : playFrom(i))}
                className={`text-xs font-sans font-bold tracking-widest hover:opacity-80 cursor-pointer ${
                  segment.player ? 'text-sky-300' : 'text-purple-400'
                }`}
              >
                {segment.speaker || 'UNKNOWN'}
                {segment.player && <span className="ml-2 text-stone-400 normal-case">(you)</span>}
              </button>
            ) : (
              <div
                className={`text-xs font-sans font-bold tracking-widest ${
                  segment.player ? 'text-sky-300' : 'text-purple-400'
                }`}
              >
                {segment.speaker || 'UNKNOWN'}
                {segment.player && <span className="ml-2 text-stone-400 normal-case">(you)</span>}
              </div>
            )}
            <MarkdownProse
              text={`\u201c${segment.text}\u201d`}
              onEntityClick={onEntityClick}
              displayMode={displayMode}
              className="text-stone-100 text-lg leading-relaxed italic space-y-2"
            />
          </div>
        ) : (
          <MarkdownProse
            key={i}
            text={segment.text}
            onEntityClick={onEntityClick}
            displayMode={displayMode}
            className="text-stone-200 text-xl leading-relaxed tracking-wide font-serif space-y-4"
          />
        )
      )}
      {hasAudio && serverPlayback && (
        <div className="flex items-center gap-2 text-xs">
          <button
            onClick={() => startServerPlayback()}
            className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-stone-900/70 border border-stone-700 text-stone-300 hover:text-purple-300 hover:border-purple-500/40 cursor-pointer transition-colors"
          >
            <Play className="w-3 h-3" />
            <span>Play turn</span>
          </button>
          <button
            onClick={stopServerPlayback}
            className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-stone-900/70 border border-stone-700 text-stone-300 hover:text-red-300 hover:border-red-500/40 cursor-pointer transition-colors"
          >
            <Square className="w-3 h-3" />
            <span>Stop</span>
          </button>
        </div>
      )}
      {hasAudio && !serverPlayback && (
        <div className="flex items-center gap-2 text-xs">
          {blocked && !playing ? (
            <>
              <button
                onClick={play}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-purple-600 hover:bg-purple-500 text-white font-sans font-bold cursor-pointer transition-colors"
              >
                <Play className="w-3.5 h-3.5 fill-stone-950" />
                <span>Play narration</span>
              </button>
              <span className="text-stone-500">The browser needs a click before it will play audio.</span>
            </>
          ) : (
            <button
              onClick={playing ? stop : play}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-stone-900/70 border border-stone-700 text-stone-300 hover:text-purple-300 hover:border-purple-500/40 cursor-pointer transition-colors"
            >
              {playing ? <Square className="w-3 h-3" /> : <Play className="w-3 h-3" />}
              <span>{playing ? 'Pause' : 'Play turn'}</span>
            </button>
          )}
        </div>
      )}
    </div>
  );
};
