import React, { useState, useEffect, useMemo } from 'react';
import { GameSummary, SystemInfo, WorldInfo } from '../../types';
import { ProceduralBanner, ProceduralIcon } from './ProceduralAsset';
import { Search, Plus, X, Settings, Play, LayoutGrid } from 'lucide-react';

export interface CampaignGalleryProps {
  isOpen: boolean;
  games: GameSummary[];
  worlds: WorldInfo[];
  systems: SystemInfo[];
  onPlayGame: (gameId: string) => void;
  onOpenSettings: (gameId: string) => void;
  onCreateCampaign: () => void;
  onClose: () => void;
}

function formatRelativeTime(dateStr?: string): string {
  if (!dateStr) return 'Never';
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return dateStr;
  const now = new Date();
  const diffSec = Math.floor((now.getTime() - date.getTime()) / 1000);
  if (diffSec < 60) return 'Just now';
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  if (diffSec < 604800) return `${Math.floor(diffSec / 86400)}d ago`;
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

type SortOption = 'recent' | 'name' | 'turns';

const CampaignCard: React.FC<{
  game: GameSummary;
  worldName?: string;
  systemName?: string;
  onPlay: () => void;
  onOpenSettings: () => void;
}> = ({ game, worldName, systemName, onPlay, onOpenSettings }) => {
  return (
    <div
      onClick={onPlay}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onPlay();
        }
      }}
      className="group relative flex flex-col text-left rounded-3xl overflow-hidden border border-white/10 hover:border-purple-400/70 bg-stone-900/60 hover:bg-stone-900 shadow-xl hover:shadow-2xl hover:-translate-y-0.5 transition-all duration-200 cursor-pointer"
      aria-label={`Play campaign ${game.name}`}
    >
      {/* Banner */}
      <div className="relative h-44 w-full overflow-hidden shrink-0">
        {game.banner_url ? (
          <img
            src={game.banner_url}
            alt={game.name}
            className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
          />
        ) : (
          <ProceduralBanner id={game.id} name={game.name} className="w-full h-full" />
        )}
        <div className="absolute inset-0 bg-gradient-to-t from-stone-900 via-stone-900/30 to-transparent pointer-events-none" />

        {/* Top Badges: Turn count & Settings button */}
        <div className="absolute top-3 right-3 flex items-center gap-2 z-10">
          <span className="px-2.5 py-1 rounded-full bg-black/60 backdrop-blur-md border border-white/15 text-[11px] font-sans font-semibold text-stone-200">
            Turn {game.turn_count}
          </span>
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onOpenSettings();
            }}
            className="w-7 h-7 rounded-full bg-black/60 hover:bg-black/90 backdrop-blur-md border border-white/15 hover:border-white/40 flex items-center justify-center text-stone-300 hover:text-white transition-all cursor-pointer"
            title="Campaign Settings"
            aria-label={`Settings for ${game.name}`}
          >
            <Settings className="w-3.5 h-3.5" />
          </button>
        </div>

        {/* Icon + Title Overlay */}
        <div className="absolute bottom-0 left-0 right-0 p-4 flex items-center gap-3.5">
          <div className="w-14 h-14 rounded-2xl overflow-hidden border-2 border-white/25 shadow-2xl shrink-0 bg-stone-900">
            {game.icon_url ? (
              <img src={game.icon_url} alt={game.name} className="w-full h-full object-cover" />
            ) : (
              <ProceduralIcon id={game.id} name={game.name} size={56} className="w-full h-full rounded-none border-0" />
            )}
          </div>
          <div className="min-w-0 flex-1">
            <h3 className="text-lg font-sans font-extrabold text-white tracking-tight truncate">
              {game.name}
            </h3>
            <div className="flex items-center gap-2 mt-0.5">
              {worldName && (
                <span className="text-[11px] font-sans font-semibold text-purple-300 truncate">
                  {worldName}
                </span>
              )}
              {worldName && systemName && <span className="text-stone-500 text-xs">•</span>}
              {systemName && (
                <span className="text-[11px] font-sans font-medium text-stone-400 truncate">
                  {systemName}
                </span>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Card Body & Footer */}
      <div className="flex-1 flex flex-col justify-between gap-3 p-4 bg-stone-950/40">
        <div className="flex items-center justify-between text-xs text-stone-400">
          <span>
            {game.player_name ? `Protagonist: ${game.player_name}` : 'Active Campaign'}
          </span>
          <span className="text-stone-500">{formatRelativeTime(game.last_played)}</span>
        </div>

        <div className="flex items-center justify-end pt-2 border-t border-white/5">
          <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-xl bg-purple-600/20 group-hover:bg-purple-600 text-purple-300 group-hover:text-white border border-purple-500/30 group-hover:border-purple-500 text-xs font-sans font-semibold transition-all">
            <Play className="w-3.5 h-3.5 fill-current" />
            <span>Play</span>
          </span>
        </div>
      </div>
    </div>
  );
};

