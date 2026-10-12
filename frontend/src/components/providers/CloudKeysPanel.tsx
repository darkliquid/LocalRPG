import React from 'react';
import { CheckCircle, Cloud, Cpu, Mic, Sparkles, Volume2, Zap } from 'lucide-react';
import type { AppConfig } from '../../types';

interface CloudKeysPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
}

// CloudKeysPanel holds the account keys several providers share: Gemini for
// narration and images, Inworld for speech, and Cartesia for transcription.
// Each card names what the key unlocks rather than which vendor sells it.
export const CloudKeysPanel: React.FC<CloudKeysPanelProps> = ({ config, setConfig }) => (
  <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
    <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
      <Cloud className="w-4 h-4" />
      <span>Cloud & Ecosystem Providers</span>
    </h3>
    <p className="text-xs text-stone-400">
      Configure shared credentials and master keys for external AI and media providers that power multiple capabilities
      across LocalRPG.
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
            <p className="text-xs text-stone-400">
              Multi-modal intelligence: text reasoning, image creation, and vocal performance.
            </p>
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
          Automatically inherited by Gemini LLM agents, Gemini/Imagen image generators, and Gemini TTS voice synthesis.
          Individual roles and media engines can still provide an override key.
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
          Sent as <code>Authorization: Basic …</code> to the Inworld LLM Router, Inworld TTS (<code>inworld-tts-2</code>
          ), and Inworld STT (<code>inworld-stt-1</code>). Create a key at{' '}
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
            <p className="text-xs text-stone-400">
              Ultra-fast voice intelligence: Sonic TTS speech synthesis and Ink Whisper STT transcription.
            </p>
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
);

export default CloudKeysPanel;
