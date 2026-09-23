import React from 'react';
import { WorldInfo } from '../../types';
import { ProceduralIcon } from './ProceduralAsset';
import { Plus } from 'lucide-react';

interface WorldFlyoutProps {
  isOpen: boolean;
  worlds: WorldInfo[];
  onSelectWorld: (worldId: string) => void;
  onCreateWorld: () => void;
}

export const WorldFlyout: React.FC<WorldFlyoutProps> = ({
  isOpen,
  worlds,
  onSelectWorld,
  onCreateWorld,
}) => {
  if (!isOpen) return null;

  return (
    <div className="absolute left-[84px] top-4 z-40 flex items-center gap-3 bg-stone-900/95 backdrop-blur-2xl border border-white/15 rounded-2xl p-2 px-3 shadow-2xl animate-in fade-in slide-in-from-left-4 duration-200 select-none">
      <div className="text-[11px] font-sans font-semibold uppercase tracking-wider text-stone-400 pl-1 pr-1 border-r border-white/10">
        Worlds
      </div>

      <div className="flex items-center gap-2 max-w-[calc(100vw-160px)] overflow-x-auto no-scrollbar py-0.5">
        {worlds.map((world) => (
          <div key={world.id} className="relative group">
            <button
              onClick={() => onSelectWorld(world.id)}
              className="w-[42px] h-[42px] rounded-xl overflow-hidden border border-white/10 hover:border-purple-400/80 hover:scale-105 transition-all cursor-pointer shadow-md"
              aria-label={world.name}
            >
              {world.icon_url ? (
                <img src={world.icon_url} alt={world.name} className="w-full h-full object-cover" />
              ) : (
                <ProceduralIcon id={world.id} name={world.name} genre={world.genre} size={42} className="w-full h-full rounded-none" />
              )}
            </button>

            {/* Tooltip */}
            <div className="pointer-events-none absolute left-1/2 -translate-x-1/2 top-full mt-2 px-2.5 py-1 bg-stone-900/95 border border-white/15 rounded-lg text-xs font-sans text-stone-100 whitespace-nowrap shadow-2xl opacity-0 group-hover:opacity-100 transition-opacity z-50">
              <span className="font-semibold text-white">{world.name}</span>
              {world.genre && <span className="text-stone-400 ml-1.5">({world.genre})</span>}
            </div>
          </div>
        ))}

        {/* Create World Tile */}
        <div className="relative group">
          <button
            onClick={onCreateWorld}
            className="w-[42px] h-[42px] rounded-xl border border-dashed border-white/25 hover:border-white/50 bg-white/[0.03] hover:bg-white/[0.08] flex items-center justify-center text-stone-300 hover:text-white transition-all cursor-pointer"
            title="Create New World"
            aria-label="Create New World"
          >
            <Plus className="w-4 h-4" />
          </button>
          <div className="pointer-events-none absolute left-1/2 -translate-x-1/2 top-full mt-2 px-2.5 py-1 bg-stone-900 border border-white/15 rounded-lg text-xs font-sans text-stone-200 whitespace-nowrap shadow-xl opacity-0 group-hover:opacity-100 transition-opacity z-50">
            Create World
          </div>
        </div>
      </div>
    </div>
  );
};
