import React, { useState, useMemo, useEffect } from 'react';
import { X, Play, Plus, Search } from 'lucide-react';
import { ProviderVoice, VoiceProfile } from '../types';
import { playVoicePreview } from '../lib/audioPreview';

export interface VoiceCatalogModalProps {
  isOpen: boolean;
  onClose: () => void;
  voices: ProviderVoice[];
  providerKey: string;
  onAddProfile: (profile: VoiceProfile) => void;
}

export const VoiceCatalogModal: React.FC<VoiceCatalogModalProps> = ({
  isOpen,
  onClose,
  voices,
  providerKey,
  onAddProfile,
}) => {
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('');
  const [addedVoiceIDs, setAddedVoiceIDs] = useState<Set<string>>(new Set());

  // Close on Escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  const categories = useMemo(() => {
    const seen = new Set<string>();
    voices.forEach((v) => v.categories?.forEach((cat) => seen.add(cat)));
    return Array.from(seen).sort();
  }, [voices]);

  const filteredVoices = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return voices.filter((v) => {
      if (category && !v.categories?.includes(category)) return false;
      if (!needle) return true;
      return (
        v.name.toLowerCase().includes(needle) ||
        v.id.toLowerCase().includes(needle) ||
        v.tags?.some((t) => t.toLowerCase().includes(needle))
      );
    });
  }, [voices, query, category]);

  if (!isOpen) return null;

  const handleAdd = (voice: ProviderVoice) => {
    const slug = voice.name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '_')
      .replace(/^_+|_+$/g, '');

    onAddProfile({
      id: slug || voice.id,
      name: voice.name,
      voice_id: voice.id,
      provider: providerKey,
      pitch: 1.0,
      speech_rate: 1.0,
      tags: voice.tags || [],
      description: voice.description || `${voice.name} voice profile.`,
      options: voice.defaults,
    });

    setAddedVoiceIDs((prev) => new Set(prev).add(voice.id));
  };

  return (
    <div
      className="fixed inset-0 z-50 bg-black/70 backdrop-blur-xs flex items-center justify-center p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="bg-stone-950 border border-stone-800 rounded-2xl w-full max-w-2xl max-h-[85vh] flex flex-col shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-stone-800">
          <div>
            <h2 className="font-sans text-base font-bold text-purple-400">Voice Catalog Browser</h2>
            <p className="text-xs text-stone-400">Audition provider voices and add them directly into your archetypes.</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded-lg text-stone-400 hover:text-stone-200 hover:bg-stone-900 cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Filter bar */}
        <div className="p-4 border-b border-stone-800/80 bg-stone-900/30 flex flex-wrap gap-2.5">
          <div className="relative flex-1 min-w-[200px]">
            <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-stone-400" />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search by voice name, ID, or tag..."
              className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-8 pr-3 py-1.5 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
          {categories.length > 0 && (
            <select
              value={category}
              onChange={(e) => setCategory(e.target.value)}
              className="bg-stone-950 border border-stone-800 rounded-xl px-3 py-1.5 text-xs text-stone-200 focus:outline-none cursor-pointer"
            >
              <option value="">All categories</option>
              {categories.map((cat) => (
                <option key={cat} value={cat}>
                  {cat}
                </option>
              ))}
            </select>
          )}
        </div>

        {/* Voice list */}
        <div className="flex-1 overflow-y-auto p-4 space-y-2">
          {filteredVoices.length === 0 ? (
            <div className="text-center py-12 text-xs text-stone-500 italic">No voices match your search.</div>
          ) : (
            filteredVoices.map((voice) => {
              const isAdded = addedVoiceIDs.has(voice.id);
              return (
                <div
                  key={voice.id}
                  className="flex items-center justify-between gap-3 p-3 rounded-xl bg-stone-900/40 border border-stone-800/80 hover:border-stone-700 transition"
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="text-xs font-semibold text-stone-200">{voice.name}</span>
                      <span className="text-[10px] font-mono text-stone-500">{voice.id}</span>
                      {voice.accent && (
                        <span className="text-[9px] px-1.5 py-0.5 rounded bg-stone-900 border border-stone-800 text-stone-400 capitalize">
                          {voice.accent}
                        </span>
                      )}
                    </div>
                    {voice.description && (
                      <p className="text-[11px] text-stone-400 mt-0.5 truncate">{voice.description}</p>
                    )}
                    {voice.tags && voice.tags.length > 0 && (
                      <div className="flex flex-wrap gap-1 mt-1.5">
                        {voice.tags.slice(0, 4).map((tag) => (
                          <span
                            key={tag}
                            className="text-[9px] px-1 py-0.2 rounded bg-stone-950 text-stone-400 border border-stone-800"
                          >
                            {tag}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>

                  <div className="flex items-center gap-1.5 shrink-0">
                    {voice.preview_url && (
                      <button
                        type="button"
                        onClick={() => playVoicePreview(voice.preview_url as string)}
                        className="p-2 rounded-lg bg-stone-900 border border-stone-800 text-purple-400 hover:text-purple-300 hover:bg-stone-800 cursor-pointer"
                        title="Audition"
                      >
                        <Play className="w-3.5 h-3.5" />
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => handleAdd(voice)}
                      className={`flex items-center gap-1 text-xs px-2.5 py-1.5 rounded-lg border cursor-pointer font-sans font-medium transition ${
                        isAdded
                          ? 'bg-emerald-950/40 border-emerald-500/40 text-emerald-300'
                          : 'bg-purple-600/20 border-purple-500/40 text-purple-300 hover:bg-purple-600/30'
                      }`}
                      title="Add as profile"
                    >
                      <Plus className="w-3.5 h-3.5" />
                      <span>{isAdded ? 'Added' : 'Add'}</span>
                    </button>
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
};
