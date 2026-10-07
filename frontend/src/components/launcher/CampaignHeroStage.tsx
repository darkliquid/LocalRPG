import React from 'react';
import { GameSummary } from '../../types';
import { ProceduralBanner, ProceduralIcon } from './ProceduralAsset';
import { Play, Settings, Compass, Sparkles, Clock, Calendar, Globe, BookOpen, Cpu } from 'lucide-react';
import { useLightbox } from '../../hooks/useLightbox';
import { ImageLightbox } from '../ImageLightbox';

interface CampaignHeroStageProps {
  game: GameSummary | null;
  worldName?: string;
  systemName?: string;
  hasGames: boolean;
  hasWorlds: boolean;
  onPlay: (gameId: string) => void;
  onOpenCampaignSettings: (gameId: string) => void;
  onCreateWorld: () => void;
  onBrowseSystems: () => void;
  onOpenDocs?: (articleID?: string) => void;
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

function formatPlayTime(seconds?: number): string {
  if (!seconds || seconds <= 0) return '0 mins';
  const mins = Math.floor(seconds / 60);
  if (mins < 60) return `${mins} mins`;
  const hours = Math.floor(mins / 60);
  const remMins = mins % 60;
  return remMins > 0 ? `${hours}h ${remMins}m` : `${hours}h`;
}

export const CampaignHeroStage: React.FC<CampaignHeroStageProps> = ({
  game,
  worldName,
  systemName,
  hasGames,
  hasWorlds,
  onPlay,
  onOpenCampaignSettings,
  onCreateWorld,
  onBrowseSystems,
  onOpenDocs,
}) => {
  const iconURL = game?.icon_url;
  const { lightbox, isLightboxOpen, openLightbox, closeLightbox } = useLightbox();
  return (
    <main className="relative flex-1 h-full overflow-hidden select-none flex flex-col justify-between py-8 pr-8 pl-[104px] bg-stone-950">
      {/* Background Banner */}
      <div className="absolute inset-0 pointer-events-none overflow-hidden">
        {game?.banner_url ? (
          <img
            key={game.banner_url}
            src={game.banner_url}
            alt={game.name}
            className="w-full h-full object-cover anim-fade-in"
          />
        ) : game ? (
          <ProceduralBanner id={game.id} name={game.name} className="anim-fade-in" />
        ) : (
          <div className="w-full h-full bg-radial-[circle_at_center] from-purple-950/20 via-stone-950/80 to-stone-950" />
        )}

        {/* Cinematic Vignettes */}
        <div className="absolute inset-0 bg-gradient-to-t from-stone-950 via-stone-950/20 to-stone-950/70" />
        <div className="absolute inset-0 bg-radial-[circle_at_center] from-transparent via-transparent to-stone-950/80" />
      </div>

      {/* Top Row: Stats Badge */}
      <div className="relative z-10 flex justify-end anim-slide-in-up">
        {game && (
          <div
            key={game.id}
            className="flex items-center gap-6 bg-stone-900/80 backdrop-blur-xl border border-white/10 rounded-2xl px-5 py-2.5 shadow-2xl"
          >
            <div className="flex items-center gap-2.5">
              <Clock className="w-4 h-4 text-purple-400" />
              <div>
                <div className="text-xs font-sans font-bold uppercase tracking-wider text-stone-400">
                  Play Time
                </div>
                <div className="text-xs font-sans font-bold text-stone-100">
                  {formatPlayTime(game.play_time_seconds)}
                </div>
              </div>
            </div>

            <div className="w-[1px] h-6 bg-white/10" />

            <div className="flex items-center gap-2.5">
              <Compass className="w-4 h-4 text-purple-400" />
              <div>
                <div className="text-xs font-sans font-bold uppercase tracking-wider text-stone-400">
                  Turns
                </div>
                <div className="text-xs font-sans font-bold text-stone-100">
                  {game.turn_count} turns
                </div>
              </div>
            </div>

            <div className="w-[1px] h-6 bg-white/10" />

            <div className="flex items-center gap-2.5">
              <Calendar className="w-4 h-4 text-purple-400" />
              <div>
                <div className="text-xs font-sans font-bold uppercase tracking-wider text-stone-400">
                  Last Played
                </div>
                <div className="text-xs font-sans font-bold text-stone-100">
                  {formatRelativeTime(game.last_played)}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Center Onboarding Zero-State (If no game is selected or available) */}
      {!game && (
        <div className="relative z-10 flex-1 flex flex-col items-center justify-center text-center max-w-md mx-auto anim-slide-in-up">
          {!hasGames && hasWorlds && (
            <div className="bg-stone-900/80 backdrop-blur-xl border border-white/10 rounded-3xl p-8 shadow-2xl">
              <div className="w-12 h-12 rounded-2xl bg-purple-500/20 border border-purple-500/40 text-purple-400 flex items-center justify-center mx-auto mb-4">
                <Compass className="w-6 h-6" />
              </div>
              <h2 className="text-xl font-sans font-bold text-white mb-2">No Campaigns Yet</h2>
              <p className="text-sm font-sans text-stone-400 leading-relaxed">
                Click the <span className="text-purple-400 font-bold">+</span> button in the left dock to choose a world and start your first adventure.
              </p>
            </div>
          )}

          {!hasGames && !hasWorlds && (
            <div className="bg-stone-900/85 backdrop-blur-xl border border-white/12 rounded-3xl p-8 shadow-2xl">
              <div className="w-12 h-12 rounded-2xl bg-purple-500/20 border border-purple-500/40 text-purple-400 flex items-center justify-center mx-auto mb-4">
                <Sparkles className="w-6 h-6" />
              </div>
              <h2 className="text-xl font-sans font-bold text-white mb-2">Welcome to LocalRPG</h2>
              <p className="text-sm font-sans text-stone-400 leading-relaxed mb-6">
                Create your first world or explore available game systems to begin crafting your tabletop campaign.
              </p>
              <div className="flex flex-wrap items-center justify-center gap-3">
                <button
                  onClick={onCreateWorld}
                  className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-purple-600 hover:bg-purple-500 text-white font-sans font-semibold text-xs transition-all shadow-lg shadow-purple-600/30 cursor-pointer"
                >
                  <Globe className="w-4 h-4" />
                  <span>Create First World</span>
                </button>
                <button
                  onClick={onBrowseSystems}
                  className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-stone-800 hover:bg-stone-700 text-stone-200 font-sans font-semibold text-xs border border-white/10 transition-all cursor-pointer"
                >
                  <BookOpen className="w-4 h-4" />
                  <span>Browse Systems</span>
                </button>
                {onOpenDocs && (
                  <button
                    onClick={() => onOpenDocs('21-local-first')}
                    className="flex items-center gap-2 px-4 py-2.5 rounded-xl bg-stone-800 hover:bg-stone-700 text-emerald-400 font-sans font-semibold text-xs border border-emerald-500/30 transition-all cursor-pointer"
                  >
                    <Cpu className="w-4 h-4" />
                    <span>Local-First Guide</span>
                  </button>
                )}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Bottom Row: Title Card & Action Cluster */}
      {game && (
        <div key={game.id} className="relative z-10 flex items-end justify-between gap-6 anim-slide-in-up">
          {/* Bottom Left: Title Card */}
          <div className="flex items-center gap-4 bg-stone-900/85 backdrop-blur-2xl border border-white/15 rounded-2xl p-4 shadow-2xl max-w-xl">
            <div className="w-[52px] h-[52px] rounded-xl overflow-hidden flex-shrink-0 shadow-lg border border-white/10">
              {iconURL ? (
                <button
                  type="button"
                  onClick={() => openLightbox(iconURL, game.name)}
                  className="w-full h-full cursor-zoom-in"
                  title={`View full size: ${game.name}`}
                  aria-label={`View full size: ${game.name}`}
                >
                  <img src={iconURL} alt={game.name} className="w-full h-full object-cover" />
                </button>
              ) : (
                <ProceduralIcon id={game.id} name={game.name} size={52} className="w-full h-full rounded-none" />
              )}
            </div>
            <div className="min-w-0">
              <h1 className="text-xl font-sans font-extrabold text-white tracking-tight truncate">
                {game.name}
              </h1>
              <div className="text-xs font-sans text-stone-400 mt-0.5 truncate">
                <span>World: {worldName || game.world_id}</span>
                <span className="mx-1.5 opacity-40">•</span>
                <span>System: {systemName || game.system_id}</span>
                {game.player_name && (
                  <>
                    <span className="mx-1.5 opacity-40">•</span>
                    <span>Hero: {game.player_name}</span>
                  </>
                )}
              </div>
            </div>
          </div>

          {/* Bottom Right: Actions */}
          <div className="flex items-center gap-3">
            <button
              onClick={() => onOpenCampaignSettings(game.id)}
              className="w-[52px] h-[52px] rounded-2xl bg-stone-900/85 hover:bg-stone-800 border border-white/15 flex items-center justify-center text-stone-300 hover:text-white transition-all shadow-xl cursor-pointer"
              title="Campaign Settings"
              aria-label="Campaign Settings"
            >
              <Settings className="w-5 h-5" />
            </button>

            <button
              onClick={() => onPlay(game.id)}
              className="h-[52px] px-9 rounded-2xl bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-500 hover:to-indigo-500 text-white font-sans font-extrabold text-base tracking-wide flex items-center gap-2.5 shadow-xl shadow-purple-600/35 transition-all hover:scale-102 cursor-pointer border border-white/20"
              aria-label="Play Campaign"
            >
              <Play className="w-5 h-5 fill-current" />
              <span>PLAY</span>
            </button>
          </div>
        </div>
      )}

      {lightbox && (
        <ImageLightbox isOpen={isLightboxOpen} src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}
    </main>
  );
};
