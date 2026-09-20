import React from 'react';
import { GameState } from '../types';
import { Clock, BookOpen } from 'lucide-react';

interface LivingWorldDrawerProps {
  state?: GameState;
}

export const LivingWorldDrawer: React.FC<LivingWorldDrawerProps> = ({ state }) => {
  return (
    <div className="space-y-6">
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
                  style={{ width: `${(arc.progress / arc.max_progress) * 100}%` }}
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
                  style={{ width: `${(clock.ticks / clock.max_ticks) * 100}%` }}
                />
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
