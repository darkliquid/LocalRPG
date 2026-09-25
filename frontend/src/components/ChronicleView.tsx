import React, { useEffect, useRef } from 'react';
import { Turn } from '../types';
import { TurnSegments } from './TurnSegments';
import { Sparkles } from 'lucide-react';

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
  onPlayTurnAudio?: (turnNumber: number, segmentIndex?: number) => void;
  onStopAudio?: () => void;
  onCorrect?: (note: string, turnNumber?: number) => void;
  addressedTurns?: Set<number>;
  onAddress?: (turnNumber: number) => void;
  turnInFlight?: boolean;
  pendingAction?: PendingAction | null;
  streamedProse?: string;
  displayMode?: 'stage_directions' | 'hidden' | 'raw';
}

export const ChronicleView: React.FC<ChronicleViewProps> = ({
  turns,
  onWikilinkClick,
  autoPlay = false,
  volume = 1,
  serverPlayback = false,
  onPlayTurnAudio,
  onStopAudio,
  onCorrect,
  addressedTurns,
  onAddress,
  turnInFlight,
  pendingAction,
  streamedProse,
  displayMode,
}) => {
  const bottomRef = useRef<HTMLDivElement | null>(null);

  // Auto-scroll when a new turn is added, turn starts, or prose streams in
  useEffect(() => {
    if (turnInFlight || streamedProse) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [turnInFlight, streamedProse, turns.length]);

  // Art is per scene, not per turn: it is shown when the party arrives somewhere
  // new and reused while they stay.
  let previousLocationID: string | undefined;
  const beats = turns.map((turn) => {
    const isSceneChange = !!turn.location_id && turn.location_id !== previousLocationID;
    previousLocationID = turn.location_id ?? previousLocationID;
    return { turn, isSceneChange };
  });

  return (
    <div className="flex-1 overflow-y-auto px-8 py-6 space-y-6">
      {turns.length === 0 && !pendingAction ? (
        <div className="h-full flex items-center justify-center text-stone-500 font-sans tracking-wider text-sm italic">
          The chronicle awaits your first action...
        </div>
      ) : (
        beats.map(({ turn, isSceneChange }, index) => (
          <div key={turn.turn_number} className="space-y-4 pb-6 border-b border-white/5 last:border-0">
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
            {turn.image_url && (
              <div className="my-4 rounded-xl overflow-hidden border border-white/10 shadow-2xl">
                <img src={turn.image_url} alt="Scene illustration" className="w-full object-cover max-h-96" />
              </div>
            )}

            {/* Scene art, when the party has moved somewhere new */}
            {turn.location_art_url && isSceneChange && (
              <div className="my-4 rounded-xl overflow-hidden border border-white/10 shadow-2xl">
                <img
                  src={turn.location_art_url}
                  alt={turn.location_name || 'Scene'}
                  className="w-full object-cover max-h-96"
                />
                {turn.location_name && (
                  <div className="px-3 py-2 text-xs font-sans tracking-widest text-stone-400 uppercase">
                    {turn.location_name}
                  </div>
                )}
              </div>
            )}

            {/* Narrated prose and attributed speech, in playback order */}
            <TurnSegments
              segments={turn.segments}
              fallback={turn.prose}
              onEntityClick={onWikilinkClick}
              displayMode={displayMode}
              // Only the newest turn narrates itself: autoplaying every turn would
              // start them all at once on load.
              autoPlay={autoPlay && index === beats.length - 1}
              volume={volume}
              serverPlayback={serverPlayback}
              onPlayTurn={onPlayTurnAudio ? (segmentIndex) => onPlayTurnAudio(turn.turn_number, segmentIndex) : undefined}
              onStopTurn={onStopAudio}
            />

            {turn.rejected && (
              <div className="text-xs font-sans text-amber-300 bg-amber-950/40 border border-amber-500/30 rounded-lg px-3 py-2">
                That action was impossible{turn.verdict?.reason ? `: ${turn.verdict.reason}` : '.'}
              </div>
            )}

            {turn.checks && turn.checks.length > 0 && (
              <div className="text-xs font-mono text-stone-400">
                {turn.checks.map((check) => (
                  <span key={check.check_id} className="mr-3">
                    {check.roll ? `${check.roll.notation}=${check.roll.total} ` : ''}
                    {check.outcome}
                  </span>
                ))}
              </div>
            )}

            {turn.recovery === 'trimmed' && (
              <div className="text-xs font-mono text-purple-400/80 pt-1">
                The narrator's reply ended mid-thought; the unfinished tail was dropped.
              </div>
            )}

            {turn.tool_calls && turn.tool_calls.length > 0 && (
              <div className="text-[11px] font-mono text-stone-500 pt-1">
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

            {turn.continuity_notes && turn.continuity_notes.length > 0 && (
              <div
                className={`text-xs font-mono pt-1 space-y-1 ${
                  addressedTurns?.has(turn.turn_number) ? 'text-stone-500 opacity-60' : 'text-purple-400/90'
                }`}
              >
                {turn.continuity_notes.map((note, index) => (
                  <div key={index} className="flex items-start gap-2">
                    <span>{note}</span>
                    {addressedTurns?.has(turn.turn_number) ? (
                      <span className="shrink-0 px-1.5 py-0.5 rounded border border-stone-700 text-stone-500 text-[10px]">
                        Addressed
                      </span>
                    ) : (
                      <div className="flex items-center gap-1.5 shrink-0">
                        <button
                          onClick={() => onCorrect?.(note, turn.turn_number)}
                          className="px-1.5 py-0.5 rounded border border-purple-500/40 hover:bg-purple-600/20 cursor-pointer transition-colors"
                          title="Review or correct this finding"
                        >
                          Correct
                        </button>
                        {onAddress && (
                          <button
                            onClick={() => onAddress(turn.turn_number)}
                            className="px-1.5 py-0.5 rounded border border-stone-600 hover:bg-stone-800 text-stone-400 cursor-pointer transition-colors text-[10px]"
                            title="Mark this finding as addressed"
                          >
                            Dismiss
                          </button>
                        )}
                      </div>
                    )}
                  </div>
                ))}
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
        ))
      )}

      {/* Pending Turn in Flight */}
      {turnInFlight && pendingAction && (
        <div className="space-y-4 pb-6 animate-fade-in">
          {/* Immediate Action Bubble */}
          {pendingAction.text && (
            <div className="flex items-start gap-3 text-stone-300 text-sm font-sans italic bg-black/40 p-3.5 rounded-xl border border-purple-500/20 shadow-inner">
              <span className="text-purple-400 font-semibold uppercase tracking-wider text-xs font-sans">
                [{pendingAction.mode || 'Action'}]
              </span>
              <span>{pendingAction.text}</span>
            </div>
          )}

          {/* Drafting feedback card or streaming prose */}
          {streamedProse ? (
            <TurnSegments
              segments={[{ kind: 'narration', text: streamedProse }]}
              fallback={streamedProse}
              onEntityClick={onWikilinkClick}
              displayMode={displayMode}
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

      <div ref={bottomRef} />
    </div>
  );
};
