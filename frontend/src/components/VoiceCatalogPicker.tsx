import React, { useMemo, useState } from 'react';
import { Play, Plus } from 'lucide-react';
import { TTSConfig, VoiceProfile } from '../types';
import { useTTSInspect } from '../hooks/useTTSInspect';
import { playVoicePreview } from '../lib/audioPreview';

interface VoiceCatalogPickerProps {
  ttsConfig: TTSConfig;
  onAddProfile: (profile: VoiceProfile) => void;
}

// VoiceCatalogPicker lists the voices the configured provider offers, so a voice
// that was never authored in Settings can still be auditioned and imported.
export const VoiceCatalogPicker: React.FC<VoiceCatalogPickerProps> = ({ ttsConfig, onAddProfile }) => {
  const { inspect, loading, error } = useTTSInspect(ttsConfig);
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('');

  const categories = useMemo(() => {
    const seen = new Set<string>();
    (inspect?.catalog.voices ?? []).forEach((voice) => voice.categories?.forEach((value) => seen.add(value)));
    return Array.from(seen).sort();
  }, [inspect]);

  const voices = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return (inspect?.catalog.voices ?? []).filter((voice) => {
      if (category && !voice.categories?.includes(category)) return false;
      if (!needle) return true;
      return voice.name.toLowerCase().includes(needle) || voice.id.toLowerCase().includes(needle);
    });
  }, [inspect, query, category]);

  if (!inspect?.catalog.available) return null;

  const addProfile = (id: string) => {
    const voice = voices.find((candidate) => candidate.id === id);
    if (!voice) return;
    onAddProfile({
      id: voice.id,
      name: voice.name,
      voice_id: voice.id,
      provider: inspect.provider_key,
      pitch: 1,
      speech_rate: 1,
      tags: voice.tags,
      description: voice.description,
      options: voice.defaults,
    });
  };

  return (
    <div className="space-y-2">
      <label className="text-xs font-sans uppercase text-stone-400">Provider Catalog</label>
      <div className="flex flex-wrap gap-2">
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search voices..."
          className="flex-1 min-w-[140px] bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
        />
        <select
          value={category}
          onChange={(e) => setCategory(e.target.value)}
          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none cursor-pointer"
        >
          <option value="">All categories</option>
          {categories.map((value) => (
            <option key={value} value={value}>
              {value}
            </option>
          ))}
        </select>
      </div>

      <div className="max-h-48 overflow-y-auto space-y-1.5 pr-1">
        {loading && voices.length === 0 && <p className="text-[11px] text-stone-500">Loading catalog...</p>}
        {(error || inspect?.error) && (
          <p className="text-[11px] text-red-400 font-mono p-1.5 bg-red-950/40 rounded border border-red-900/60">
            {error || inspect?.error}
          </p>
        )}
        {!loading && !error && !inspect?.error && voices.length === 0 && (
          <p className="text-[11px] text-stone-500">No voices match.</p>
        )}
        {voices.map((voice) => (
          <div
            key={voice.id}
            className="flex items-center justify-between gap-2 p-2 rounded bg-stone-950/70 border border-stone-800/80"
          >
            <div className="min-w-0">
              <div className="text-xs text-stone-200 truncate">{voice.name}</div>
              <div className="text-[10px] font-mono text-stone-500 truncate">{voice.id}</div>
            </div>
            <div className="flex items-center gap-1.5 shrink-0">
              {voice.preview_url && (
                <button
                  type="button"
                  onClick={() => playVoicePreview(voice.preview_url as string)}
                  className="p-1.5 rounded bg-stone-900 border border-stone-800 text-purple-400 hover:text-purple-300 cursor-pointer"
                  title="Audition"
                >
                  <Play className="w-3.5 h-3.5" />
                </button>
              )}
              <button
                type="button"
                onClick={() => addProfile(voice.id)}
                className="p-1.5 rounded bg-stone-900 border border-stone-800 text-purple-400 hover:text-purple-300 cursor-pointer"
                title="Add as profile"
              >
                <Plus className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
