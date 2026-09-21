import React from 'react';
import { TurnSegment } from '../types';

interface TurnSegmentsProps {
  segments?: TurnSegment[];
  fallback: string;
  onEntityClick?: (name: string) => void;
}

const renderWithLinks = (text: string, onEntityClick?: (name: string) => void) =>
  text.split(/(\[\[[^\]]+\]\])/g).map((part, i) => {
    if (part.startsWith('[[') && part.endsWith(']]')) {
      const link = part.slice(2, -2);
      return (
        <button
          key={i}
          onClick={() => onEntityClick?.(link)}
          className="text-amber-400 hover:text-amber-300 underline font-medium cursor-pointer mx-1 transition-colors"
        >
          {link}
        </button>
      );
    }
    return <span key={i}>{part}</span>;
  });

export const TurnSegments: React.FC<TurnSegmentsProps> = ({ segments, fallback, onEntityClick }) => {
  const ordered = segments && segments.length > 0 ? segments : [{ kind: 'narration' as const, text: fallback }];

  return (
    <div className="space-y-3">
      {ordered.map((segment, i) =>
        segment.kind === 'speech' ? (
          <div
            key={i}
            className="bg-glass-card border-l-4 border-amber-500/90 pl-4 py-3 pr-4 rounded-r-xl shadow-lg space-y-2"
          >
            <div className="text-xs text-amber-400 font-cinzel font-bold tracking-widest">
              {segment.speaker || 'UNKNOWN'}
            </div>
            <p className="text-stone-100 text-lg leading-relaxed italic">
              &ldquo;{renderWithLinks(segment.text, onEntityClick)}&rdquo;
            </p>
          </div>
        ) : (
          <div key={i} className="text-stone-200 text-xl leading-relaxed tracking-wide font-serif">
            {renderWithLinks(segment.text, onEntityClick)}
          </div>
        )
      )}
    </div>
  );
};
