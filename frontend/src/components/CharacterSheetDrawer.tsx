import React from 'react';
import { Advancement, PlayerState } from '../types';
import { Heart } from 'lucide-react';

interface CharacterSheetDrawerProps {
  player?: PlayerState;
  advancement?: Advancement;
  onSpend?: (unlockId: string) => void;
  spending?: boolean;
  spendError?: string | null;
}

// spendReason explains why an unlock cannot be bought right now, or null when it
// can. The server computes the flags; the drawer only phrases them.
const spendReason = (unlock: { affordable: boolean; requires_met: boolean; gate_open: boolean }): string | null => {
  if (!unlock.gate_open) return 'needs downtime';
  if (!unlock.requires_met) return 'requirements unmet';
  if (!unlock.affordable) return 'not affordable';
  return null;
};

export const CharacterSheetDrawer: React.FC<CharacterSheetDrawerProps> = ({
  player,
  advancement,
  onSpend,
  spending,
  spendError,
}) => {
  if (!player) return <div className="p-6 text-stone-500">No character loaded.</div>;

  const hp = (player.state?.hp as number) ?? 20;
  const maxHp = (player.state?.max_hp as number) ?? 20;
  const level = (player.state?.level as number) ?? 1;

  return (
    <div className="space-y-6">
      <div className="border-b border-white/10 pb-4">
        <h2 className="text-2xl font-sans text-purple-400 font-bold">{player.name}</h2>
        <span className="text-xs font-mono uppercase tracking-widest text-stone-400">Level {level} {player.type}</span>
      </div>

      {/* HP Bar */}
      <div className="space-y-1.5">
        <div className="flex justify-between text-xs font-sans text-stone-300">
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

      {/* Advancement, when the system declares a currency and unlocks. */}
      {advancement && (
        <div className="space-y-3 bg-black/40 p-4 rounded-xl border border-white/5 shadow-inner">
          <div className="flex items-baseline justify-between">
            <span className="text-xs uppercase text-stone-400 tracking-wider font-sans">
              {advancement.label || advancement.currency}
            </span>
            <span className="text-lg font-bold text-purple-400 font-mono">{advancement.value}</span>
          </div>

          {advancement.track && (
            <div className="space-y-1">
              <div className="w-full h-2 bg-stone-900 rounded-full overflow-hidden border border-white/5">
                <div
                  className="h-full bg-purple-500 transition-all duration-300"
                  style={{ width: `${Math.min(100, (advancement.track.filled / advancement.track.size) * 100)}%` }}
                />
              </div>
              <span className="text-[11px] font-mono text-stone-500">
                {advancement.track.filled} / {advancement.track.size}
              </span>
            </div>
          )}

          {advancement.unlocks && advancement.unlocks.length > 0 && (
            <ul className="space-y-2">
              {advancement.unlocks.map((unlock) => {
                const reason = spendReason(unlock);
                return (
                  <li key={unlock.id} className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <span className="block text-sm text-stone-200 truncate">{unlock.label || unlock.id}</span>
                      {unlock.description && (
                        <span className="block text-xs text-stone-500">{unlock.description}</span>
                      )}
                    </div>
                    <button
                      type="button"
                      disabled={!!reason || !!spending}
                      title={reason || undefined}
                      onClick={() => onSpend?.(unlock.id)}
                      className="shrink-0 text-xs font-sans px-3 py-1.5 rounded-xl border transition-all cursor-pointer bg-purple-600/80 hover:bg-purple-600 text-white border-white/10 disabled:opacity-40 disabled:cursor-not-allowed"
                    >
                      Spend {unlock.cost}
                      {reason && <span className="ml-1 text-[10px] text-stone-200/70">({reason})</span>}
                    </button>
                  </li>
                );
              })}
            </ul>
          )}

          {spendError && <p className="text-xs text-red-400">{spendError}</p>}
        </div>
      )}

      {/* Authored description, when the campaign was created with character creation. */}
      {(player.appearance || player.voice) && (
        <div className="space-y-3 bg-black/40 p-4 rounded-xl border border-white/5 shadow-inner">
          <span className="text-xs uppercase text-stone-400 tracking-wider block font-sans">Description</span>
          {player.appearance && <p className="text-sm text-stone-200 leading-relaxed">{player.appearance}</p>}
          {player.voice && (
            <p className="text-xs font-mono text-purple-300">
              Voice: {player.voice.name || player.voice.voice_id} ({player.voice.voice_id})
            </p>
          )}
        </div>
      )}

      {/* Dynamic Attributes Grid */}
      <div className="grid grid-cols-2 gap-3">
        {Object.entries(player.state || {}).map(([key, val]) => {
          if (key === 'hp' || key === 'max_hp' || key === 'level') return null;
          return (
            <div key={key} className="bg-black/40 p-3 rounded-xl border border-white/5 shadow-inner">
              <span className="text-xs uppercase text-stone-400 tracking-wider block font-sans">{key}</span>
              <span className="text-lg font-bold text-purple-400 font-mono">{String(val)}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
};
