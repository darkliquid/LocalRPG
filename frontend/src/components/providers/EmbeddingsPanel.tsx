import React from 'react';
import { BrainCircuit } from 'lucide-react';
import type { AppConfig, EmbeddingProviderConfig, EmbeddingsConfig } from '../../types';
import { ProviderManager } from '../ProviderManager';
import { EmbeddingProviderEditor, type EmbeddingPreset } from '../EmbeddingProviderEditor';

interface EmbeddingsPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
  embeddings: EmbeddingsConfig;
  setEmbeddings: (patch: Partial<EmbeddingsConfig>) => void;
  embeddingPresets: Record<string, EmbeddingPreset>;
  encoderInstalled: boolean;
  sharedGeminiKey: boolean;
  setMissingModelPrompt: (prompt: { id: string; name: string; sizeBytes: number } | null) => void;
}

// EmbeddingsPanel chooses the embedding provider that powers recall and
// semantic search, and its size and batch settings.
export const EmbeddingsPanel: React.FC<EmbeddingsPanelProps> = ({
  config,
  setConfig,
  embeddings,
  setEmbeddings,
  embeddingPresets,
  encoderInstalled,
  sharedGeminiKey,
  setMissingModelPrompt,
}) => (
  <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
        <BrainCircuit className="w-4 h-4" />
        <span>Embeddings</span>
      </h3>
      <label className="flex items-center gap-2 text-xs text-stone-300">
        <input
          type="checkbox"
          aria-label="Enable semantic search"
          checked={embeddings.enabled}
          onChange={(e) => setEmbeddings({ enabled: e.target.checked })}
        />
        Enable semantic search
      </label>
    </div>
    <p className="text-xs text-stone-400">
      Embeddings power recall and semantic search. The built-in projection needs nothing; the
      built-in encoder generalises semantically and downloads its model once.
    </p>
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
      <div>
        <label className="block text-xs font-sans uppercase text-stone-300 mb-1" htmlFor="embeddings-dimensions">
          Dimensions
        </label>
        <input
          id="embeddings-dimensions"
          type="number"
          className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          value={embeddings.dimensions ?? 384}
          onChange={(e) => setEmbeddings({ dimensions: Number(e.target.value) || 0 })}
        />
      </div>
      <div>
        <label className="block text-xs font-sans uppercase text-stone-300 mb-1" htmlFor="embeddings-batch">
          Batch size
        </label>
        <input
          id="embeddings-batch"
          type="number"
          className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          value={embeddings.batch_size ?? 32}
          onChange={(e) => setEmbeddings({ batch_size: Number(e.target.value) || 0 })}
        />
      </div>
    </div>
    <ProviderManager
      family="embedding"
      config={config}
      onChange={setConfig}
      renderEditor={(_name, value, onChange) => (
        <EmbeddingProviderEditor
          value={value as EmbeddingProviderConfig}
          onChange={onChange as (next: EmbeddingProviderConfig) => void}
          presets={embeddingPresets}
          modelInstalled={encoderInstalled}
          sharedGeminiKey={sharedGeminiKey}
          onDownloadModel={() =>
            setMissingModelPrompt({ id: 'embedding-encoder', name: 'BGE Small Encoder', sizeBytes: 34235934 })
          }
        />
      )}
    />
  </div>
);

export default EmbeddingsPanel;
