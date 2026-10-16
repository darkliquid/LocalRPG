import type { Dispatch, RefObject, SetStateAction } from 'react';
import { Palette, Sparkles, Tag, Trash2, Upload } from 'lucide-react';
import { AIGenerateButton } from '../../ui/AIGenerateButton';
import { safeImagePreview } from '../../../utils/security';
import { GenerationFailure, SystemInfo, WorldSelection } from '../../../types';

export interface WorldLoreTabProps {
  name: string;
  slugID: string;
  slugError: boolean;
  genre: string;
  artStyle: string;
  description: string;
  tags: string;
  defaultSystem: string;
  systems: SystemInfo[];
  selection: WorldSelection;
  isDraft: boolean;
  isSaving: boolean;
  savedID: string | null;
  generatingKind: 'banner' | 'icon' | null;
  bannerPreview: string | null;
  iconPreview: string | null;
  bannerInputRef: RefObject<HTMLInputElement | null>;
  iconInputRef: RefObject<HTMLInputElement | null>;
  setName: (name: string) => void;
  setSlugID: (id: string) => void;
  setSlugError: (error: boolean) => void;
  setGenre: (genre: string) => void;
  setArtStyle: (style: string) => void;
  setDescription: (description: string) => void;
  setTags: (tags: string) => void;
  setDefaultSystem: (system: string) => void;
  setBannerFile: Dispatch<SetStateAction<File | null>>;
  setBannerPreview: Dispatch<SetStateAction<string | null>>;
  setIconFile: Dispatch<SetStateAction<File | null>>;
  setIconPreview: Dispatch<SetStateAction<string | null>>;
  markDirty: () => void;
  reportGenerationError: (failure: GenerationFailure) => void;
  getWorldContext: () => Record<string, string>;
  handleAIGenerate: (kind: 'banner' | 'icon') => void | Promise<void>;
  openLightbox: (url: string, alt: string) => void;
  setDeleteTarget: (target: { id: string; name: string }) => void;
  setDeleteWorldError: (error: string | null) => void;
  setCanForceDelete: (canForce: boolean) => void;
}

