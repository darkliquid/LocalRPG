import React from 'react';
import { GameState, Recap } from '../types';
import { Clock, BookOpen, ScrollText } from 'lucide-react';

// A clock with no maximum would divide by zero, so progress is clamped.
const percent = (value: number, max: number) => `${max > 0 ? Math.min(100, (value / max) * 100) : 0}%`;

interface LivingWorldDrawerProps {
  state?: GameState;
  recap?: Recap;
  idleTurns?: number;
  onRefreshRecap?: () => void;
}

export const LivingWorldDrawer: React.FC<LivingWorldDrawerProps> = ({
  state,
  recap,
  idleTurns = 10,
  onRefreshRecap,
}) => {
  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-cinzel text-amber-400 font-bold uppercase tracking-wider flex items-center gap-1.5">
            <ScrollText className="w-4 h-4" />
            <span>Story So Far</span>
          </h3>
          {recap?.enabled && onRefreshRecap && (
            <button
              onClick={onRefreshRecap}
              className="text-[11px] font-cinzel px-2 py-1 rounded-lg bg-stone-900 border border-stone-700 text-stone-300 hover:text-amber-300 cursor-pointer transition-colors"
            >
              Refresh
            </button>
          )}
        </div>
        {recap?.summary ? (
          <>
            <p className="text-xs text-stone-300 leading-relaxed whitespace-pre-wrap bg-black/30 p-3 rounded-xl border border-white/5">
              {recap.summary}
            </p>
            <p className="text-[11px] font-mono text-stone-500">Through turn {recap.through_turn}</p>
          </>
        ) : (!recap?.threads || recap.threads.length === 0) ? (
          <p className="text-stone-500 text-xs italic bg-black/30 p-3 rounded-xl border border-white/5">
            {recap?.enabled === false
              ? 'Summaries are switched off in Settings.'
              : 'The story has not turned far enough to be summarised yet.'}
          </p>
        ) : null}
      </div>

      {recap?.threads && recap.threads.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-cinzel text-amber-400 font-bold uppercase tracking-wider">Open Threads</h3>
          {recap.threads.map((thread) => {
            // A quiet thread is the one most likely to be forgotten, so it is the one
            // the player is nudged about. The narrator sees them all either way.
            const stale = thread.idle >= idleTurns;
            return (
              <div
                key={thread.id}
                className={`text-xs rounded-xl border p-2.5 space-y-1 ${
                  stale ? 'bg-amber-950/30 border-amber-500/40' : 'bg-black/30 border-white/5'
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="text-stone-200">{thread.name}</span>
                  <span className="font-mono text-[11px] text-stone-400">{thread.status}</span>
                </div>
                <div className="text-[11px] font-mono text-stone-500">
                  {thread.last_advanced > 0
                    ? `Last advanced at turn ${thread.last_advanced} (${thread.idle} turns ago)`
                    : 'Not advanced yet'}
                  {stale && ' - worth returning to'}
                </div>
              </div>
            );
          })}
        </div>
      )}

      <div className="space-y-3">
        <h3 className="text-sm font-cinzel text-amber-400 font-bold uppercase tracking-wider flex items-center gap-1.5">
          <BookOpen className="w-4 h-4" />
          <span>Active Narrative Arcs</span>
        </h3>
        {(!state?.arcs || state.arcs.length === 0) ? (
          <p className="text-stone-500 text-xs italic bg-black/30 p-3 rounded-xl border border-white/5">
            No active world narrative arcs registered.
          </p>
        ) : (
          state.arcs.map((arc) => (
            <div key={arc.id} className="bg-black/40 p-3 rounded-xl border border-white/5 space-y-1.5 shadow-inner">
              <div className="flex justify-between text-xs font-cinzel">
                <span className="text-stone-300">{arc.name}</span>
                <span className="text-amber-400 font-mono">{arc.progress}/{arc.max_progress}</span>
              </div>
              <div className="w-full h-1.5 bg-stone-900 rounded-full overflow-hidden">
                <div
                  className="h-full bg-amber-500"
                  style={{ width: percent(arc.progress, arc.max_progress) }}
                />
              </div>
            </div>
          ))
        )}
      </div>

      <div className="space-y-3">
        <h3 className="text-sm font-cinzel text-amber-400 font-bold uppercase tracking-wider flex items-center gap-1.5">
          <Clock className="w-4 h-4" />
          <span>Faction Clocks</span>
        </h3>
        {(!state?.clocks || state.clocks.length === 0) ? (
          <p className="text-stone-500 text-xs italic bg-black/30 p-3 rounded-xl border border-white/5">
            All factions are quiet in the shadows.
          </p>
        ) : (
          state.clocks.map((clock, i) => (
            <div key={i} className="bg-black/40 p-3 rounded-xl border border-white/5 space-y-1.5 shadow-inner">
              <div className="flex justify-between text-xs font-cinzel">
                <span className="text-stone-300">{clock.name} ({clock.faction})</span>
                <span className="text-red-400 font-mono">{clock.ticks}/{clock.max_ticks}</span>
              </div>
              <div className="w-full h-1.5 bg-stone-900 rounded-full overflow-hidden">
                <div
                  className="h-full bg-red-600"
                  style={{ width: percent(clock.ticks, clock.max_ticks) }}
                />
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
