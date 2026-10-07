import React, { useEffect, useRef } from 'react';
import { Turn, TurnSegment } from '../types';
import { TurnSegments, TurnAudioState } from './TurnSegments';
import { RecordNotice } from './RecordNotice';
import { Sparkles } from 'lucide-react';
import { useLightbox } from '../hooks/useLightbox';
import { ImageLightbox } from './ImageLightbox';

interface PendingAction {
  mode: string;
  text: string;
}

interface ChronicleViewProps {
  turns: Turn[];
  onWikilinkClick: (entityId: string) => void;
  autoPlay?: boolean;
  volume?: number;
  serverPlayback?: boolean;
  onPlayTurnAudio?: (turnNumber: number, segmentIndex?: number, force?: boolean) => void;
  onStopAudio?: () => void;
  onCorrect?: (note: string, turnNumber?: number) => void;
  addressedTurns?: Set<number>;
  onAddress?: (turnNumber: number) => void;
  turnInFlight?: boolean;
  pendingAction?: PendingAction | null;
  // The segments parsed so far, which render in place of the raw stream while a
  // turn runs: a control record never appears, because it produces no segment.
  streamedSegments?: TurnSegment[];
  displayMode?: 'stage_directions' | 'hidden' | 'raw';
  turnAudioStatus?: Record<number, { state: TurnAudioState; message?: string }>;
  segmentAudioStatus?: Record<string, { state: TurnAudioState; message?: string }>;
  characterPortraits?: Record<string, { url: string; hasCustom: boolean }>;
  segmentProgress?: Record<number, string>;
  // The campaign whose clips the beat controls regenerate.
  gameId?: string;
  // Clips already heard while the turn streamed, which playback must skip.
  skipAudioKeys?: ReadonlySet<string>;
  // onGenerateImage asks the server to illustrate a turn that has no image.
  onGenerateImage?: (turnNumber: number) => void;
}