// WorldLoreTab is the world's own metadata: what it is called, the setting name and
// description a catalogue shows, the system it defaults to, and the banner and icon
// its campaigns borrow.
export function WorldLoreTab({
  name,
  slugID,
  slugError,
  genre,
  artStyle,
  description,
  tags,
  defaultSystem,
  systems,
  selection,
  isDraft,
  isSaving,
  savedID,
  generatingKind,
  bannerPreview,
  iconPreview,
  bannerInputRef,
  iconInputRef,
  setName,
  setSlugID,
  setSlugError,
  setGenre,
  setArtStyle,
  setDescription,
  setTags,
  setDefaultSystem,
  setBannerFile,
  setBannerPreview,
  setIconFile,
  setIconPreview,
  markDirty,
  reportGenerationError,
  getWorldContext,
  handleAIGenerate,
  openLightbox,
  setDeleteTarget,
  setDeleteWorldError,
  setCanForceDelete,
}: WorldLoreTabProps) {
  return (
    <div className="flex-1 overflow-y-auto min-h-0 space-y-4 pr-2">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <label className="text-xs font-sans uppercase tracking-wider text-stone-300">World Setting Name</label>
            <AIGenerateButton
              formType="world"
              fieldName="name"
              onError={reportGenerationError}
              getContext={getWorldContext}
              onGenerated={(val) => {
                setName(val);
                markDirty();
                if (isDraft) {
                  setSlugID(
                    val
                      .toLowerCase()
                      .replace(/[^a-z0-9_]+/g, '_')
                      .replace(/^_+|_+$/g, ''),
                  );
                }
              }}
              systemID={defaultSystem}
            />
          </div>
          <input
            type="text"
            required
            placeholder="e.g. Solitary Defiance"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              markDirty();
              setSlugError(false);
              if (isDraft) {
                setSlugID(
                  e.target.value
                    .toLowerCase()
                    .replace(/[^a-z0-9_]+/g, '_')
                    .replace(/^_+|_+$/g, ''),
                );
              }
            }}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
          />
        </div>

        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <label className="text-xs font-sans uppercase tracking-wider text-stone-300">Genre / Setting Style</label>
            <AIGenerateButton
              formType="world"
              fieldName="genre"
              onError={reportGenerationError}
              getContext={getWorldContext}
              onGenerated={(val) => {
                setGenre(val);
                markDirty();
              }}
              systemID={defaultSystem}
            />
          </div>
          <input
            type="text"
            placeholder="e.g. Gothic Fantasy, Cyberpunk"
            value={genre}
            onChange={(e) => {
              setGenre(e.target.value);
              markDirty();
            }}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
          />
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase tracking-wider text-stone-300">Directory Slug ID</label>
          <input
            type="text"
            disabled={selection?.kind === 'saved'}
            placeholder="e.g. solitary_defiance"
            value={slugID}
            onChange={(e) => {
              setSlugID(e.target.value);
              setSlugError(false);
            }}
            className={`w-full bg-stone-950 border rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none transition-colors font-mono disabled:opacity-60 ${
              slugError ? 'border-red-500/70 focus:border-red-500' : 'border-stone-800 focus:border-purple-500/50'
            }`}
          />
          {slugError && <p className="text-xs text-red-400">That id already exists. Change the name or slug.</p>}
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase tracking-wider text-stone-300">Default Rule System</label>
          <select
            value={defaultSystem}
            onChange={(e) => setDefaultSystem(e.target.value)}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3.5 pr-9 py-2.5 text-sm text-stone-100 focus:outline-none focus:border-purple-500/50 transition-colors cursor-pointer"
          >
            {systems.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name} ({s.id})
              </option>
            ))}
          </select>
        </div>
      </div>

      <div className="space-y-1.5">
        <div className="flex items-center justify-between">
          <label className="text-xs font-sans uppercase tracking-wider text-stone-300 flex items-center gap-1.5">
            <Palette className="w-3.5 h-3.5 text-purple-400" />
            <span>Visual Art Style Prompt Guide</span>
          </label>
          <AIGenerateButton
            formType="world"
            fieldName="art_style"
            onError={reportGenerationError}
            getContext={getWorldContext}
            onGenerated={(val) => {
              setArtStyle(val);
              markDirty();
            }}
            systemID={defaultSystem}
            seed={artStyle}
          />
        </div>
        <input
          type="text"
          placeholder="e.g. Dark watercolor gothic, mist, gaslight, copper accents, muted palette"
          value={artStyle}
          onChange={(e) => {
            setArtStyle(e.target.value);
            markDirty();
          }}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
        />
        <p className="text-xs text-stone-400">
          Injected into image generation prompts to create consistent scene illustrations in this world.
        </p>
      </div>

      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase tracking-wider text-stone-300 flex items-center gap-1.5">
          <Tag className="w-3.5 h-3.5 text-purple-400" />
          <span>World Tags (Comma-separated)</span>
        </label>
        <input
          type="text"
          placeholder="e.g. gothic, horror, city, rebellion"
          value={tags}
          onChange={(e) => {
            setTags(e.target.value);
            markDirty();
          }}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors"
        />
      </div>

      <div className="space-y-1.5">
        <div className="flex items-center justify-between">
          <label className="text-xs font-sans uppercase tracking-wider text-stone-300">World Synopsis & Lore</label>
          <AIGenerateButton
            formType="world"
            fieldName="description"
            onError={reportGenerationError}
            getContext={getWorldContext}
            onGenerated={(val) => {
              setDescription(val);
              markDirty();
            }}
            systemID={defaultSystem}
            seed={description}
          />
        </div>
        <textarea
          rows={4}
          placeholder="Describe the setting, major conflicts, factions, and atmosphere..."
          value={description}
          onChange={(e) => {
            setDescription(e.target.value);
            markDirty();
          }}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl p-4 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/50 transition-colors resize-none"
        />
      </div>

      {/* World Artwork (Banner & Icon) */}
      <div className="space-y-3 pt-2">
        <label className="text-xs font-sans uppercase tracking-wider text-stone-300">World Artwork</label>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {/* Banner */}
          <div className="p-3 bg-stone-950 border border-stone-800 rounded-xl space-y-2">
            <span className="text-xs font-sans font-semibold text-stone-400">World Banner</span>
            <input
              type="file"
              ref={bannerInputRef}
              onChange={() => {
                const file = bannerInputRef.current?.files?.[0];
                if (file) {
                  setBannerFile(file);
                  const reader = new FileReader();
                  reader.onloadend = () => {
                    if (typeof reader.result === 'string') {
                      setBannerPreview(reader.result);
                    }
                  };
                  reader.readAsDataURL(file);
                }
              }}
              accept="image/png,image/jpeg,image/webp,image/svg+xml"
              className="hidden"
            />
            <div
              onClick={() =>
                bannerPreview ? openLightbox(bannerPreview, 'World banner') : bannerInputRef.current?.click()
              }
              className={`h-24 rounded-lg overflow-hidden border border-stone-800 bg-stone-900/40 flex items-center justify-center ${
                bannerPreview ? 'cursor-zoom-in' : 'cursor-pointer'
              }`}
            >
              {safeImagePreview(bannerPreview) ? (
                <img
                  src={safeImagePreview(bannerPreview)}
                  alt="Banner Preview"
                  className="w-full h-full object-cover"
                  onError={() => setBannerPreview(null)}
                />
              ) : (
                <span className="text-stone-600 text-xs">Click to upload banner</span>
              )}
            </div>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => bannerInputRef.current?.click()}
                disabled={isSaving || generatingKind === 'banner'}
                className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
              >
                <Upload className="w-3.5 h-3.5" />
                <span>Upload</span>
              </button>
              <button
                type="button"
                onClick={() => handleAIGenerate('banner')}
                disabled={isSaving || generatingKind === 'banner'}
                className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
              >
                <Sparkles className="w-3.5 h-3.5" />
                <span>{generatingKind === 'banner' ? 'Gen...' : 'AI Gen'}</span>
              </button>
            </div>
          </div>

          {/* Icon */}
          <div className="p-3 bg-stone-950 border border-stone-800 rounded-xl space-y-2">
            <span className="text-xs font-sans font-semibold text-stone-400">World Icon</span>
            <input
              type="file"
              ref={iconInputRef}
              onChange={() => {
                const file = iconInputRef.current?.files?.[0];
                if (file) {
                  setIconFile(file);
                  const reader = new FileReader();
                  reader.onloadend = () => {
                    if (typeof reader.result === 'string') {
                      setIconPreview(reader.result);
                    }
                  };
                  reader.readAsDataURL(file);
                }
              }}
              accept="image/png,image/jpeg,image/webp,image/svg+xml"
              className="hidden"
            />
            <div
              onClick={() => (iconPreview ? openLightbox(iconPreview, 'World icon') : iconInputRef.current?.click())}
              className={`h-24 rounded-lg overflow-hidden border border-stone-800 bg-stone-900/40 flex items-center justify-center ${
                iconPreview ? 'cursor-zoom-in' : 'cursor-pointer'
              }`}
            >
              {safeImagePreview(iconPreview) ? (
                <img
                  src={safeImagePreview(iconPreview)}
                  alt="Icon Preview"
                  className="w-16 h-16 rounded-xl object-cover"
                  onError={() => setIconPreview(null)}
                />
              ) : (
                <span className="text-stone-600 text-xs">Click to upload icon</span>
              )}
            </div>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => iconInputRef.current?.click()}
                disabled={isSaving || generatingKind === 'icon'}
                className="flex-1 py-1.5 px-2 rounded-lg bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-200 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
              >
                <Upload className="w-3.5 h-3.5" />
                <span>Upload</span>
              </button>
              <button
                type="button"
                onClick={() => handleAIGenerate('icon')}
                disabled={isSaving || generatingKind === 'icon'}
                className="flex-1 py-1.5 px-2 rounded-lg bg-purple-600/20 hover:bg-purple-600/30 border border-purple-500/40 text-xs font-sans font-semibold text-purple-300 flex items-center justify-center gap-1.5 transition-all cursor-pointer disabled:opacity-50"
              >
                <Sparkles className="w-3.5 h-3.5" />
                <span>{generatingKind === 'icon' ? 'Gen...' : 'AI Gen'}</span>
              </button>
            </div>
          </div>
        </div>

        {savedID && (
          <div className="p-4 bg-red-950/20 border border-red-500/30 rounded-2xl flex items-center justify-between mt-4">
            <div>
              <div className="text-xs font-sans font-bold text-red-400 flex items-center gap-1.5">
                <Trash2 className="w-3.5 h-3.5" />
                <span>Danger Zone: Delete World</span>
              </div>
              <p className="text-xs font-sans text-stone-400 mt-1">
                Permanently delete this world and all of its lore and entities from disk. This cannot be undone.
              </p>
            </div>
            <button
              type="button"
              onClick={() => {
                setDeleteTarget({ id: savedID, name: name || savedID });
                setDeleteWorldError(null);
                setCanForceDelete(false);
              }}
              className="px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white font-sans text-xs font-bold transition-all cursor-pointer shadow"
            >
              Delete World
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
