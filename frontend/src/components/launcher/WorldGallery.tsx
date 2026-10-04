import React from 'react';
import { WorldInfo } from '../../types';
import { ProceduralBanner, ProceduralIcon, getGenreIcon } from './ProceduralAsset';
import { X, Plus, Globe, Maximize2 } from 'lucide-react';
import { useLightbox } from '../../hooks/useLightbox';
import { useMountTransition } from '../../hooks/useMountTransition';
import { ImageLightbox } from '../ImageLightbox';

interface WorldGalleryProps {
  isOpen: boolean;
  worlds: WorldInfo[];
  onSelectWorld: (worldId: string) => void;
  onCreateWorld: () => void;
  onClose: () => void;
}

const WorldCard: React.FC<{
  world: WorldInfo;
  onSelect: () => void;
}> = ({ world, onSelect }) => {
  const GenreIcon = getGenreIcon(world.genre) || getGenreIcon(world.name);
  const tags = world.tags ?? [];
  const { lightbox, isLightboxOpen, openLightbox, closeLightbox } = useLightbox();
  const artworkURL = world.banner_url || world.icon_url;

  return (
    <div className="group relative">
      <button
        onClick={onSelect}
        className="w-full flex flex-col text-left rounded-3xl overflow-hidden border border-white/10 hover:border-purple-400/70 bg-stone-900/60 hover:bg-stone-900 shadow-xl hover:shadow-2xl hover:-translate-y-0.5 transition-all duration-200 cursor-pointer"
        aria-label={`Create a campaign in ${world.name}`}
      >
        {/* Banner */}
        <div className="relative h-44 w-full overflow-hidden shrink-0">
          {world.banner_url ? (
            <img
              src={world.banner_url}
              alt={world.name}
              className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
            />
          ) : (
            <ProceduralBanner id={world.id} name={world.name} className="w-full h-full" />
          )}
          <div className="absolute inset-0 bg-gradient-to-t from-stone-900 via-stone-900/30 to-transparent pointer-events-none" />

          {/* Icon + Name overlay */}
          <div className="absolute bottom-0 left-0 right-0 p-4 flex items-center gap-3.5">
            <div className="w-16 h-16 rounded-2xl overflow-hidden border-2 border-white/25 shadow-2xl shrink-0 bg-stone-900">
              {world.icon_url ? (
                <img src={world.icon_url} alt={world.name} className="w-full h-full object-cover" />
              ) : (
                <ProceduralIcon id={world.id} name={world.name} genre={world.genre} size={64} className="w-full h-full rounded-none border-0" />
              )}
            </div>
            <div className="min-w-0">
              <h3 className="text-lg font-sans font-extrabold text-white tracking-tight truncate">
                {world.name}
              </h3>
              {world.genre && (
                <span className="inline-flex items-center gap-1 text-xs font-sans font-semibold uppercase tracking-wider text-purple-300">
                  {GenreIcon && <GenreIcon className="w-3 h-3" />}
                  {world.genre}
                </span>
              )}
            </div>
          </div>
        </div>

        {/* Body */}
        <div className="flex-1 flex flex-col gap-3 p-5">
          <p className="text-sm font-sans text-stone-300 leading-relaxed whitespace-pre-line">
            {world.description || 'Explore uncharted territory and shape the fate of this realm.'}
          </p>

          {tags.length > 0 && (
            <div className="flex flex-wrap gap-1.5 mt-auto pt-2">
              {tags.map((tag) => (
                <span
                  key={tag}
                  className="text-xs font-sans font-medium px-2 py-0.5 rounded-full bg-white/[0.06] border border-white/10 text-stone-300"
                >
                  {tag}
                </span>
              ))}
            </div>
          )}
        </div>
      </button>

      {artworkURL && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            openLightbox(artworkURL, world.name);
          }}
          className="absolute top-3 right-3 w-8 h-8 rounded-lg bg-black/60 hover:bg-black/80 border border-white/15 flex items-center justify-center text-white opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity cursor-zoom-in"
          title={`View full size: ${world.name}`}
          aria-label={`View full size: ${world.name}`}
        >
          <Maximize2 className="w-4 h-4" />
        </button>
      )}

      {lightbox && (
        <ImageLightbox isOpen={isLightboxOpen} src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}
    </div>
  );
};

export const WorldGallery: React.FC<WorldGalleryProps> = ({
  isOpen,
  worlds,
  onSelectWorld,
  onCreateWorld,
  onClose,
}) => {
  const { mounted, state } = useMountTransition(isOpen, 200);
  if (!mounted) return null;

  return (
    <div
      data-state={state}
      className={`fixed inset-0 z-50 flex flex-col bg-stone-950/98 backdrop-blur-xl ${
        state === 'enter' ? 'anim-fade-in' : 'anim-fade-out pointer-events-none'
      }`}
      style={{ '--anim-dur': '200ms' } as React.CSSProperties}
    >
      {/* Header */}
      <header className="h-16 px-8 border-b border-white/10 flex items-center justify-between bg-stone-900/70 shrink-0">
        <div className="flex items-center gap-3">
          <div className="w-9 h-9 rounded-xl bg-purple-600/20 border border-purple-500/30 flex items-center justify-center">
            <Globe className="w-5 h-5 text-purple-300" />
          </div>
          <div>
            <h2 className="text-base font-sans font-bold text-white tracking-tight">Explore Worlds</h2>
            <p className="text-xs font-sans text-stone-400">
              {worlds.length} {worlds.length === 1 ? 'world' : 'worlds'} available
            </p>
          </div>
        </div>
        <button
          onClick={onClose}
          className="w-9 h-9 rounded-full bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 flex items-center justify-center text-stone-400 hover:text-white transition-all cursor-pointer"
          title="Close gallery"
          aria-label="Close gallery"
        >
          <X className="w-4 h-4" />
        </button>
      </header>

      {/* Grid */}
      <div className="flex-1 overflow-y-auto p-8">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6 max-w-[1600px] mx-auto content-start">
          {worlds.map((world) => (
            <WorldCard key={world.id} world={world} onSelect={() => onSelectWorld(world.id)} />
          ))}

          {/* Create World Tile */}
          <button
            onClick={onCreateWorld}
            className="rounded-3xl border border-dashed border-white/20 hover:border-white/45 bg-white/[0.02] hover:bg-white/[0.05] flex flex-col items-center justify-center gap-3 text-stone-400 hover:text-white transition-all cursor-pointer min-h-[280px]"
            aria-label="Create New World"
          >
            <div className="w-14 h-14 rounded-2xl border border-dashed border-white/25 flex items-center justify-center">
              <Plus className="w-6 h-6" />
            </div>
            <span className="text-sm font-sans font-semibold tracking-wide">Create New World</span>
          </button>
        </div>
      </div>
    </div>
  );
};
