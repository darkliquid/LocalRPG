import React from 'react';
import { AlertCircle, CheckCircle, Sparkles, Zap } from 'lucide-react';
import type { ImageConfig, TestProviderResponse } from '../../types';
import { tierLabel } from './TierBadge';

interface ImageProviderEditorProps {
  entry: ImageConfig;
  onChange: (next: ImageConfig) => void;
  presets: Record<string, { label: string; description: string; config: ImageConfig; tier?: string; caveat?: string }>;
  geminiSharedKey: boolean;
  onTest: (category: 'llm' | 'tts' | 'stt' | 'image', provider: unknown, testPrompt?: string) => void;
  testingCategory: string | null;
  testResult: { category: string; res: TestProviderResponse } | null;
}

// ImageProviderEditor edits one image generator: its preset, engine type,
// endpoint, and key, with a test that renders a single picture.
export const ImageProviderEditor: React.FC<ImageProviderEditorProps> = ({
  entry,
  onChange,
  presets,
  geminiSharedKey,
  onTest,
  testingCategory,
  testResult,
}) => (
  <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
        <Sparkles className="w-4 h-4" />
        <span>Scene Art / Image Generator</span>
      </h3>
      <div className="flex flex-wrap items-center gap-3">
        <select
          onChange={(e) => {
            const key = e.target.value;
            if (key && presets[key]) {
              const preset = presets[key].config;
              onChange({ ...preset, auto_generate: entry.auto_generate });
              e.target.value = '';
            }
          }}
          className="bg-stone-900 border border-purple-500/30 text-purple-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
          defaultValue=""
        >
          <option value="" disabled>⚡ Load Image Preset…</option>
          {Object.entries(presets).map(([id, p]) => (
            <option key={id} value={id} title={p.caveat}>
              {p.label}{p.tier ? ` · ${tierLabel(p.tier)}` : ''}
            </option>
          ))}
        </select>

        <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
          <input
            type="checkbox"
            checked={entry.auto_generate}
            onChange={(e) =>
              onChange({ ...entry, auto_generate: e.target.checked })
            }
            className="rounded bg-stone-950 border-stone-800 text-purple-600 focus:ring-0"
          />
          <span>Auto-generate Scene Art</span>
        </label>

        <label htmlFor="location-art-appearance" className="text-xs font-sans uppercase text-stone-300">
          Location Art Appearance
        </label>
        <select
          id="location-art-appearance"
          value={entry.location_art_appearance ?? 'original_and_current'}
          title="The authored look alone, or that look with what play has done to the place layered on top of it."
          onChange={(e) =>
            onChange({ ...entry, location_art_appearance: e.target.value as 'original_and_current' | 'original' })
          }
          className="bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
        >
          <option value="original_and_current">Authored look, with what happened to it</option>
          <option value="original">Authored look alone</option>
        </select>

        <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
          <input
            type="checkbox"
            checked={entry.builtin_fallback !== false}
            onChange={(e) =>
              onChange({ ...entry, builtin_fallback: e.target.checked })
            }
            className="rounded bg-stone-950 border-stone-800 text-purple-600 focus:ring-0"
          />
          <span>Fallback to Procedural Art</span>
        </label>
      </div>
    </div>

    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase text-stone-300">Image Provider Type</label>
        <select
          value={entry.type}
          onChange={(e) =>
            onChange({ ...entry, type: e.target.value as any })
          }
          className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
        >
          <option value="disabled">Disabled</option>
          <option value="gemini">Google Gemini / Imagen (GenAI Cloud)</option>
          <option value="http">HTTP (ComfyUI, Automatic1111, LocalAI, DALL-E)</option>
          <option value="comfyui">ComfyUI Dedicated (Port 8188)</option>
          <option value="cli">CLI Command (e.g. sd-cli)</option>
          <option value="builtin">Builtin (procedural-art / mock / gemini)</option>
        </select>
      </div>

      {entry.type === 'builtin' && (
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Built-in Art Engine</label>
          <select
            value={entry.builtin_name || 'procedural-art'}
            onChange={(e) =>
              onChange({ ...entry, builtin_name: e.target.value })
            }
            className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
          >
            <option value="procedural-art">procedural-art (Pure-Go Vector Dark Fantasy SVG)</option>
            <option value="echo">echo (Debug Mock)</option>
          </select>
        </div>
      )}

      {entry.type === 'http' && (
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Image Endpoint URL</label>
          <input
            type="text"
            placeholder="e.g. http://localhost:7860/v1/images/generations"
            value={entry.endpoint || ''}
            onChange={(e) =>
              onChange({ ...entry, endpoint: e.target.value })
            }
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
        </div>
      )}

      {(entry.type === 'gemini' ||
        (entry.type === 'builtin' && entry.builtin_name === 'gemini')) && (
        <>
          <div className="space-y-1.5 col-span-1 md:col-span-2">
            <label className="text-xs font-sans uppercase text-stone-300">Gemini / Imagen Model</label>
            <input
              type="text"
              placeholder="e.g. imagen-3.0-generate-002 or gemini-3.1-flash-image"
              value={entry.model || 'imagen-3.0-generate-002'}
              onChange={(e) =>
                onChange({ ...entry, model: e.target.value })
              }
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <div className="flex flex-wrap gap-1.5 pt-1">
              {[
                { id: 'imagen-3.0-generate-002', label: 'Imagen 3' },
                { id: 'imagen-3.0-fast-generate-001', label: 'Imagen 3 Fast' },
                { id: 'gemini-3.1-flash-image', label: 'Nano Banana 2' },
                { id: 'gemini-3.1-flash-lite-image', label: 'Nano Banana 2 Lite' },
                { id: 'gemini-3-pro-image', label: 'Nano Banana Pro' },
                { id: 'gemini-2.5-flash-image', label: 'Nano Banana Original' },
              ].map((m) => (
                <button
                  key={m.id}
                  type="button"
                  onClick={() =>
                    onChange({ ...entry, model: m.id })
                  }
                  className={`px-2 py-0.5 text-xs font-mono rounded border transition-colors cursor-pointer ${
                    (entry.model || 'imagen-3.0-generate-002') === m.id
                      ? 'bg-purple-500/20 border-purple-500/50 text-purple-300'
                      : 'bg-stone-950 border-stone-800 text-stone-400 hover:text-stone-200'
                  }`}
                >
                  {m.label}
                </button>
              ))}
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Aspect Ratio</label>
            <select
              value={entry.aspect_ratio || '16:9'}
              onChange={(e) =>
                onChange({ ...entry, aspect_ratio: e.target.value })
              }
              className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
            >
              <option value="16:9">16:9 (Cinematic Widescreen - Default)</option>
              <option value="1:1">1:1 (Square)</option>
              <option value="4:3">4:3 (Landscape)</option>
              <option value="3:4">3:4 (Portrait)</option>
              <option value="9:16">9:16 (Vertical)</option>
              <option value="21:9">21:9 (Ultrawide)</option>
            </select>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Person Generation</label>
            <select
              value={entry.person_generation || 'ALLOW_ADULT'}
              onChange={(e) =>
                onChange({ ...entry, person_generation: e.target.value })
              }
              className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
            >
              <option value="ALLOW_ADULT">ALLOW_ADULT (Default — Adults, NPCs, Guards)</option>
              <option value="ALLOW_ALL">ALLOW_ALL (All Characters & Children)</option>
              <option value="DONT_ALLOW">DONT_ALLOW (No Characters / Landscapes Only)</option>
            </select>
          </div>

          <div className="space-y-1.5 col-span-1 md:col-span-2">
            <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
              <span>API Key Override</span>
              {geminiSharedKey && (
                <span className="text-xs text-emerald-400 font-mono">Shared key active</span>
              )}
            </label>
            <input
              type="password"
              placeholder={geminiSharedKey ? 'Using shared key (leave blank)' : 'Optional override or GEMINI_API_KEY env'}
              value={entry.api_key || ''}
              onChange={(e) =>
                onChange({ ...entry, api_key: e.target.value })
              }
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
        </>
      )}
    </div>

    {entry.type !== 'disabled' && (
      <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
        <button
          onClick={() => onTest('image', entry)}
          disabled={testingCategory === 'image'}
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg bg-stone-900 border border-purple-500/30 hover:bg-stone-800 text-purple-400 transition-all cursor-pointer"
        >
          <Zap className="w-3.5 h-3.5" />
          <span>{testingCategory === 'image' ? 'Generating...' : 'Test Image Generator'}</span>
        </button>

        {testResult?.category === 'image' && (
          <span
            className={`text-xs font-mono flex items-center gap-1 ${
              testResult.res.success ? 'text-emerald-400' : 'text-red-400'
            }`}
          >
            {testResult.res.success ? <CheckCircle className="w-3.5 h-3.5" /> : <AlertCircle className="w-3.5 h-3.5" />}
            <span>{testResult.res.message}</span>
          </span>
        )}
      </div>
    )}
  </div>
);

export default ImageProviderEditor;
