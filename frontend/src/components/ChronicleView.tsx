import React from 'react';
import { Turn } from '../types';
import { TurnSegments } from './TurnSegments';

interface ChronicleViewProps {
  turns: Turn[];
  onWikilinkClick: (entityId: string) => void;
}

export const ChronicleView: React.FC<ChronicleViewProps> = ({ turns, onWikilinkClick }) => {
  return (
    <div className="flex-1 overflow-y-auto px-8 py-6 space-y-6">
      {turns.length === 0 ? (
        <div className="h-full flex items-center justify-center text-stone-500 font-cinzel tracking-wider text-sm italic">
          The chronicle awaits your first action...
        </div>
      ) : (
        turns.map((turn) => (
          <div key={turn.turn_number} className="space-y-4 pb-6 border-b border-white/5 last:border-0">
            {/* Player Input Block */}
            {turn.input_text && (
              <div className="flex items-start gap-3 text-stone-300 text-sm font-sans italic bg-black/40 p-3.5 rounded-xl border border-white/5 shadow-inner">
                <span className="text-amber-400 font-semibold uppercase tracking-wider text-xs font-cinzel">
                  [{turn.mode || 'Action'}]
                </span>
                <span>{turn.input_text}</span>
              </div>
            )}

            {/* Scene Illustration if available */}
            {turn.image_url && (
              <div className="my-4 rounded-xl overflow-hidden border border-white/10 shadow-2xl">
                <img src={turn.image_url} alt="Scene illustration" className="w-full object-cover max-h-96" />
              </div>
            )}

            {/* Narrated prose and attributed speech, in playback order */}
            <TurnSegments segments={turn.segments} fallback={turn.prose} onEntityClick={onWikilinkClick} />

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
