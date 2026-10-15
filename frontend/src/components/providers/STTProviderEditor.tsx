import type { TestProviderRequest } from '../../types';
import React from 'react';
import { AlertCircle, CheckCircle, Mic, Zap } from 'lucide-react';
import type { STTConfig, TestProviderResponse } from '../../types';
import { tierLabel } from './TierBadge';

interface STTProviderEditorProps {
  entry: STTConfig;
  onChange: (next: STTConfig) => void;
  presets: Record<string, { label: string; description: string; config: STTConfig; tier?: string; caveat?: string }>;
  webSpeechAvailable: boolean;
  cartesiaSharedKey: boolean;
  onTest: (
    category: 'llm' | 'tts' | 'stt' | 'image',
    provider: TestProviderRequest['provider'],
    testPrompt?: string,
  ) => void;
  testingCategory: string | null;
  testResult: { category: string; res: TestProviderResponse } | null;
}

// STTProviderEditor edits one speech-to-text provider: its preset, engine
// type, endpoint, and key, with a test that transcribes a short sample.
export const STTProviderEditor: React.FC<STTProviderEditorProps> = ({
  entry,
  onChange,
  presets,
  webSpeechAvailable,
  cartesiaSharedKey,
  onTest,
  testingCategory,
  testResult,
}) => (
  <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
        <Mic className="w-4 h-4" />
        <span>Speech-to-Text (STT) Engine</span>
      </h3>
      <div className="flex flex-wrap items-center gap-3">
        <select
          onChange={(e) => {
            const key = e.target.value;
            if (key && presets[key]) {
              const preset = presets[key].config;
              onChange({ ...preset });
              e.target.value = '';
            }
          }}
          className="bg-stone-900 border border-purple-500/30 text-purple-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
          defaultValue=""
        >
          <option value="" disabled>
            ⚡ Load STT Preset…
          </option>
          {Object.entries(presets).map(([id, p]) => (
            <option key={id} value={id} title={p.caveat}>
              {p.label}
              {p.tier ? ` · ${tierLabel(p.tier)}` : ''}
            </option>
          ))}
        </select>
      </div>
    </div>

    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase text-stone-300">STT Provider Type</label>
        <select
          value={entry?.type === 'builtin' ? `builtin:${entry.builtin_name || 'cartesia'}` : entry?.type || 'disabled'}
          onChange={(e) => {
            const val = e.target.value;
            if (val.startsWith('builtin:')) {
              const builtinName = val.split(':')[1];
              onChange({
                ...(entry || { type: 'disabled' }),
                type: 'builtin',
                builtin_name: builtinName,
                model: builtinName === 'cartesia' ? entry?.model || 'ink-whisper' : entry?.model,
              });
            } else {
              onChange({
                ...(entry || { type: 'disabled' }),
                type: val as NonNullable<typeof entry>['type'],
                builtin_name: undefined,
              });
            }
          }}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
        >
          <option value="disabled">Disabled</option>
          <option value="builtin:cartesia">Built-in: Cartesia Ink (Cloud, metered)</option>
          <option value="web-speech" disabled={!webSpeechAvailable}>
            Web Speech API (Browser Native){webSpeechAvailable ? '' : ' — unavailable in this window'}
          </option>
          <option value="http">HTTP (Faster-Whisper, OpenAI Whisper)</option>
          <option value="cli">CLI Command (e.g. whisper-cli)</option>
        </select>
        {!webSpeechAvailable && (
          <p className="text-xs text-amber-300/80">
            Web Speech is not available in this window. Choose an HTTP or CLI Whisper provider for voice input.
          </p>
        )}
      </div>

      {entry?.type === 'http' && (
        <>
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Transcription Endpoint URL</label>
            <input
              type="text"
              placeholder="e.g. http://localhost:8000/v1/audio/transcriptions"
              value={entry.endpoint || ''}
              onChange={(e) => onChange({ ...entry, endpoint: e.target.value })}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Model Name</label>
            <input
              type="text"
              placeholder="e.g. whisper-1"
              value={entry.model || ''}
              onChange={(e) => onChange({ ...entry, model: e.target.value })}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
        </>
      )}

      {entry?.type === 'cli' && (
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Command / Binary</label>
          <input
            type="text"
            placeholder="e.g. whisper-cli"
            value={entry.command || ''}
            onChange={(e) => onChange({ ...entry, command: e.target.value })}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
        </div>
      )}

      {(entry?.type === 'cartesia' || (entry?.type === 'builtin' && entry?.builtin_name === 'cartesia')) && (
        <>
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Model Name</label>
            <input
              type="text"
              placeholder="e.g. ink-whisper"
              value={entry?.model || 'ink-whisper'}
              onChange={(e) => onChange({ ...entry, model: e.target.value })}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
              <span>API Key Override</span>
              {cartesiaSharedKey && !entry?.api_key && (
                <span className="text-xs text-emerald-400 font-mono">Using shared Cartesia key</span>
              )}
            </label>
            <input
              type="password"
              placeholder={
                cartesiaSharedKey
                  ? 'Using shared key from providers.cartesia.api_key'
                  : 'Optional override or CARTESIA_API_KEY env'
              }
              value={entry?.api_key || ''}
              onChange={(e) => onChange({ ...entry, api_key: e.target.value })}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
        </>
      )}
    </div>

    {entry?.type && entry.type !== 'disabled' && (
      <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
        <button
          onClick={() => onTest('stt', entry)}
          disabled={testingCategory === 'stt'}
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg bg-stone-900 border border-purple-500/30 hover:bg-stone-800 text-purple-400 transition-all cursor-pointer"
        >
          <Zap className="w-3.5 h-3.5" />
          <span>{testingCategory === 'stt' ? 'Transcribing...' : 'Test STT Connection'}</span>
        </button>

        {testResult?.category === 'stt' && (
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

export default STTProviderEditor;