export const ChronicleView: React.FC<ChronicleViewProps> = ({
  turns,
  onWikilinkClick,
  autoPlay = false,
  volume = 1,
  serverPlayback = false,
  onPlayTurnAudio,
  onStopAudio,
  turnInFlight,
  pendingAction,
  streamedSegments,
  displayMode,
  turnAudioStatus = {},
  segmentAudioStatus = {},
  characterPortraits,
  segmentProgress,
  gameId,
  skipAudioKeys,
  onGenerateImage,
}) => {
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const { lightbox, isLightboxOpen, openLightbox, closeLightbox } = useLightbox();

  // Auto-scroll when a new turn is added, turn starts, or a segment streams in
  useEffect(() => {
    if (turnInFlight || (streamedSegments?.length ?? 0) > 0) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [turnInFlight, streamedSegments, turns.length]);

  return (
    <div className="flex-1 overflow-y-auto px-8 py-6 space-y-6">
      {turns.length === 0 && !pendingAction ? (
        <div className="h-full flex items-center justify-center text-stone-500 font-sans tracking-wider text-sm italic">
          The chronicle awaits your first action...
        </div>
      ) : (
        turns.map((turn, index) => {
          const audioStatus = turnAudioStatus[turn.turn_number];
          const imageURL = turn.image_url;
          return (
            <div key={turn.turn_number} className="space-y-4 pb-6 border-b border-white/5 last:border-0">
              {/* Scene break indicator */}
              {turn.scene_break && (
                <div className="relative flex py-4 items-center" role="separator" aria-label="Scene break">
                  <div className="flex-grow border-t border-purple-500/30"></div>
                  <span className="flex-shrink mx-4 text-xs font-mono uppercase tracking-widest text-purple-400/80 bg-black/40 px-3 py-1 rounded-full border border-purple-500/20 shadow-sm flex items-center gap-1.5">
                    <Sparkles className="w-3.5 h-3.5 text-purple-400" />
                    Scene Break
                  </span>
                  <div className="flex-grow border-t border-purple-500/30"></div>
                </div>
              )}

              {/* Player Input Block. A spoken line is rendered as speech below, so
                  the input block is skipped for it to avoid printing it twice. */}
              {turn.input_text && !(turn.segments ?? []).some((segment) => segment.player) && (
                <div className="flex items-start gap-3 text-stone-300 text-sm font-sans italic bg-black/40 p-3.5 rounded-xl border border-white/5 shadow-inner">
                  <span className="text-purple-400 font-semibold uppercase tracking-wider text-xs font-sans">
                    [{turn.mode || 'Action'}]
                  </span>
                  <span>{turn.input_text}</span>
                  {turn.outcome && (
                    <span className="ml-auto text-xs font-mono text-stone-400">{turn.outcome}</span>
                  )}
                </div>
              )}

              {/* Scene Illustration if available */}
              {imageURL && (
                <div className="my-4 rounded-xl overflow-hidden border border-white/10 shadow-2xl">
                  <button
                    type="button"
                    onClick={() => openLightbox(imageURL, 'Scene illustration')}
                    className="block w-full cursor-zoom-in"
                    title="View full size scene illustration"
                    aria-label="View full size scene illustration"
                  >
                    <img src={imageURL} alt="Scene illustration" className="w-full object-cover max-h-96" />
                  </button>
                </div>
              )}

              {/* On-demand illustration, when the policy skipped this beat. */}
              {!imageURL && onGenerateImage && (
                <button
                  type="button"
                  onClick={() => onGenerateImage(turn.turn_number)}
                  className="my-2 flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg border border-stone-700 hover:border-purple-500/50 text-stone-400 hover:text-purple-300 cursor-pointer"
                >
                  <Sparkles className="w-3.5 h-3.5" />
                  <span>Generate image</span>
                </button>
              )}

              {/* Narrated prose and attributed speech, in playback order */}
              <TurnSegments
                segments={turn.segments}
                fallback={turn.prose}
                onEntityClick={onWikilinkClick}
                displayMode={displayMode}
                // Only the newest turn narrates itself: autoplaying every turn would
                // start them all at once on load.
                autoPlay={autoPlay && index === turns.length - 1}
                volume={volume}
                serverPlayback={serverPlayback}
                onPlayTurn={onPlayTurnAudio ? (segmentIndex, force) => onPlayTurnAudio(turn.turn_number, segmentIndex, force) : undefined}
                onStopTurn={onStopAudio}
                turnAudioState={audioStatus?.state}
                turnAudioMessage={audioStatus?.message}
                turnNumber={turn.turn_number}
                segmentAudioStatus={segmentAudioStatus}
                characterPortraits={characterPortraits}
                segmentProgress={index === turns.length - 1 ? segmentProgress : undefined}
                checks={turn.checks}
                gameId={gameId}
                skipAudioKeys={skipAudioKeys}
              />

              {turn.rejected && (
                <div className="text-xs font-sans text-amber-300 bg-amber-950/40 border border-amber-500/30 rounded-lg px-3 py-2">
                  That action was impossible.
                </div>
              )}

              <RecordNotice report={turn.record_report} />

              {turn.health_effects && turn.health_effects.length > 0 && (
                <div className="text-xs font-sans text-rose-300 bg-rose-950/40 border border-rose-500/30 rounded-lg px-3 py-2">
                  {turn.health_effects.map((effect, effectIndex) => (
                    <div key={effectIndex}>{effect.effect}</div>
                  ))}
                </div>
              )}

              {turn.world_tick && (
                <div className="text-xs font-sans text-purple-300/90 bg-purple-950/30 border border-purple-500/25 rounded-lg px-3 py-2">
                  {turn.world_tick}
                </div>
              )}

              {turn.recovery === 'trimmed' && (
                <div className="text-xs font-mono text-purple-400/80 pt-1">
                  The narrator's reply ended mid-thought; the unfinished tail was dropped.
                </div>
              )}

              {turn.tool_calls && turn.tool_calls.length > 0 && (
                <div className="text-xs font-mono text-stone-500 pt-1">
                  Looked up: {turn.tool_calls.map((call) => `${call.name} (${call.result_chars})`).join(', ')}
                </div>
              )}

              {turn.truncated && (
                <div className="text-xs font-mono text-purple-400/80 pt-1">
                  The narrator's reply could not be completed. Raise the response limit for the gm role in Settings, or
                  check the provider.
                </div>
              )}

              {turn.context_notes && turn.context_notes.length > 0 && (
                <div className="text-xs font-mono text-stone-500 pt-1">
                  Context trimmed to fit the prompt budget: {turn.context_notes.join(', ')}. Raise the context budget in
                  Settings to keep more.
                </div>
              )}

              {/* Entities involved in this turn */}
              {turn.entities_hit && turn.entities_hit.length > 0 && (
                <div className="flex flex-wrap items-center gap-2 pt-1">
                  {turn.entities_hit.map((entityId) => (
                    <button
                      key={entityId}
                      onClick={() => onWikilinkClick(entityId)}
                      className="px-2 py-0.5 text-xs font-sans tracking-wider rounded-full bg-white/5 border border-white/10 text-stone-300 hover:text-purple-300 hover:border-purple-500/60 cursor-pointer transition-colors"
                    >
                      {entityId}
                    </button>
                  ))}
                </div>
              )}
            </div>
          );
        })
      )}

      {/* Pending Turn in Flight */}
      {turnInFlight && pendingAction && (
        <div className="space-y-4 pb-6 anim-fade-in">
          {/* Immediate Action Bubble */}
          {pendingAction.text && !(streamedSegments ?? []).some((segment) => segment.player) && (
            <div className="flex items-start gap-3 text-stone-300 text-sm font-sans italic bg-black/40 p-3.5 rounded-xl border border-purple-500/20 shadow-inner">
              <span className="text-purple-400 font-semibold uppercase tracking-wider text-xs font-sans">
                [{pendingAction.mode || 'Action'}]
              </span>
              <span>{pendingAction.text}</span>
            </div>
          )}

          {/* Drafting feedback card, or the parsed segments as they arrive */}
          {(streamedSegments?.length ?? 0) > 0 ? (
            <TurnSegments
              segments={streamedSegments}
              onEntityClick={onWikilinkClick}
              displayMode={displayMode}
              characterPortraits={characterPortraits}
              segmentProgress={segmentProgress}
            />
          ) : (
            <div className="flex items-center gap-3 p-4 rounded-xl bg-purple-950/20 border border-purple-500/30 text-stone-300 text-sm animate-pulse">
              <div className="p-2 rounded-lg bg-purple-600/20 text-purple-400">
                <Sparkles className="w-4 h-4 animate-spin" />
              </div>
              <div className="space-y-0.5">
                <div className="font-sans text-xs font-bold text-purple-400 uppercase tracking-wider">
                  The narrator is drafting the scene...
                </div>
                <div className="text-xs text-stone-400 font-sans">
                  Weaving your action into the chronicle.
                </div>
              </div>
            </div>
          )}
        </div>
      )}

      {lightbox && (
        <ImageLightbox isOpen={isLightboxOpen} src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}

      <div ref={bottomRef} />
    </div>
  );
};
