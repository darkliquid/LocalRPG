import React, { useState } from 'react';
import { BrainCircuit, CheckCircle, Cloud, Cpu, Layers, Mic, Sparkles, Volume2, Zap } from 'lucide-react';
import type { AppConfig, EmbeddingProviderConfig, EmbeddingsConfig } from '../../types';
import { ProviderManager } from '../ProviderManager';
import { EmbeddingProviderEditor, type EmbeddingPreset } from '../EmbeddingProviderEditor';
import { OfflinePreset } from '../OfflinePreset';
import { SubTabs } from '../ui/SubTabs';

interface ProvidersPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
  embeddings: EmbeddingsConfig;
  setEmbeddings: (patch: Partial<EmbeddingsConfig>) => void;
  embeddingPresets: Record<string, EmbeddingPreset>;
  encoderInstalled: boolean;
  sharedGeminiKey: boolean;
  setMissingModelPrompt: (prompt: { id: string; name: string; sizeBytes: number } | null) => void;
}

// ProvidersPanel owns the account-level credentials and the embedding
// provider: the keys several providers share, and one place to choose the
// embedding model. It keeps its own secondary tab, so the studio shell does
// not track which of the three the user last read.
export const ProvidersPanel: React.FC<ProvidersPanelProps> = ({
  config,
  setConfig,
  embeddings,
  setEmbeddings,
  embeddingPresets,
  encoderInstalled,
  sharedGeminiKey,
  setMissingModelPrompt,
}) => {
  const [providersTab, setProvidersTab] = useState<'cloud' | 'embeddings' | 'offline'>('cloud');

  return (
  <div className="space-y-4 flex-1 overflow-y-auto pr-1">
    <SubTabs
      ariaLabel="Providers"
      tabs={[
        { id: 'cloud', label: 'Cloud Keys', icon: <Cloud className="w-3.5 h-3.5" /> },
        { id: 'embeddings', label: 'Embeddings', icon: <BrainCircuit className="w-3.5 h-3.5" /> },
        { id: 'offline', label: 'Offline', icon: <Layers className="w-3.5 h-3.5" /> },
      ]}
      active={providersTab}
      onChange={(id) => setProvidersTab(id as 'cloud' | 'embeddings' | 'offline')}
    />

    {providersTab === 'offline' && <OfflinePreset onChange={setConfig} />}

    {providersTab === 'embeddings' && (
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
    )}

    {providersTab === 'cloud' && (
    <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
      <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
        <Cloud className="w-4 h-4" />
        <span>Cloud & Ecosystem Providers</span>
      </h3>
      <p className="text-xs text-stone-400">
        Configure shared credentials and master keys for external AI and media providers that power multiple capabilities across LocalRPG.
      </p>

      {/* Google Gemini Provider Card */}
      <div className="p-4 bg-stone-950/80 border border-stone-800/90 rounded-xl space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-stone-800/60 pb-3">
          <div className="flex items-center gap-2">
            <div className="p-1.5 rounded-lg bg-purple-500/10 border border-purple-500/30 text-purple-400">
              <Sparkles className="w-4 h-4" />
            </div>
            <div>
              <h4 className="text-xs font-sans font-bold text-stone-200">Google Gemini (GenAI)</h4>
              <p className="text-xs text-stone-400">Multi-modal intelligence: text reasoning, image creation, and vocal performance.</p>
            </div>
          </div>
          <div>
            {config.providers?.gemini?.api_key ? (
              <span className="flex items-center gap-1 text-emerald-400 font-mono text-xs bg-emerald-950/40 border border-emerald-800/40 px-2 py-0.5 rounded-md">
                <CheckCircle className="w-3.5 h-3.5" />
                <span>Configured in Settings</span>
              </span>
            ) : (
              <span className="text-xs font-mono text-stone-500 bg-stone-900 border border-stone-800 px-2 py-0.5 rounded-md">
                Using env or unconfigured
              </span>
            )}
          </div>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
            <span>Shared Gemini API Key</span>
            <span className="text-xs text-stone-500 font-mono">
              {config.providers?.gemini?.api_key ? '✓ Custom Key Saved' : 'Optional if GEMINI_API_KEY is set'}
            </span>
          </label>
          <input
            type="password"
            placeholder="AIzaSy... or leave blank for GEMINI_API_KEY / GOOGLE_API_KEY env var"
            value={config.providers?.gemini?.api_key || ''}
            onChange={(e) => {
              setConfig({
                ...config,
                providers: {
                  ...config.providers,
                  gemini: {
                    ...config.providers?.gemini,
                    api_key: e.target.value,
                  },
                },
              });
            }}
            className="w-full bg-stone-900 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
          <p className="text-xs text-stone-500">
            Automatically inherited by Gemini LLM agents, Gemini/Imagen image generators, and Gemini TTS voice synthesis. Individual roles and media engines can still provide an override key.
          </p>
        </div>

        <div className="pt-2 border-t border-stone-800/50">
          <div className="text-xs font-sans uppercase text-stone-400 font-semibold mb-2">Connected Subsystems</div>
          <div className="flex flex-wrap gap-2">
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-xs text-stone-300 font-mono">
              <Cpu className="w-3 h-3 text-purple-400" />
              <span>AI Agents (GM, Narrator, Extractor)</span>
            </span>
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-xs text-stone-300 font-mono">
              <Sparkles className="w-3 h-3 text-purple-400" />
              <span>Image Generation (Imagen 3, Nano Banana)</span>
            </span>
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-xs text-stone-300 font-mono">
              <Volume2 className="w-3 h-3 text-purple-400" />
              <span>Voice Synthesis (Gemini 3.1 & 2.5 Flash/Pro TTS)</span>
            </span>
          </div>
        </div>
      </div>

      {/* Inworld AI Provider Card */}
      <div className="p-4 bg-stone-950/80 border border-stone-800/90 rounded-xl space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-stone-800/60 pb-3">
          <div className="flex items-center gap-2">
            <div className="p-1.5 rounded-lg bg-indigo-500/10 border border-indigo-500/30 text-indigo-400">
              <Cloud className="w-4 h-4" />
            </div>
            <div>
              <h4 className="text-xs font-sans font-bold text-stone-200">Inworld AI</h4>
              <p className="text-xs text-stone-400">Frontier model routing, cloud speech synthesis, and transcription.</p>
            </div>
          </div>
          <div>
            {config.providers?.inworld?.api_key ? (
              <span className="flex items-center gap-1 text-emerald-400 font-mono text-xs bg-emerald-950/40 border border-emerald-800/40 px-2 py-0.5 rounded-md">
                <CheckCircle className="w-3.5 h-3.5" />
                <span>Configured in Settings</span>
              </span>
            ) : (
              <span className="text-xs font-mono text-stone-500 bg-stone-900 border border-stone-800 px-2 py-0.5 rounded-md">
                Optional if INWORLD_API_KEY is set
              </span>
            )}
          </div>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
            <span>Shared Inworld API Key (Base64)</span>
            <span className="text-xs text-stone-500 font-mono">
              {config.providers?.inworld?.api_key ? '✓ Custom Key Saved' : 'Optional if INWORLD_API_KEY is set'}
            </span>
          </label>
          <input
            type="password"
            placeholder="Paste the Basic (Base64) key, or leave blank for INWORLD_API_KEY"
            value={config.providers?.inworld?.api_key || ''}
            onChange={(e) => {
              setConfig({
                ...config,
                providers: {
                  ...config.providers,
                  inworld: {
                    ...config.providers?.inworld,
                    api_key: e.target.value,
                  },
                },
              });
            }}
            className="w-full bg-stone-900 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-indigo-500/60"
          />
          <p className="text-xs text-stone-500">
            Sent as <code>Authorization: Basic …</code> to the Inworld LLM Router, Inworld TTS (<code>inworld-tts-2</code>), and Inworld STT
            (<code>inworld-stt-1</code>). Create a key at{' '}
            <a
              href="https://platform.inworld.ai/api-keys"
              target="_blank"
              rel="noreferrer"
              className="text-amber-400 hover:underline"
            >
              platform.inworld.ai/api-keys
            </a>{' '}
            or run <code>inworld auth login</code>. Individual roles and media engines can still provide an override key.
          </p>
        </div>

        <div className="pt-2 border-t border-stone-800/50">
          <div className="text-xs font-sans uppercase text-stone-400 font-semibold mb-2">Connected Subsystems</div>
          <div className="flex flex-wrap gap-2">
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-xs text-stone-300 font-mono">
              <Cpu className="w-3 h-3 text-indigo-400" />
              <span>LLM Router (compare-frontier-models)</span>
            </span>
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-xs text-stone-300 font-mono">
              <Volume2 className="w-3 h-3 text-indigo-400" />
              <span>Voice Synthesis (inworld-tts-2)</span>
            </span>
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-xs text-stone-300 font-mono">
              <Mic className="w-3 h-3 text-indigo-400" />
              <span>Transcription (inworld-stt-1)</span>
            </span>
          </div>
        </div>
      </div>

      {/* Cartesia Provider Card */}
      <div className="p-4 bg-stone-950/80 border border-stone-800/90 rounded-xl space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-stone-800/60 pb-3">
          <div className="flex items-center gap-2">
            <div className="p-1.5 rounded-lg bg-purple-500/10 border border-purple-500/30 text-purple-400">
              <Zap className="w-4 h-4" />
            </div>
            <div>
              <h4 className="text-xs font-sans font-bold text-stone-200">Cartesia</h4>
              <p className="text-xs text-stone-400">Ultra-fast voice intelligence: Sonic TTS speech synthesis and Ink Whisper STT transcription.</p>
            </div>
          </div>
          <div>
            {config.providers?.cartesia?.api_key ? (
              <span className="flex items-center gap-1 text-emerald-400 font-mono text-xs bg-emerald-950/40 border border-emerald-800/40 px-2 py-0.5 rounded-md">
                <CheckCircle className="w-3.5 h-3.5" />
                <span>Configured in Settings</span>
              </span>
            ) : (
              <span className="text-xs font-mono text-stone-500 bg-stone-900 border border-stone-800 px-2 py-0.5 rounded-md">
                Using env or unconfigured
              </span>
            )}
          </div>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
            <span>Shared Cartesia API Key</span>
            <span className="text-xs text-stone-500 font-mono">
              {config.providers?.cartesia?.api_key ? '✓ Custom Key Saved' : 'Optional if CARTESIA_API_KEY is set'}
            </span>
          </label>
          <input
            type="password"
            placeholder="sk_car_... or leave blank for CARTESIA_API_KEY env var"
            value={config.providers?.cartesia?.api_key || ''}
            onChange={(e) => {
              setConfig({
                ...config,
                providers: {
                  ...config.providers,
                  cartesia: {
                    ...config.providers?.cartesia,
                    api_key: e.target.value,
                  },
                },
              });
            }}
            className="w-full bg-stone-900 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
          <p className="text-xs text-stone-500">
            Shared key automatically inherited by Cartesia Sonic TTS voice synthesis and Cartesia Ink STT transcription.
          </p>
        </div>

        <div className="pt-2 border-t border-stone-800/50">
          <div className="text-xs font-sans uppercase text-stone-400 font-semibold mb-2">Connected Subsystems</div>
          <div className="flex flex-wrap gap-2">
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-xs text-stone-300 font-mono">
              <Volume2 className="w-3 h-3 text-purple-400" />
              <span>Voice Synthesis (Cartesia Sonic 3.6)</span>
            </span>
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-xs text-stone-300 font-mono">
              <Mic className="w-3 h-3 text-purple-400" />
              <span>Speech-to-Text (Cartesia Ink Whisper)</span>
            </span>
          </div>
        </div>
      </div>
    </div>
    )}
  </div>
  );
};

export default ProvidersPanel;