export const CampaignGallery: React.FC<CampaignGalleryProps> = ({
  isOpen,
  games,
  worlds,
  systems,
  onPlayGame,
  onOpenSettings,
  onCreateCampaign,
  onClose,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [sortBy, setSortBy] = useState<SortOption>('recent');

  useEffect(() => {
    if (!isOpen) {
      setSearchQuery('');
      setSortBy('recent');
    }
  }, [isOpen]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  const worldMap = useMemo(() => {
    return new Map(worlds.map((w) => [w.id, w.name]));
  }, [worlds]);

  const systemMap = useMemo(() => {
    return new Map(systems.map((s) => [s.id, s.name]));
  }, [systems]);

  const filteredGames = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    let result = games.filter((g) => {
      if (!query) return true;
      const worldName = worldMap.get(g.world_id)?.toLowerCase() || '';
      const systemName = systemMap.get(g.system_id)?.toLowerCase() || '';
      const name = g.name.toLowerCase();
      const playerName = g.player_name?.toLowerCase() || '';
      return (
        name.includes(query) ||
        playerName.includes(query) ||
        worldName.includes(query) ||
        systemName.includes(query)
      );
    });

    result.sort((a, b) => {
      if (sortBy === 'recent') {
        const timeA = a.last_played ? new Date(a.last_played).getTime() : 0;
        const timeB = b.last_played ? new Date(b.last_played).getTime() : 0;
        return timeB - timeA;
      }
      if (sortBy === 'turns') {
        return b.turn_count - a.turn_count;
      }
      if (sortBy === 'name') {
        return a.name.localeCompare(b.name);
      }
      return 0;
    });

    return result;
  }, [games, searchQuery, sortBy, worldMap, systemMap]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-stone-950/98 backdrop-blur-xl select-none animate-in fade-in duration-200">
      {/* Header */}
      <header className="h-16 px-8 border-b border-white/10 flex items-center justify-between bg-stone-900/70 shrink-0">
        <div className="flex items-center gap-3">
          <div className="w-9 h-9 rounded-xl bg-purple-600/20 border border-purple-500/30 flex items-center justify-center">
            <LayoutGrid className="w-5 h-5 text-purple-400" />
          </div>
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-sans font-bold text-white tracking-wide">
              Campaigns
            </h2>
            <span className="px-2 py-0.5 rounded-full bg-white/[0.06] border border-white/10 text-xs font-mono text-stone-400">
              {games.length}
            </span>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={onCreateCampaign}
            className="flex items-center gap-2 px-4 py-2 rounded-xl bg-purple-600 hover:bg-purple-500 text-white text-xs font-sans font-semibold shadow-lg shadow-purple-600/20 transition-all cursor-pointer"
          >
            <Plus className="w-4 h-4" />
            <span>New Campaign</span>
          </button>
          <button
            onClick={onClose}
            className="w-9 h-9 rounded-xl bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 flex items-center justify-center text-stone-400 hover:text-white transition-all cursor-pointer"
            aria-label="Close Campaign Gallery"
          >
            <X className="w-5 h-5" />
          </button>
        </div>
      </header>

      {/* Search & Sort Bar */}
      <div className="px-8 py-4 border-b border-white/5 bg-stone-900/30 flex flex-wrap items-center justify-between gap-4 shrink-0">
        <div className="relative flex-1 max-w-md">
          <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-stone-500 pointer-events-none" />
          <input
            type="text"
            placeholder="Search campaigns, worlds, systems..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-10 pr-4 py-2 rounded-xl bg-white/[0.05] border border-white/10 focus:border-purple-500/60 focus:bg-white/[0.08] text-sm text-white placeholder-stone-500 outline-none transition-all font-sans"
          />
          {searchQuery && (
            <button
              onClick={() => setSearchQuery('')}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-stone-400 hover:text-white"
            >
              <X className="w-4 h-4" />
            </button>
          )}
        </div>

        <div className="flex items-center gap-2">
          <span className="text-xs text-stone-400 font-sans">Sort by:</span>
          <select
            value={sortBy}
            onChange={(e) => setSortBy(e.target.value as SortOption)}
            className="bg-stone-900 border border-white/15 rounded-xl px-3 py-1.5 text-xs text-stone-200 outline-none focus:border-purple-500 cursor-pointer"
          >
            <option value="recent">Recently Played</option>
            <option value="turns">Turn Count</option>
            <option value="name">Alphabetical</option>
          </select>
        </div>
      </div>

      {/* Grid Content */}
      <div className="flex-1 overflow-y-auto p-8">
        {filteredGames.length > 0 ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6 max-w-7xl mx-auto">
            {filteredGames.map((game) => (
              <CampaignCard
                key={game.id}
                game={game}
                worldName={worldMap.get(game.world_id)}
                systemName={systemMap.get(game.system_id)}
                onPlay={() => onPlayGame(game.id)}
                onOpenSettings={() => onOpenSettings(game.id)}
              />
            ))}

            {/* Create Campaign Card */}
            <button
              onClick={onCreateCampaign}
              className="group flex flex-col items-center justify-center min-h-[260px] rounded-3xl border border-dashed border-white/20 hover:border-purple-400/60 bg-white/[0.02] hover:bg-white/[0.05] p-6 text-center transition-all cursor-pointer"
            >
              <div className="w-12 h-12 rounded-2xl bg-white/[0.05] group-hover:bg-purple-600/20 border border-white/10 group-hover:border-purple-500/40 flex items-center justify-center text-stone-400 group-hover:text-purple-300 mb-3 transition-all">
                <Plus className="w-6 h-6" />
              </div>
              <h3 className="text-sm font-sans font-bold text-stone-200 group-hover:text-white">
                New Campaign
              </h3>
              <p className="text-xs font-sans text-stone-500 mt-1 max-w-[200px]">
                Create a campaign from any available world
              </p>
            </button>
          </div>
        ) : (
          <div className="h-full flex flex-col items-center justify-center text-center p-8 max-w-md mx-auto">
            <div className="w-16 h-16 rounded-2xl bg-white/[0.03] border border-white/10 flex items-center justify-center text-stone-500 mb-4">
              <LayoutGrid className="w-8 h-8" />
            </div>
            {searchQuery ? (
              <>
                <h3 className="text-base font-sans font-bold text-white">No campaigns found</h3>
                <p className="text-xs text-stone-400 mt-1 mb-4">
                  No campaigns match your search for "{searchQuery}".
                </p>
                <button
                  onClick={() => setSearchQuery('')}
                  className="px-4 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans text-stone-300 hover:text-white transition-all cursor-pointer"
                >
                  Clear Search
                </button>
              </>
            ) : (
              <>
                <h3 className="text-base font-sans font-bold text-white">No campaigns yet</h3>
                <p className="text-xs text-stone-400 mt-1 mb-4">
                  Create your first campaign to embark on an adventure.
                </p>
                <button
                  onClick={onCreateCampaign}
                  className="flex items-center gap-2 px-4 py-2 rounded-xl bg-purple-600 hover:bg-purple-500 text-white text-xs font-sans font-semibold transition-all cursor-pointer"
                >
                  <Plus className="w-4 h-4" />
                  <span>Create Campaign</span>
                </button>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
};
