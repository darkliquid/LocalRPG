import React from 'react';
import { Turn } from '../types';
import { Volume2 } from 'lucide-react';

interface ChronicleViewProps {
  turns: Turn[];
  onWikilinkClick: (entityId: string) => void;
}

export const ChronicleView: React.FC<ChronicleViewProps> = ({ turns, onWikilinkClick }) => {
  const renderFormattedText = (text: string) => {
    // Replace [[wikilinks]] with clickable buttons
    const parts = text.split(/(\[\[[^\]]+\]\])/g);
    return parts.map((part, i) => {
      if (part.startsWith('[[') && part.endsWith(']]')) {
        const link = part.slice(2, -2);
        return (
          <button
            key={i}
            onClick={() => onWikilinkClick(link)}
            className="text-amber-400 hover:text-amber-300 underline font-medium cursor-pointer mx-1 transition-colors"
          >
            {link}
          </button>
        );
      }
      return <span key={i}>{part}</span>;
    });
  };

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

            {/* Dialogue with Speaker Bubble */}
            {turn.dialogue && (
              <div className="bg-glass-card border-l-4 border-amber-500/90 pl-4 py-3 pr-4 rounded-r-xl shadow-lg my-3 space-y-2">
                <div className="flex items-center justify-between text-xs text-amber-400 font-cinzel font-bold tracking-widest">
                  <span>{turn.speaker || 'UNKNOWN'}</span>
                  {turn.audio_url && (
                    <button
                      onClick={() => new Audio(turn.audio_url).play()}
                      className="flex items-center gap-1.5 hover:text-amber-300 cursor-pointer text-stone-400 transition-colors"
                      title="Play voice clip"
                    >
                      <Volume2 className="w-3.5 h-3.5" />
                      <span>Play</span>
                    </button>
                  )}
                </div>
                <p className="text-stone-100 text-lg leading-relaxed italic">
                  "{renderFormattedText(turn.dialogue)}"
                </p>
              </div>
            )}

            {/* Narrator Prose */}
            {turn.prose && (
              <div className="text-stone-200 text-xl leading-relaxed tracking-wide font-serif">
                {renderFormattedText(turn.prose)}
              </div>
            )}
          </div>
        ))
      )}
    </div>
  );
};
