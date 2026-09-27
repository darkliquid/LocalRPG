import React from 'react';
import { GameSummary } from '../../types';
import { ProceduralIcon } from './ProceduralAsset';
import { Plus, Globe, BookOpen, Settings, LayoutGrid, HelpCircle } from 'lucide-react';

interface LauncherDockProps {
  games: GameSummary[];
  activeGameID: string | null;
  onSelectGame: (gameId: string) => void;
  isFlyoutOpen: boolean;
  onToggleFlyout: () => void;
  isCampaignGalleryOpen: boolean;
  onToggleCampaignGallery: () => void;
  onOpenWorldsStudio: () => void;
  onOpenSystemsStudio: () => void;
  onOpenSettings: () => void;
  onOpenDocs?: () => void;
}

export const LauncherDock: React.FC<LauncherDockProps> = ({
  games,
  activeGameID,
  onSelectGame,
  isFlyoutOpen,
  onToggleFlyout,
  isCampaignGalleryOpen,
  onToggleCampaignGallery,
  onOpenWorldsStudio,
  onOpenSystemsStudio,
  onOpenSettings,
  onOpenDocs,
}) => {
  const [hoveredGame, setHoveredGame] = React.useState<{ game: GameSummary; top: number } | null>(null);

  return (
    <aside className="absolute left-0 top-0 w-[72px] h-full flex flex-col items-center py-4 bg-stone-900/70 backdrop-blur-2xl border-r border-white/10 z-30 select-none anim-fade-in">
      {/* Add Campaign Button (+) */}
      <div className="relative group mb-3">
        <button
          onClick={onToggleFlyout}
          className={`w-[46px] h-[46px] rounded-2xl flex items-center justify-center transition-all duration-200 cursor-pointer ${
            isFlyoutOpen
              ? 'bg-purple-600/30 border-2 border-purple-500 text-purple-300 shadow-[0_0_15px_rgba(168,85,247,0.4)] rotate-45'
              : 'bg-white/[0.04] border border-dashed border-white/25 text-stone-300 hover:text-white hover:border-white/50 hover:bg-white/[0.08]'
          }`}
          title={isFlyoutOpen ? undefined : "New Campaign (Explore Worlds)"}
          aria-label="New Campaign"
        >
          <Plus className="w-5 h-5 transition-transform duration-200" />
        </button>
        {/* Tooltip */}
        {!isFlyoutOpen && (
          <div className="pointer-events-none absolute left-[64px] top-1/2 -translate-y-1/2 px-2.5 py-1 bg-stone-900 border border-white/15 rounded-lg text-xs font-sans text-stone-200 whitespace-nowrap shadow-xl opacity-0 group-hover:opacity-100 transition-opacity z-50">
            New Campaign
          </div>
        )}
      </div>

      <div className="w-8 h-[1px] bg-white/10 mb-3" />

      {/* Campaign Gallery Grid Button */}
      <div className="relative group mb-3">
        <button
          onClick={onToggleCampaignGallery}
          className={`w-[46px] h-[46px] rounded-2xl flex items-center justify-center transition-all duration-200 cursor-pointer ${
            isCampaignGalleryOpen
              ? 'bg-purple-600/30 border-2 border-purple-500 text-purple-300 shadow-[0_0_15px_rgba(168,85,247,0.4)]'
              : 'bg-white/[0.04] border border-white/15 text-stone-300 hover:text-white hover:border-purple-400/60 hover:bg-white/[0.08]'
          }`}
          title="Browse Campaigns (Grid View)"
          aria-label="Browse Campaigns (Grid View)"
        >
          <LayoutGrid className="w-5 h-5" />
        </button>
        {/* Tooltip */}
        {!isCampaignGalleryOpen && (
          <div className="pointer-events-none absolute left-[64px] top-1/2 -translate-y-1/2 px-2.5 py-1 bg-stone-900 border border-white/15 rounded-lg text-xs font-sans text-stone-200 whitespace-nowrap shadow-xl opacity-0 group-hover:opacity-100 transition-opacity z-50">
            Browse Campaigns
          </div>
        )}
      </div>

      {/* Campaigns List (Scrollable) */}
      <div
        className="flex-1 w-full overflow-y-auto overflow-x-hidden flex flex-col items-center gap-3 no-scrollbar py-1 anim-stagger"
        onScroll={() => setHoveredGame(null)}
      >
        {games.map((game) => {
          const isActive = game.id === activeGameID;
          return (
            <div key={game.id} className="relative group">
              {/* Active Indicator Bar */}
              {isActive && (
                <div className="absolute -left-[13px] top-1/2 -translate-y-1/2 w-1 h-6 bg-purple-500 rounded-r shadow-[0_0_10px_#a855f7]" />
              )}

              <button
                onClick={() => {
                  setHoveredGame(null);
                  onSelectGame(game.id);
                }}
                onMouseEnter={(e) => {
                  const rect = e.currentTarget.getBoundingClientRect();
                  setHoveredGame({ game, top: rect.top + rect.height / 2 });
                }}
                onMouseLeave={() => setHoveredGame(null)}
                className={`relative w-[46px] h-[46px] rounded-2xl overflow-hidden transition-all duration-200 cursor-pointer ${
                  isActive
                    ? 'ring-2 ring-purple-500 ring-offset-2 ring-offset-stone-950 shadow-[0_0_16px_rgba(168,85,247,0.45)] scale-105'
                    : 'opacity-70 hover:opacity-100 hover:scale-102 border border-white/10'
                }`}
                aria-label={game.name}
              >
                {game.icon_url ? (
                  <img src={game.icon_url} alt={game.name} className="w-full h-full object-cover" />
                ) : (
                  <ProceduralIcon id={game.id} name={game.name} size={46} className="w-full h-full rounded-none" />
                )}
              </button>
            </div>
          );
        })}
      </div>

      {/* Bottom Utility Icons */}
      <div className="flex flex-col items-center gap-2.5 pt-3 border-t border-white/10 anim-stagger">
        <div className="relative group">
          <button
            onClick={onOpenWorldsStudio}
            className="w-10 h-10 rounded-xl flex items-center justify-center text-stone-400 hover:text-white hover:bg-white/[0.08] transition-all cursor-pointer"
            title="Worlds Studio"
            aria-label="Worlds Studio"
          >
            <Globe className="w-5 h-5" />
          </button>
          <div className="pointer-events-none absolute left-[64px] top-1/2 -translate-y-1/2 px-2.5 py-1 bg-stone-900 border border-white/15 rounded-lg text-xs font-sans text-stone-200 whitespace-nowrap shadow-xl opacity-0 group-hover:opacity-100 transition-opacity z-50">
            Worlds Studio
          </div>
        </div>

        <div className="relative group">
          <button
            onClick={onOpenSystemsStudio}
            className="w-10 h-10 rounded-xl flex items-center justify-center text-stone-400 hover:text-white hover:bg-white/[0.08] transition-all cursor-pointer"
            title="Systems Studio"
            aria-label="Systems Studio"
          >
            <BookOpen className="w-5 h-5" />
          </button>
          <div className="pointer-events-none absolute left-[64px] top-1/2 -translate-y-1/2 px-2.5 py-1 bg-stone-900 border border-white/15 rounded-lg text-xs font-sans text-stone-200 whitespace-nowrap shadow-xl opacity-0 group-hover:opacity-100 transition-opacity z-50">
            Systems Studio
          </div>
        </div>

        <div className="relative group">
          <button
            onClick={onOpenSettings}
            className="w-10 h-10 rounded-xl flex items-center justify-center text-stone-400 hover:text-white hover:bg-white/[0.08] transition-all cursor-pointer"
            title="Settings"
            aria-label="Settings"
          >
            <Settings className="w-5 h-5" />
          </button>
          <div className="pointer-events-none absolute left-[64px] top-1/2 -translate-y-1/2 px-2.5 py-1 bg-stone-900 border border-white/15 rounded-lg text-xs font-sans text-stone-200 whitespace-nowrap shadow-xl opacity-0 group-hover:opacity-100 transition-opacity z-50">
            Settings
          </div>
        </div>

        <div className="relative group">
          <button
            onClick={onOpenDocs}
            className="w-10 h-10 rounded-xl flex items-center justify-center text-stone-400 hover:text-white hover:bg-white/[0.08] transition-all cursor-pointer"
            title="Help & Documentation"
            aria-label="Help & Documentation"
          >
            <HelpCircle className="w-5 h-5" />
          </button>
          <div className="pointer-events-none absolute left-[64px] top-1/2 -translate-y-1/2 px-2.5 py-1 bg-stone-900 border border-white/15 rounded-lg text-xs font-sans text-stone-200 whitespace-nowrap shadow-xl opacity-0 group-hover:opacity-100 transition-opacity z-50">
            Help & Documentation
          </div>
        </div>
      </div>

      {/* Floating Campaign Tooltip */}
      {!isFlyoutOpen && hoveredGame && (
        <div
          style={{ top: `${hoveredGame.top}px` }}
          className="pointer-events-none fixed left-[76px] -translate-y-1/2 px-2.5 py-1.5 bg-stone-900/95 border border-white/15 rounded-lg text-xs font-sans text-stone-100 whitespace-nowrap shadow-2xl z-50 anim-fade-in"
        >
          <div className="font-semibold text-white">{hoveredGame.game.name}</div>
          <div className="text-xs text-stone-400 mt-0.5">
            {hoveredGame.game.turn_count} {hoveredGame.game.turn_count === 1 ? 'turn' : 'turns'}
          </div>
        </div>
      )}
    </aside>
  );
};
