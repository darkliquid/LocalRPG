import React from 'react';
import { TurnSegment, TurnCheck } from '../types';
import { useSegmentPlayback } from '../hooks/useSegmentPlayback';
import { anySegmentHasAudio, segmentIsGroupLeader } from '../lib/audio';
import { MarkdownProse } from './MarkdownProse';
import { ImageLightbox } from './ImageLightbox';
import { useLightbox } from '../hooks/useLightbox';
import { DiceCheckCard } from './DiceCheckCard';
import { Play, Square, RotateCw, Loader2 } from 'lucide-react';
import { SegmentAudioControls } from './SegmentAudioControls';

export const segmentAudioKey = (turnNumber: number, segmentIndex: number): string =>
  `${turnNumber}:${segmentIndex}`;

export type TurnAudioState = 'idle' | 'generating' | 'playing' | 'error';

interface TurnSegmentsProps {
  segments?: TurnSegment[];
  fallback: string;
  onEntityClick?: (entityId: string) => void;
  autoPlay?: boolean;
  volume?: number;
  // When set, playback is the application's job: the browser never starts audio,
  // so nothing depends on an autoplay gesture.
  serverPlayback?: boolean;
  onPlayTurn?: (segmentIndex?: number, force?: boolean) => void;
  onStopTurn?: () => void;
  // Managed externally by App when serverPlayback is true
  turnAudioState?: TurnAudioState;
  turnAudioMessage?: string;
  turnNumber?: number;
  gameId?: string;
  segmentAudioStatus?: Record<string, { state: TurnAudioState; message?: string }>;
  // Checks resolved this turn, rendered inline at the segment that narrates them.
  checks?: TurnCheck[];
  displayMode?: 'stage_directions' | 'hidden' | 'raw';
  // Clips already heard while the turn streamed, which playback must skip.
  skipAudioKeys?: ReadonlySet<string>;
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
  turnAudioState = 'idle',
  turnAudioMessage,
  turnNumber,
  gameId,
  segmentAudioStatus,
  checks,
  displayMode = 'stage_directions',
  skipAudioKeys,
}) => {
  const ordered = segments && segments.length > 0 ? segments : [{ kind: 'narration' as const, text: fallback }];
  const hasAudio = anySegmentHasAudio(segments);
  const { playing, blocked, playingIndex, play, playFrom, regenerateFrom, stop } = useSegmentPlayback(
    segments,
    {
      autoPlay: autoPlay && hasAudio && !serverPlayback,
      volume,
      skipKeys: skipAudioKeys,
      gameId,
      turnNumber,
    }
  );

  const startServerPlayback = (segmentIndex?: number, force?: boolean) => onPlayTurn?.(segmentIndex, force);
  const stopServerPlayback = () => onStopTurn?.();

  const isGenerating = turnAudioState === 'generating';
  const isPlaying = turnAudioState === 'playing';
  const isError = turnAudioState === 'error';

  const segmentState = (index: number): { state: TurnAudioState; message?: string } => {
    if (serverPlayback) {
      const key = turnNumber !== undefined ? segmentAudioKey(turnNumber, index) : '';
      return segmentAudioStatus?.[key] ?? { state: 'idle' };
    }
    return playingIndex === index ? { state: 'playing' } : { state: 'idle' };
  };

  const segmentControls = (index: number) => {
    if (!ordered[index]?.audio_urls?.length) return null;
    // A group's control renders once, on its first segment: the hover belongs to
    // the whole clip, not to each segment it covers.
    if (!segmentIsGroupLeader(ordered, index)) return null;
    const status = segmentState(index);
    return (
      <SegmentAudioControls
        state={status.state}
        message={status.message}
        grouped={!!ordered[index]?.clip_group}
        onPlay={() => (serverPlayback ? startServerPlayback(index) : playFrom(index))}
        onStop={() => (serverPlayback ? stopServerPlayback() : stop())}
        onRegenerate={() => (serverPlayback ? startServerPlayback(index, true) : regenerateFrom(index))}
      />
    );
  };

  const { lightbox, isLightboxOpen, openLightbox, closeLightbox } = useLightbox();

  // Merge checks into the segment stream so a roll renders immediately before the
  // line it produced. A check the GM did not attach leads the prose instead of
  // trailing it, so cause reads before effect.
  const checkByID = new Map((checks ?? []).map((check) => [check.check_id, check]));
  const usedChecks = new Set<string>();
  const stream: Array<{ segment?: TurnSegment; check?: TurnCheck; index?: number }> = [];
  ordered.forEach((segment, index) => {
    if (segment.check_ref) {
      const check = checkByID.get(segment.check_ref);
      if (check && !usedChecks.has(check.check_id)) {
        usedChecks.add(check.check_id);
        stream.push({ check });
      }
    }
    stream.push({ segment, index });
  });
  const unattached = (checks ?? []).filter((check) => !usedChecks.has(check.check_id));
  if (unattached.length > 0) {
    stream.unshift(...unattached.map((check) => ({ check })));
  }

  return (
    <div className="space-y-3">
      {stream.map((item, streamIndex) => {
        if (item.check) {
          return <DiceCheckCard key={`check-${item.check.check_id}`} check={item.check} />;
        }
        const segment = item.segment as TurnSegment;
        const i = item.index as number;
        return segment.kind === 'speech' ? (
          <div
            key={streamIndex}
            className={`group relative bg-glass-card border-l-4 pl-4 py-3 pr-4 rounded-r-xl shadow-lg space-y-2 anim-fade-in ${
              segment.player ? 'border-sky-400/90' : 'border-purple-500/90'
            }`}
          >
            {segmentControls(i)}
            <div className="flex items-center gap-3">
              {segment.portrait_url && (
                <div
                  onClick={() => openLightbox(segment.portrait_url!, segment.speaker || 'Portrait')}
                  className={`w-9 h-9 rounded-full overflow-hidden shrink-0 border-2 shadow-md cursor-zoom-in transition-transform hover:scale-105 ${
                    segment.player ? 'border-sky-400/80' : 'border-purple-400/80'
                  }`}
                  title={`View portrait of ${segment.speaker || 'character'}`}
                >
                  <img
                    src={segment.portrait_url}
                    alt={segment.speaker || 'Speaker portrait'}
                    className="w-full h-full object-cover"
                    loading="lazy"
                  />
                </div>
              )}
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
            </div>
            <MarkdownProse
              text={`\u201c${segment.text}\u201d`}
              onEntityClick={onEntityClick}
              displayMode={displayMode}
              className="text-stone-100 text-lg leading-relaxed italic space-y-2"
            />
          </div>
        ) : (
          <div key={streamIndex} className="group relative anim-fade-in">
            {segmentControls(i)}
            <MarkdownProse
              text={segment.text}
              onEntityClick={onEntityClick}
              displayMode={displayMode}
              className="text-stone-200 text-xl leading-relaxed tracking-wide font-serif space-y-4"
            />
          </div>
        );
      })}

      {/* Server-side playback controls */}
      {hasAudio && serverPlayback && (
        <div className="flex flex-wrap items-center gap-2 text-xs">
          {/* Play button — spinner while generating, disabled while playing */}
          <button
            onClick={() => !isGenerating && !isPlaying ? startServerPlayback() : undefined}
            disabled={isGenerating || isPlaying}
            className={`inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border transition-colors ${
              isGenerating || isPlaying
                ? 'opacity-40 cursor-not-allowed bg-stone-900/70 border-stone-700 text-stone-400'
                : 'bg-stone-900/70 border-stone-700 text-stone-300 hover:text-purple-300 hover:border-purple-500/40 cursor-pointer'
            }`}
          >
            {isGenerating
              ? <><Loader2 className="w-3.5 h-3.5 animate-spin" /><span>Generating speech…</span></>
              : <><Play className="w-3 h-3" /><span>Play turn</span></>
            }
          </button>

          {/* Stop button — only enabled while playing */}
          <button
            onClick={isPlaying ? stopServerPlayback : undefined}
            disabled={!isPlaying}
            className={`inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border transition-colors ${
              isPlaying
                ? 'text-rose-400 border-rose-500/50 hover:bg-rose-500/20 cursor-pointer bg-stone-900/70'
                : 'opacity-30 cursor-not-allowed pointer-events-none bg-stone-900/70 border-stone-700 text-stone-400'
            }`}
          >
            <Square className="w-3 h-3" />
            <span>Stop</span>
          </button>

          {/* Force regenerate button */}
          <button
            onClick={() => !isGenerating && !isPlaying ? startServerPlayback(undefined, true) : undefined}
            disabled={isGenerating || isPlaying}
            title="Force regenerate speech"
            className={`inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border transition-colors ${
              isGenerating || isPlaying
                ? 'opacity-30 cursor-not-allowed pointer-events-none bg-stone-900/70 border-stone-700 text-stone-400'
                : 'bg-stone-900/70 border-stone-700 text-stone-400 hover:text-amber-300 hover:border-amber-500/40 cursor-pointer'
            }`}
          >
            <RotateCw className="w-3 h-3" />
            <span>Regenerate</span>
          </button>

          {/* Status chip */}
          {isGenerating && (
            <span className="inline-flex items-center gap-1 px-2 py-1 rounded-full bg-purple-900/50 border border-purple-500/40 text-purple-300 animate-pulse">
              <span className="w-1.5 h-1.5 rounded-full bg-purple-400 inline-block" />
              Rendering speech (calling provider)…
            </span>
          )}
          {isError && turnAudioMessage && (
            <span className="inline-flex items-center gap-1 px-2 py-1 rounded-full bg-rose-900/40 border border-rose-500/40 text-rose-300">
              <span className="w-1.5 h-1.5 rounded-full bg-rose-400 inline-block" />
              Error: {turnAudioMessage}
            </span>
          )}
        </div>
      )}

      {/* Client-side browser playback controls */}
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
      {lightbox && (
        <ImageLightbox isOpen={isLightboxOpen} src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}
    </div>
  );
};
