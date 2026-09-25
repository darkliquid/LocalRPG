import React from 'react';
import { WorldInfo } from '../../types';
import { ProceduralIcon } from './ProceduralAsset';
import { Plus, LayoutGrid } from 'lucide-react';

interface WorldFlyoutProps {
  isOpen: boolean;
  worlds: WorldInfo[];
  onSelectWorld: (worldId: string) => void;
  onCreateWorld: () => void;
  onExpand: () => void;
}

export const WorldFlyout: React.FC<WorldFlyoutProps> = ({
  isOpen,
  worlds,
  onSelectWorld,
  onCreateWorld,
  onExpand,
}) => {
  if (!isOpen) return null;

  return (
    <div className="absolute left-[84px] top-4 z-40 flex items-center gap-3 bg-stone-900/95 backdrop-blur-2xl border border-white/15 rounded-2xl p-2 px-3 shadow-2xl animate-in fade-in slide-in-from-left-4 duration-200 select-none">
      <div className="text-[11px] font-sans font-semibold uppercase tracking-wider text-stone-400 pl-1 pr-1 border-r border-white/10 shrink-0">
        Worlds
      </div>

      <div
        onWheel={(e) => {
          if (e.currentTarget.scrollWidth > e.currentTarget.clientWidth) {
            e.currentTarget.scrollLeft += e.deltaY;
          }
        }}
        className="flex items-center gap-2 max-w-[min(720px,calc(100vw-150px))] overflow-x-auto overflow-y-hidden no-scrollbar py-1 px-0.5"
      >
        {worlds.map((world) => (
          <div key={world.id} className="relative group shrink-0">
            <button
              onClick={() => onSelectWorld(world.id)}
              className="w-[42px] h-[42px] rounded-xl overflow-hidden border border-white/10 hover:border-purple-400/80 hover:scale-105 transition-all cursor-pointer shadow-md block"
              aria-label={world.name}
              title={`${world.name}${world.genre ? ` (${world.genre})` : ''}`}
            >
              {world.icon_url ? (
                <img src={world.icon_url} alt={world.name} className="w-full h-full object-cover" />
              ) : (
                <ProceduralIcon id={world.id} name={world.name} genre={world.genre} size={42} className="w-full h-full rounded-none" />
              )}
            </button>
          </div>
        ))}

        {/* Create World Tile */}
        <div className="relative group shrink-0">
          <button
            onClick={onCreateWorld}
            className="w-[42px] h-[42px] rounded-xl border border-dashed border-white/25 hover:border-white/50 bg-white/[0.03] hover:bg-white/[0.08] flex items-center justify-center text-stone-300 hover:text-white transition-all cursor-pointer"
            title="Create New World"
            aria-label="Create New World"
          >
            <Plus className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Expand to full gallery */}
      <div className="shrink-0 border-l border-white/10 pl-2">
        <button
          onClick={onExpand}
          className="w-[42px] h-[42px] rounded-xl border border-white/10 hover:border-purple-400/80 bg-white/[0.03] hover:bg-white/[0.08] flex items-center justify-center text-stone-300 hover:text-white transition-all cursor-pointer"
          title="Expand world gallery"
          aria-label="Expand world gallery"
        >
          <LayoutGrid className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
};
