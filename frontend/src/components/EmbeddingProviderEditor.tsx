import React from 'react';
import { Download, Check } from 'lucide-react';
import type { EmbeddingProviderConfig, EmbeddingProviderType } from '../types';

export interface EmbeddingPreset {
  label: string;
  description: string;
  config: EmbeddingProviderConfig;
  tier?: string;
  caveat?: string;
}

interface EmbeddingProviderEditorProps {
  value: EmbeddingProviderConfig;
  onChange: (value: EmbeddingProviderConfig) => void;
  presets: Record<string, EmbeddingPreset>;
  // modelInstalled reports whether the local encoder is present. It only matters
  // for the onnx type.
  modelInstalled: boolean;
  onDownloadModel: () => void;
  // sharedGeminiKey reports whether a Gemini key is configured elsewhere, so the
  // gemini type can say it will inherit it.
  sharedGeminiKey: boolean;
}

const inputClass =
  'w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60';
const labelClass = 'block text-xs font-sans uppercase text-stone-300 mb-1';

// EmbeddingProviderEditor edits one named embedding provider. The fields follow
// the selected type: the built-in projection takes nothing, the encoder takes an
// optional model path, and the API types take an endpoint and a key.
export const EmbeddingProviderEditor: React.FC<EmbeddingProviderEditorProps> = ({
  value,
  onChange,
  presets,
  modelInstalled,
  onDownloadModel,
  sharedGeminiKey,
}) => {
  const set = (patch: Partial<EmbeddingProviderConfig>) => onChange({ ...value, ...patch });
  const presetIDs = Object.keys(presets);

  return (
    <div className="space-y-3">
      {presetIDs.length > 0 && (
        <div>
          <label className={labelClass} htmlFor="embedding-preset">
            Load Preset
          </label>
          <select
            id="embedding-preset"
            className={inputClass}
            defaultValue=""
            onChange={(e) => {
              const preset = presets[e.target.value];
              if (preset) onChange({ ...preset.config });
              e.target.value = '';
            }}
          >
            <option value="" disabled>
              Choose a preset...
            </option>
            {presetIDs.map((id) => (
              <option key={id} value={id} title={presets[id].caveat}>
                {presets[id].label}
              </option>
            ))}
          </select>
        </div>
      )}

      <div>
        <label className={labelClass} htmlFor="embedding-type">
          Type
        </label>
        <select
          id="embedding-type"
          className={inputClass}
          value={value.type}
          onChange={(e) => set({ type: e.target.value as EmbeddingProviderType })}
        >
          <option value="builtin">Built-in projection (no model)</option>
          <option value="onnx">Built-in BGE encoder (local model)</option>
          <option value="http">OpenAI-compatible endpoint (OpenAI, Ollama)</option>
          <option value="gemini">Google Gemini</option>
        </select>
      </div>

      {value.type === 'onnx' && (
        <div className="space-y-2">
          <div>
            <label className={labelClass} htmlFor="embedding-model-path">
              Model directory (optional)
            </label>
            <input
              id="embedding-model-path"
              type="text"
              className={inputClass}
              placeholder="Leave blank to use the app's model cache"
              value={value.model_path ?? ''}
              onChange={(e) => set({ model_path: e.target.value })}
            />
          </div>
          <div className="flex items-center gap-2">
            {modelInstalled ? (
              <span className="inline-flex items-center gap-1.5 text-xs text-emerald-400">
                <Check className="h-3.5 w-3.5" />
                Encoder installed
              </span>
            ) : (
              <button
                type="button"
                onClick={onDownloadModel}
                className="inline-flex items-center gap-1.5 rounded-lg border border-purple-500/40 bg-purple-500/10 px-2.5 py-1 text-xs font-medium text-purple-200 hover:bg-purple-500/20"
              >
                <Download className="h-3.5 w-3.5" />
                Download encoder (~34 MB)
              </button>
            )}
          </div>
        </div>
      )}

      {(value.type === 'http' || value.type === 'gemini') && (
        <div className="space-y-2">
          {value.type === 'http' && (
            <div>
              <label className={labelClass} htmlFor="embedding-endpoint">
                Endpoint
              </label>
              <input
                id="embedding-endpoint"
                type="text"
                className={inputClass}
                placeholder="https://api.openai.com/v1"
                value={value.endpoint ?? value.url ?? ''}
                onChange={(e) => set({ endpoint: e.target.value })}
              />
            </div>
          )}
          <div>
            <label className={labelClass} htmlFor="embedding-model">
              Model
            </label>
            <input
              id="embedding-model"
              type="text"
              className={inputClass}
              placeholder={value.type === 'gemini' ? 'text-embedding-004' : 'text-embedding-3-small'}
              value={value.model ?? ''}
              onChange={(e) => set({ model: e.target.value })}
            />
          </div>
          <div>
            <label className={labelClass} htmlFor="embedding-api-key">
              API key
            </label>
            <input
              id="embedding-api-key"
              type="password"
              className={inputClass}
              placeholder={
                value.type === 'gemini' && sharedGeminiKey
                  ? 'Using the shared Gemini key (leave blank)'
                  : 'Required for a cloud endpoint'
              }
              value={value.api_key ?? ''}
              onChange={(e) => set({ api_key: e.target.value })}
            />
          </div>
        </div>
      )}
    </div>
  );
};
