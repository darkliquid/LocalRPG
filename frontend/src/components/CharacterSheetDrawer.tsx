import React from 'react';
import { PlayerState } from '../types';
import { Heart } from 'lucide-react';

interface CharacterSheetDrawerProps {
  player?: PlayerState;
}

export const CharacterSheetDrawer: React.FC<CharacterSheetDrawerProps> = ({ player }) => {
  if (!player) return <div className="p-6 text-stone-500">No character loaded.</div>;

  const hp = (player.state?.hp as number) ?? 20;
  const maxHp = (player.state?.max_hp as number) ?? 20;
  const level = (player.state?.level as number) ?? 1;

  return (
    <div className="space-y-6">
      <div className="border-b border-white/10 pb-4">
        <h2 className="text-2xl font-cinzel text-amber-400 font-bold">{player.name}</h2>
        <span className="text-xs font-mono uppercase tracking-widest text-stone-400">Level {level} {player.type}</span>
      </div>

      {/* HP Bar */}
      <div className="space-y-1.5">
        <div className="flex justify-between text-xs font-cinzel text-stone-300">
          <span className="flex items-center gap-1"><Heart className="w-3.5 h-3.5 text-red-500" /> Health</span>
          <span>{hp} / {maxHp}</span>
        </div>
        <div className="w-full h-3 bg-stone-900 rounded-full overflow-hidden border border-white/5">
          <div
            className="h-full bg-red-600 transition-all duration-300"
            style={{ width: `${Math.min(100, (hp / maxHp) * 100)}%` }}
          />
        </div>
      </div>

      {/* Dynamic Attributes Grid */}
      <div className="grid grid-cols-2 gap-3">
        {Object.entries(player.state || {}).map(([key, val]) => {
          if (key === 'hp' || key === 'max_hp' || key === 'level') return null;
          return (
            <div key={key} className="bg-black/40 p-3 rounded-xl border border-white/5 shadow-inner">
              <span className="text-xs uppercase text-stone-400 tracking-wider block font-cinzel">{key}</span>
              <span className="text-lg font-bold text-amber-400 font-mono">{String(val)}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
};
