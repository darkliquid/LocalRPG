import React from 'react';
import { Turn } from '../types';
import { TurnSegments } from './TurnSegments';

interface ChronicleViewProps {
  turns: Turn[];
  onWikilinkClick: (entityId: string) => void;
  autoPlay?: boolean;
  volume?: number;
  serverPlayback?: boolean;
  onPlayTurnAudio?: (turnNumber: number, segmentIndex?: number) => void;
  onStopAudio?: () => void;
  onCorrect?: (note: string) => void;
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
}) => {
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
      {turns.length === 0 ? (
        <div className="h-full flex items-center justify-center text-stone-500 font-cinzel tracking-wider text-sm italic">
          The chronicle awaits your first action...
        </div>
      ) : (
        beats.map(({ turn, isSceneChange }, index) => (
          <div key={turn.turn_number} className="space-y-4 pb-6 border-b border-white/5 last:border-0">
            {/* Player Input Block */}
            {turn.input_text && (
              <div className="flex items-start gap-3 text-stone-300 text-sm font-sans italic bg-black/40 p-3.5 rounded-xl border border-white/5 shadow-inner">
                <span className="text-amber-400 font-semibold uppercase tracking-wider text-xs font-cinzel">
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
                  <div className="px-3 py-2 text-xs font-cinzel tracking-widest text-stone-400 uppercase">
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
              // Only the newest turn narrates itself: autoplaying every turn would
              // start them all at once on load.
              autoPlay={autoPlay && index === beats.length - 1}
              volume={volume}
              serverPlayback={serverPlayback}
              onPlayTurn={onPlayTurnAudio ? (segmentIndex) => onPlayTurnAudio(turn.turn_number, segmentIndex) : undefined}
              onStopTurn={onStopAudio}
            />

            {turn.truncated && (
              <div className="text-xs font-mono text-amber-400/80 pt-1">
                The narrator was cut off by the model's token limit. Raise the response limit for the gm role in Settings.
              </div>
            )}

            {turn.context_notes && turn.context_notes.length > 0 && (
              <div className="text-xs font-mono text-stone-500 pt-1">
                Context trimmed to fit the prompt budget: {turn.context_notes.join(', ')}. Raise the context budget in
                Settings to keep more.
              </div>
            )}

            {turn.continuity_notes && turn.continuity_notes.length > 0 && (
              <div className="text-xs font-mono text-amber-400/90 pt-1 space-y-1">
                {turn.continuity_notes.map((note, index) => (
                  <div key={index} className="flex items-start gap-2">
                    <span>{note}</span>
                    <button
                      onClick={() => onCorrect?.(note)}
                      className="shrink-0 px-1.5 py-0.5 rounded border border-amber-500/40 hover:bg-amber-600/20 cursor-pointer transition-colors"
                      title="Send this as a correction to the GM"
                    >
                      Correct
                    </button>
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
                    className="px-2 py-0.5 text-xs font-cinzel tracking-wider rounded-full bg-white/5 border border-white/10 text-stone-300 hover:text-amber-300 hover:border-amber-500/60 cursor-pointer transition-colors"
                  >
                    {entityId}
                  </button>
                ))}
              </div>
            )}
          </div>
        ))
      )}
    </div>
  );
};
