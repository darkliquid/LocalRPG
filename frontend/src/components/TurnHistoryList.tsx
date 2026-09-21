import React from 'react';

interface TurnHistoryListProps {
  turns?: number[];
  onTurnClick?: (turnNumber: number) => void;
}

export const TurnHistoryList: React.FC<TurnHistoryListProps> = ({ turns, onTurnClick }) => {
  if (!turns || turns.length === 0) {
    return null;
  }

  return (
    <div className="flex flex-wrap items-center gap-2 text-xs font-cinzel tracking-wider text-stone-400">
      <span className="uppercase">Appears in</span>
      {turns.map((turn) => (
        <button
          key={turn}
          onClick={() => onTurnClick?.(turn)}
          className="px-2 py-0.5 rounded border border-white/10 hover:border-amber-500/60 hover:text-amber-300 cursor-pointer transition-colors"
        >
          Turn {turn}
        </button>
      ))}
    </div>
  );
};
