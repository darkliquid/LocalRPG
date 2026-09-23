import React, { useState, useEffect, useRef } from 'react';
import { WorldInfo, SystemInfo, CreateGameRequest } from '../../types';
import { ProceduralBanner, ProceduralIcon } from './ProceduralAsset';
import { X, Upload, Check } from 'lucide-react';

interface NewCampaignModalProps {
  isOpen: boolean;
  world: WorldInfo | null;
  systems: SystemInfo[];
  onClose: () => void;
  onCreateGame: (data: CreateGameRequest, bannerFile?: File, iconFile?: File) => Promise<void>;
  isSubmitting: boolean;
}

export const NewCampaignModal: React.FC<NewCampaignModalProps> = ({
  isOpen,
  world,
  systems,
  onClose,
  onCreateGame,
  isSubmitting,
}) => {
  const [campaignName, setCampaignName] = useState('');
  const [selectedSystemID, setSelectedSystemID] = useState('');
  const [playerName, setPlayerName] = useState('');
  const [openingPrompt, setOpeningPrompt] = useState('');
  const [bannerFile, setBannerFile] = useState<File | null>(null);
  const [iconFile, setIconFile] = useState<File | null>(null);
  const [bannerPreview, setBannerPreview] = useState<string | null>(null);
  const [iconPreview, setIconPreview] = useState<string | null>(null);

  const bannerInputRef = useRef<HTMLInputElement>(null);
  const iconInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (world) {
      setCampaignName(`Chronicles of ${world.name}`);
      const compatible = systems.find((s) => world.compatible_systems?.includes(s.id));
      setSelectedSystemID(compatible ? compatible.id : systems[0]?.id || '');
      setBannerFile(null);
      setIconFile(null);
      setBannerPreview(null);
      setIconPreview(null);
    }
  }, [world, systems]);

  if (!isOpen || !world) return null;

  const handleBannerSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setBannerFile(file);
      setBannerPreview(URL.createObjectURL(file));
    }
  };

  const handleIconSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setIconFile(file);
      setIconPreview(URL.createObjectURL(file));
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!campaignName.trim() || !selectedSystemID || !playerName.trim()) return;

    onCreateGame(
      {
        name: campaignName.trim(),
        system_id: selectedSystemID,
        world_id: world.id,
        player_name: playerName.trim(),
        opening_prompt: openingPrompt.trim() || undefined,
      },
      bannerFile || undefined,
      iconFile || undefined
    );
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-md select-none animate-in fade-in duration-200">
      <div className="relative w-full max-w-2xl bg-stone-900/95 border border-white/15 rounded-3xl overflow-hidden shadow-2xl flex flex-col max-h-[90vh]">
        {/* Modal Banner Header */}
        <div className="relative h-44 w-full overflow-hidden flex items-end p-6 border-b border-white/10">
          {world.banner_url ? (
            <img src={world.banner_url} alt={world.name} className="absolute inset-0 w-full h-full object-cover" />
          ) : (
            <ProceduralBanner id={world.id} name={world.name} className="absolute inset-0" />
          )}

          <div className="absolute inset-0 bg-gradient-to-t from-stone-950 via-stone-950/40 to-transparent" />

          {/* Close button */}
          <button
            onClick={onClose}
            className="absolute top-4 right-4 w-8 h-8 rounded-full bg-black/50 border border-white/20 flex items-center justify-center text-white/80 hover:text-white hover:bg-black/70 transition-all cursor-pointer z-10"
          >
            <X className="w-4 h-4" />
          </button>

          {/* World Info Row */}
          <div className="relative z-10 flex items-center gap-4">
            <div className="w-16 h-16 rounded-2xl overflow-hidden shadow-2xl border-2 border-white/20 flex-shrink-0">
              {world.icon_url ? (
                <img src={world.icon_url} alt={world.name} className="w-full h-full object-cover" />
              ) : (
                <ProceduralIcon id={world.id} name={world.name} genre={world.genre} size={64} className="w-full h-full rounded-none" />
              )}
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-xl font-sans font-extrabold text-white tracking-tight">
                  New Campaign
                </h2>
                <span className="text-[11px] font-sans font-semibold bg-purple-500/20 text-purple-300 border border-purple-500/30 px-2 py-0.5 rounded-full">
                  {world.name}
                </span>
              </div>
              <p className="text-xs font-sans text-stone-300 mt-1 line-clamp-2 max-w-lg">
                {world.description || 'Explore uncharted territory and shape the fate of this realm.'}
              </p>
            </div>
          </div>
        </div>

        {/* Modal Form */}
        <form onSubmit={handleSubmit} className="flex-1 overflow-y-auto p-6 space-y-5">
          {/* Campaign Name */}
          <div className="space-y-1.5">
            <label className="text-xs font-sans font-semibold text-stone-300 uppercase tracking-wider">
              Campaign Title
            </label>
            <input
              type="text"
              required
              value={campaignName}
              onChange={(e) => setCampaignName(e.target.value)}
              placeholder="e.g. Chronicles of Eldoria"
              className="w-full bg-stone-950 border border-white/10 rounded-xl px-3.5 py-2.5 text-sm font-sans text-white focus:outline-none focus:border-purple-500 transition-colors"
            />
          </div>

          {/* Rules System Picker */}
          <div className="space-y-1.5">
            <div className="flex justify-between items-center">
              <label className="text-xs font-sans font-semibold text-stone-300 uppercase tracking-wider">
                Rules System
              </label>
              <span className="text-[11px] font-sans text-purple-400">
                {world.compatible_systems?.length ? 'Compatible systems highlighted' : 'All systems'}
              </span>
            </div>
            <div className="grid grid-cols-2 gap-2 max-h-36 overflow-y-auto no-scrollbar">
              {systems.map((s) => {
                const isSelected = s.id === selectedSystemID;
                const isCompatible = world.compatible_systems?.includes(s.id);
                return (
                  <button
                    key={s.id}
                    type="button"
                    onClick={() => setSelectedSystemID(s.id)}
                    className={`text-left p-3 rounded-xl border transition-all cursor-pointer ${
                      isSelected
                        ? 'bg-purple-600/20 border-purple-500 text-white shadow-[0_0_12px_rgba(168,85,247,0.3)]'
                        : isCompatible
                        ? 'bg-stone-950 border-white/15 text-stone-200 hover:border-white/30'
                        : 'bg-stone-950/50 border-white/5 text-stone-400 hover:border-white/20'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <div className="text-xs font-sans font-bold text-white">{s.name}</div>
                      {isSelected && <Check className="w-3.5 h-3.5 text-purple-400" />}
                    </div>
                    <div className="text-[11px] font-sans text-stone-400 mt-0.5 line-clamp-1">
                      {s.description || 'Rules system'}
                    </div>
                  </button>
                );
              })}
            </div>
          </div>

          {/* Protagonist Name */}
          <div className="space-y-1.5">
            <label className="text-xs font-sans font-semibold text-stone-300 uppercase tracking-wider">
              Protagonist Name
            </label>
            <input
              type="text"
              required
              value={playerName}
              onChange={(e) => setPlayerName(e.target.value)}
              placeholder="e.g. Valen Duskwarden"
              className="w-full bg-stone-950 border border-white/10 rounded-xl px-3.5 py-2.5 text-sm font-sans text-white focus:outline-none focus:border-purple-500 transition-colors"
            />
          </div>

          {/* Opening Prompt Directive */}
          <div className="space-y-1.5">
            <label className="text-xs font-sans font-semibold text-stone-300 uppercase tracking-wider">
              Opening Scene Prompt (Optional)
            </label>
            <textarea
              rows={2}
              value={openingPrompt}
              onChange={(e) => setOpeningPrompt(e.target.value)}
              placeholder="Custom starting scenario (e.g. You awaken in the hold of a smuggler's ship during a tempest...)"
              className="w-full bg-stone-950 border border-white/10 rounded-xl px-3.5 py-2.5 text-xs font-sans text-stone-200 focus:outline-none focus:border-purple-500 transition-colors resize-none"
            />
          </div>

          {/* Custom Artwork (Banner & Icon Uploads) */}
          <div className="p-3.5 bg-stone-950 border border-white/10 rounded-2xl space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <div className="text-xs font-sans font-bold text-white">Custom Campaign Artwork</div>
                <div className="text-[11px] font-sans text-stone-400">
                  Optional. A procedural theme is automatically generated if omitted.
                </div>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3 pt-1">
              {/* Banner Upload */}
              <div>
                <input
                  type="file"
                  ref={bannerInputRef}
                  onChange={handleBannerSelect}
                  accept="image/png,image/jpeg,image/webp,image/svg+xml"
                  className="hidden"
                />
                <button
                  type="button"
                  onClick={() => bannerInputRef.current?.click()}
                  className="w-full h-16 rounded-xl border border-dashed border-white/20 hover:border-purple-400/60 bg-white/[0.02] flex items-center justify-center gap-2 text-xs font-sans text-stone-300 hover:text-white transition-all cursor-pointer overflow-hidden"
                >
                  {bannerPreview ? (
                    <img src={bannerPreview} alt="Banner Preview" className="w-full h-full object-cover" />
                  ) : (
                    <>
                      <Upload className="w-3.5 h-3.5" />
                      <span>Upload Banner</span>
                    </>
                  )}
                </button>
              </div>

              {/* Icon Upload */}
              <div>
                <input
                  type="file"
                  ref={iconInputRef}
                  onChange={handleIconSelect}
                  accept="image/png,image/jpeg,image/webp,image/svg+xml"
                  className="hidden"
                />
                <button
                  type="button"
                  onClick={() => iconInputRef.current?.click()}
                  className="w-full h-16 rounded-xl border border-dashed border-white/20 hover:border-purple-400/60 bg-white/[0.02] flex items-center justify-center gap-2 text-xs font-sans text-stone-300 hover:text-white transition-all cursor-pointer overflow-hidden"
                >
                  {iconPreview ? (
                    <img src={iconPreview} alt="Icon Preview" className="w-12 h-12 rounded-lg object-cover" />
                  ) : (
                    <>
                      <Upload className="w-3.5 h-3.5" />
                      <span>Upload Icon</span>
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>

          {/* Footer Actions */}
          <div className="flex items-center justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-xl text-xs font-sans font-semibold text-stone-400 hover:text-white transition-colors cursor-pointer"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isSubmitting || !campaignName.trim() || !playerName.trim()}
              className="px-6 py-2.5 rounded-xl bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white font-sans font-bold text-xs shadow-lg shadow-purple-600/30 transition-all cursor-pointer"
            >
              {isSubmitting ? 'Creating...' : 'Create Campaign'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
