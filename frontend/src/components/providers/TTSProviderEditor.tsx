import type { TestProviderRequest } from '../../types';
import React from 'react';
import { AlertCircle, CheckCircle, Play, Plus, RotateCcw, Sparkles, Trash2, Users, Volume2 } from 'lucide-react';
import type {
  ModelStatus,
  ProviderVoice,
  TestProviderResponse,
  TTSConfig,
  TTSInspectResponse,
  VoiceProfile,
} from '../../types';
import { VoiceCombobox } from '../VoiceCombobox';
import { VoiceOptionsControl } from '../VoiceOptionsControl';
import { tierLabel } from './TierBadge';
import { DEFAULT_VOICE_PROFILES, KOKORO_VOICE_PROFILES } from '../../lib/voiceProfiles';
import { DEFAULT_TTS_PREVIEW_TEXT } from '../../lib/mediaProviders';

interface TTSProviderEditorProps {
  entry: TTSConfig;
  onChange: (next: TTSConfig) => void;
  presets: Record<string, { label: string; description: string; config: TTSConfig; tier?: string; caveat?: string }>;
  voices: ProviderVoice[];
  inspect: TTSInspectResponse | null;
  inspecting: boolean;
  onRefreshInspect: () => void;
  inspectError: string | null;
  kokoroStatus: ModelStatus | undefined;
  isBuiltinKokoro: boolean;
  isKokoro: boolean;
  isGeminiTTS: boolean;
  isElevenLabsTTS: boolean;
  isCartesiaTTS: boolean;
  geminiSharedKey: boolean;
  cartesiaSharedKey: boolean;
  previewText: string;
  onPreviewTextChange: (text: string) => void;
  onOpenVoiceCatalog: () => void;
  onTest: (
    category: 'llm' | 'tts' | 'stt' | 'image',
    provider: TestProviderRequest['provider'],
    testPrompt?: string,
  ) => void;
  testingCategory: string | null;
  testResult: { category: string; res: TestProviderResponse } | null;
  onMissingModel: (prompt: { id: string; name: string; sizeBytes: number }) => void;
}

// TTSProviderEditor edits one speech engine: its preset, type, endpoint, and
// key, the voice it narrates with, and the voice profile library that names
// each character's archetype.
export const TTSProviderEditor: React.FC<TTSProviderEditorProps> = ({
  entry,
  onChange,
  presets,
  voices,
  inspect,
  inspecting,
  onRefreshInspect,
  inspectError,
  kokoroStatus,
  isBuiltinKokoro,
  isKokoro,
  isGeminiTTS,
  isElevenLabsTTS,
  isCartesiaTTS,
  geminiSharedKey,
  cartesiaSharedKey,
  previewText,
  onPreviewTextChange,
  onOpenVoiceCatalog,
  onTest,
  testingCategory,
  testResult,
  onMissingModel,
}) => (
  <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
        <Volume2 className="w-4 h-4" />
        <span>Text-to-Speech (TTS) Engine</span>
      </h3>
      <div className="flex flex-wrap items-center gap-3">
        <select
          onChange={(e) => {
            const key = e.target.value;
            if (key && presets[key]) {
              const preset = presets[key].config;
              onChange({
                ...preset,
                builtin_name: preset.builtin_name,
                options: preset.options,
                endpoint: preset.endpoint,
                model: preset.model,
                api_key: preset.api_key,
                auto_play: entry.auto_play,
                voice_profiles: preset.voice_profiles ?? entry.voice_profiles,
              });
              e.target.value = '';
            }
          }}
          className="bg-stone-900 border border-purple-500/30 text-purple-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
          defaultValue=""
        >
          <option value="" disabled>
            ⚡ Load TTS Preset…
          </option>
          {Object.entries(presets).map(([id, p]) => (
            <option key={id} value={id} title={p.caveat}>
              {p.label}
              {p.tier ? ` · ${tierLabel(p.tier)}` : ''}
            </option>
          ))}
        </select>

        <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
          <input
            type="checkbox"
            checked={entry.auto_play}
            onChange={(e) => onChange({ ...entry, auto_play: e.target.checked })}
            className="rounded bg-stone-950 border-stone-800 text-purple-600 focus:ring-0"
          />
          <span>Auto-play Narration</span>
        </label>
      </div>
    </div>

    <div className="space-y-1.5">
      <label className="text-xs font-sans uppercase text-stone-300">Narration Markdown</label>
      <select
        value={entry.markdown || 'auto'}
        onChange={(e) => onChange({ ...entry, markdown: e.target.value as 'auto' | 'strip' | 'keep' })}
        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60"
      >
        <option value="auto">Auto - reduce formatting unless the provider understands it</option>
        <option value="strip">Always reduce formatting to plain speech</option>
        <option value="keep">Keep formatting as written</option>
      </select>
      <p className="text-xs text-stone-500">
        Markdown emphasis, headings, lists and wikilinks are otherwise read aloud by engines that do not interpret them.
      </p>
    </div>

    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase text-stone-300">TTS Engine</label>
        <select
          value={
            entry.type === 'gemini' || (entry.type === 'builtin' && entry.builtin_name === 'gemini')
              ? 'gemini'
              : entry.type === 'cartesia' || (entry.type === 'builtin' && entry.builtin_name === 'cartesia')
                ? 'builtin:cartesia'
                : entry.type === 'builtin'
                  ? `builtin:${entry.builtin_name || 'native-os'}`
                  : entry.type === 'fish-audio' ||
                      (entry.type === 'http' && (entry.model?.includes('fish') || entry.model?.includes('s2-pro')))
                    ? 'fish-audio'
                    : entry.type
          }
          onChange={(e) => {
            const val = e.target.value;
            if (val.startsWith('builtin:')) {
              const builtinName = val.split(':')[1];
              // A builtin that needs a model and a stock voice gets sane
              // defaults here, so choosing the engine alone is enough.
              const builtinDefaults =
                builtinName === 'elevenlabs'
                  ? {
                      model: entry.model && entry.model.includes('eleven') ? entry.model : 'eleven_multilingual_v2',
                      default_voice:
                        entry.default_voice && entry.default_voice.startsWith('EXAV')
                          ? entry.default_voice
                          : 'EXAVITQu4vr4xnSDxMaL',
                    }
                  : builtinName === 'cartesia'
                    ? {
                        model: entry.model && entry.model.includes('sonic') ? entry.model : 'sonic-3.6',
                        default_voice: entry.default_voice || 'db6b0ed5-d5d3-463d-ae85-518a07d3c2b4',
                      }
                    : builtinName === 'sherpa-onnx'
                      ? {
                          default_voice:
                            entry.default_voice && entry.default_voice.startsWith('af_')
                              ? entry.default_voice
                              : 'af_bella',
                        }
                      : {};
              onChange({
                ...entry,
                ...builtinDefaults,
                type: 'builtin',
                builtin_name: builtinName,
                options: builtinName === 'elevenlabs' ? entry.options : undefined,
              });
            } else if (val === 'gemini') {
              onChange({
                ...entry,
                type: 'gemini',
                builtin_name: undefined,
                model: entry.model && entry.model.includes('gemini') ? entry.model : 'gemini-3.8-flash-tts',
                default_voice:
                  entry.default_voice &&
                  !entry.default_voice.startsWith('EXAV') &&
                  !entry.default_voice.startsWith('af_')
                    ? entry.default_voice
                    : 'Aoede',
                options: undefined,
              });
            } else if (val === 'fish-audio') {
              onChange({
                ...entry,
                type: 'http',
                builtin_name: undefined,
                endpoint: entry.endpoint && entry.endpoint.includes('8091') ? entry.endpoint : 'http://localhost:8091',
                model: 'fishaudio/s2-pro',
                default_voice: entry.default_voice || 'default',
                options: undefined,
              });
            } else if (val === 'http') {
              onChange({
                ...entry,
                type: 'http',
                builtin_name: undefined,
                endpoint: entry.endpoint || 'http://localhost:8880/v1/audio/speech',
                model:
                  entry.model &&
                  !entry.model.includes('eleven') &&
                  !entry.model.includes('gemini') &&
                  !entry.model.includes('fish') &&
                  !entry.model.includes('s2-pro')
                    ? entry.model
                    : 'kokoro',
                default_voice:
                  entry.default_voice && !entry.default_voice.startsWith('EXAV') && entry.default_voice !== 'Aoede'
                    ? entry.default_voice
                    : 'af_bella',
                options: undefined,
              });
            } else {
              onChange({ ...entry, type: val as (typeof entry)['type'], builtin_name: undefined, options: undefined });
            }
          }}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
        >
          <option value="disabled">Disabled</option>
          <option value="gemini">Google Gemini TTS (Cloud, metered)</option>
          <option value="builtin:cartesia">Built-in: Cartesia Sonic (Cloud, metered)</option>
          <option value="builtin:sherpa-onnx">Built-in: Sherpa-ONNX (Kokoro Neural Voice)</option>
          <option value="builtin:native-os">Built-in: Native OS Speech (spd-say / SAPI / procedural)</option>
          <option value="builtin:elevenlabs">Built-in: ElevenLabs (Cloud, metered)</option>
          <option value="fish-audio">Fish Audio S2 (Local vLLM-Omni)</option>
          <option value="http">HTTP Endpoint (Kokoro-FastAPI, AllTalk, OpenAI Speech)</option>
          <option value="cli">CLI Command (e.g. piper)</option>
        </select>
      </div>

      {entry.type === 'http' && (
        <>
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Endpoint URL</label>
            <input
              type="text"
              placeholder="e.g. http://localhost:8880"
              value={entry.endpoint || ''}
              onChange={(e) => onChange({ ...entry, endpoint: e.target.value })}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <p className="text-xs text-stone-500">
              Accepts either the base server URL (e.g. http://localhost:8880) or the full /v1/audio/speech endpoint.
            </p>
          </div>
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Model Name</label>
            <input
              type="text"
              placeholder="e.g. kokoro or tts-1"
              value={entry.model || ''}
              onChange={(e) => onChange({ ...entry, model: e.target.value })}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">API Key (Optional)</label>
            <input
              type="password"
              placeholder="Optional authorization token (e.g. for OpenAI)"
              value={entry.api_key || ''}
              onChange={(e) => onChange({ ...entry, api_key: e.target.value })}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
        </>
      )}

      {entry.type === 'cli' && (
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Command / Binary</label>
          <input
            type="text"
            placeholder="e.g. piper"
            value={entry.command || ''}
            onChange={(e) => onChange({ ...entry, command: e.target.value })}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
        </div>
      )}

      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
          <span>Master Volume</span>
          <span className="font-mono text-purple-400">{Math.round((entry.master_volume || 1.0) * 100)}%</span>
        </label>
        <input
          type="range"
          min="0"
          max="1"
          step="0.05"
          value={entry.master_volume || 1.0}
          onChange={(e) => onChange({ ...entry, master_volume: parseFloat(e.target.value) })}
          className="w-full accent-purple-500"
        />
      </div>
    </div>

    {isBuiltinKokoro && (
      <div className="p-3 bg-stone-950/80 border border-stone-800 rounded-xl flex flex-wrap items-center justify-between gap-3 text-xs">
        <div className="flex items-center gap-2">
          <Volume2 className="w-4 h-4 text-purple-400" />
          <span className="font-medium text-stone-200">Kokoro Model:</span>
          {kokoroStatus?.installed ? (
            <span className="flex items-center gap-1 text-emerald-400 font-mono text-xs bg-emerald-950/40 border border-emerald-800/40 px-2 py-0.5 rounded-md">
              <CheckCircle className="w-3.5 h-3.5" />
              <span>Installed</span>
            </span>
          ) : kokoroStatus?.downloading ? (
            <div className="flex items-center gap-2 text-purple-400 font-mono text-xs">
              <span>Downloading {Math.round(kokoroStatus.progress * 100)}%</span>
              {/* Decoration: the percentage is written beside it. */}
              <div aria-hidden="true" className="w-20 h-1.5 bg-stone-800 rounded-full overflow-hidden">
                <div
                  className="h-full bg-purple-500"
                  style={{ width: `${Math.round(kokoroStatus.progress * 100)}%` }}
                />
              </div>
            </div>
          ) : (
            <span className="flex items-center gap-1 text-purple-400/90 font-mono text-xs bg-purple-950/40 border border-purple-800/40 px-2 py-0.5 rounded-md">
              <AlertCircle className="w-3.5 h-3.5" />
              <span>Not Installed (~320 MB)</span>
            </span>
          )}
        </div>

        {!kokoroStatus?.installed && (
          <button
            disabled={kokoroStatus?.downloading}
            onClick={() =>
              onMissingModel({
                id: 'kokoro-tts',
                name: 'Kokoro Voice Pack',
                sizeBytes: 319625534,
              })
            }
            className="flex items-center gap-1.5 px-3 py-1 bg-purple-600 hover:bg-purple-500 text-stone-950 font-sans font-bold rounded-lg transition shadow text-xs cursor-pointer disabled:opacity-50"
          >
            <span>{kokoroStatus?.downloading ? 'Downloading...' : 'Download Model'}</span>
          </button>
        )}
      </div>
    )}

    {isGeminiTTS && (
      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase text-stone-300">Gemini TTS Model</label>
        <input
          type="text"
          placeholder="e.g. gemini-3.8-flash-tts"
          value={entry.model || 'gemini-3.8-flash-tts'}
          onChange={(e) => onChange({ ...entry, model: e.target.value })}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
        />
        <div className="flex flex-wrap gap-1.5 pt-1">
          {[
            { id: 'gemini-3.8-flash-tts', label: '3.8 Flash TTS' },
            { id: 'gemini-3.8-flash-lite-tts', label: '3.8 Flash-Lite TTS' },
            { id: 'gemini-3.1-flash-tts-preview', label: '3.1 Flash TTS' },
            { id: 'gemini-2.5-flash-preview-tts', label: '2.5 Flash TTS' },
            { id: 'gemini-2.5-pro-preview-tts', label: '2.5 Pro TTS' },
          ].map((m) => (
            <button
              key={m.id}
              type="button"
              onClick={() => onChange({ ...entry, model: m.id })}
              className={`text-xs px-2.5 py-1 rounded-lg border font-mono transition cursor-pointer ${
                (entry.model || 'gemini-3.8-flash-tts') === m.id
                  ? 'bg-purple-500/20 border-purple-500/60 text-purple-300'
                  : 'bg-stone-900/60 border-stone-800 text-stone-400 hover:text-stone-200'
              }`}
            >
              {m.label}
            </button>
          ))}
        </div>
      </div>
    )}

    {isCartesiaTTS && (
      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase text-stone-300">Cartesia TTS Model</label>
        <input
          type="text"
          placeholder="e.g. sonic-3.6"
          value={entry.model || 'sonic-3.6'}
          onChange={(e) => onChange({ ...entry, model: e.target.value })}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
        />
        <div className="flex flex-wrap gap-1.5 pt-1">
          {[
            { id: 'sonic-3.6', label: 'Sonic 3.6 (Latest)' },
            { id: 'sonic', label: 'Sonic (Default)' },
          ].map((m) => (
            <button
              key={m.id}
              type="button"
              onClick={() => onChange({ ...entry, model: m.id })}
              className={`text-xs px-2.5 py-1 rounded-lg border font-mono transition cursor-pointer ${
                (entry.model || 'sonic-3.6') === m.id
                  ? 'bg-purple-500/20 border-purple-500/60 text-purple-300'
                  : 'bg-stone-900/60 border-stone-800 text-stone-400 hover:text-stone-200'
              }`}
            >
              {m.label}
            </button>
          ))}
        </div>
      </div>
    )}

    {inspect?.metered && (
      <div className="flex items-center gap-2 text-xs font-mono text-purple-400/90">
        <span className="px-1.5 py-0.5 rounded border border-purple-500/40 bg-purple-500/10">METERED</span>
        <span>This provider charges per request. Cached clips are reused.</span>
      </div>
    )}

    {inspect?.key_required && !inspect.key_present && (
      <div className="text-xs font-mono text-stone-400">
        {isGeminiTTS
          ? 'No Gemini API key configured. Enter one below, or set GEMINI_API_KEY / GOOGLE_API_KEY in the environment.'
          : isCartesiaTTS
            ? 'No Cartesia API key configured. Enter one below, configure it in the Providers tab, or set CARTESIA_API_KEY in the environment.'
            : isElevenLabsTTS
              ? 'No API key configured. Enter one below, or set ELEVENLABS_API_KEY in the environment.'
              : 'No API key configured. Enter one below.'}
      </div>
    )}

    {inspect?.key_required && (
      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
          <span>API Key</span>
          {isGeminiTTS && geminiSharedKey && !entry.api_key && (
            <span className="text-xs text-emerald-400 font-mono">Using shared Gemini key</span>
          )}
          {isCartesiaTTS && cartesiaSharedKey && !entry.api_key && (
            <span className="text-xs text-emerald-400 font-mono">Using shared Cartesia key</span>
          )}
        </label>
        <input
          type="password"
          placeholder={
            isGeminiTTS && geminiSharedKey
              ? 'Using shared key from providers.gemini.api_key'
              : isCartesiaTTS && cartesiaSharedKey
                ? 'Using shared key from providers.cartesia.api_key'
                : "Leave empty to use the provider's environment variable"
          }
          value={entry.api_key || ''}
          onChange={(e) => onChange({ ...entry, api_key: e.target.value })}
          className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
        />
        <p className="text-xs text-stone-500">
          {isGeminiTTS
            ? 'Stored in your configuration file. Set GEMINI_API_KEY instead to keep it off disk.'
            : isCartesiaTTS
              ? 'Stored in your configuration file. Set CARTESIA_API_KEY or configure in Providers tab to share across TTS and STT.'
              : isElevenLabsTTS
                ? 'Stored in your configuration file. Set ELEVENLABS_API_KEY instead to keep it off disk.'
                : 'Stored in your configuration file.'}
        </p>
      </div>
    )}

    {inspect && inspect.options && inspect.options.length > 0 && (
      <div className="p-3 rounded-lg bg-stone-950/70 border border-stone-800/80 space-y-2">
        <label className="text-xs font-sans uppercase text-stone-300">Provider Tuning</label>
        <VoiceOptionsControl
          schema={inspect.options}
          values={entry.options ?? {}}
          onChange={(key, value) => onChange({ ...entry, options: { ...(entry.options ?? {}), [key]: value } })}
        />
      </div>
    )}

    {inspect?.catalog?.available && (
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs font-mono text-stone-400">
        <span>
          {inspect.catalog.voices?.length ?? 0} voices
          {inspect.catalog.fetched_at ? ` - last checked ${new Date(inspect.catalog.fetched_at).toLocaleString()}` : ''}
          {inspect.catalog.stale ? ' (catalog unavailable, showing the last copy)' : ''}
        </span>
        <button
          type="button"
          onClick={onRefreshInspect}
          disabled={inspecting}
          className="px-2 py-1 rounded bg-stone-900 border border-stone-800 text-purple-400 hover:text-purple-300 hover:border-purple-500/40 cursor-pointer disabled:opacity-50"
        >
          {inspecting ? 'Refreshing...' : 'Refresh Catalog'}
        </button>
      </div>
    )}

    {(inspect?.error || inspectError) && (
      <div className="p-2.5 rounded-lg bg-red-950/40 border border-red-900/60 text-xs text-red-300 font-mono">
        {inspect?.error || inspectError}
      </div>
    )}

    {entry.type !== 'disabled' && (
      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase text-stone-300">Default Voice</label>
        <VoiceCombobox
          value={entry.default_voice || ''}
          onChange={(voiceID) => onChange({ ...entry, default_voice: voiceID })}
          voices={voices}
          placeholder="Select default provider voice..."
        />
        <p className="text-xs text-stone-500">Fallback voice used for turn narration and unvoiced characters.</p>
      </div>
    )}

    {entry.type !== 'disabled' && (
      <div className="space-y-3 p-4 rounded-xl bg-stone-900/40 border border-stone-800">
        <div className="flex items-center justify-between">
          <div className="space-y-0.5">
            <div className="text-xs font-sans font-bold text-stone-200">Speech Steering & Acting Cues</div>
            <div className="text-xs text-stone-400">
              Instruct the GM to use emotive directions (e.g. [whispers], [sighs]) when supported.
            </div>
          </div>
          <input
            type="checkbox"
            checked={entry.speech_cues?.enabled ?? true}
            onChange={(e) =>
              onChange({
                ...entry,
                speech_cues: {
                  ...entry.speech_cues,
                  enabled: e.target.checked,
                },
              })
            }
            className="w-4 h-4 rounded border-stone-700 bg-stone-900 text-purple-500 focus:ring-purple-500/40 cursor-pointer"
          />
        </div>

        {inspect?.speech_cues && (
          <div className="text-xs p-2.5 rounded-lg bg-stone-950/60 border border-stone-800/80 text-stone-400">
            <span className="font-semibold text-stone-300">Provider Capabilities: </span>
            {inspect.speech_cues.audio_tags ? (
              <span className="text-purple-400">
                Supports bracketed vocal cues (
                {inspect.speech_cues.supported_tags
                  ?.slice(0, 5)
                  .map((t) => `[${t}]`)
                  .join(', ')}
                ...)
              </span>
            ) : inspect.speech_cues.markdown_emphasis ? (
              <span className="text-stone-300">Supports Markdown emphasis (*emphasis*)</span>
            ) : (
              <span className="text-stone-500">Plain text only; vocal tags are stripped before synthesis.</span>
            )}
          </div>
        )}

        {(entry.speech_cues?.enabled ?? true) && (
          <div className="space-y-1.5 pt-1">
            <label className="text-xs font-sans uppercase text-stone-300">Transcript Display Mode</label>
            <select
              value={entry.speech_cues?.display_mode || 'stage_directions'}
              onChange={(e) =>
                onChange({
                  ...entry,
                  speech_cues: {
                    ...entry.speech_cues,
                    enabled: entry.speech_cues?.enabled ?? true,
                    display_mode: e.target.value as NonNullable<(typeof entry)['speech_cues']>['display_mode'],
                  },
                })
              }
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
            >
              <option value="stage_directions">Stage Directions (styled tags in transcript)</option>
              <option value="hidden">Hidden (acted out in audio, hidden in transcript)</option>
              <option value="raw">Raw text (unmodified brackets)</option>
            </select>
          </div>
        )}
      </div>
    )}

    {entry.type !== 'disabled' && (
      <div className="pt-2 space-y-2 border-t border-stone-800/60">
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Preview Phrase</label>
          <input
            type="text"
            value={previewText}
            onChange={(e) => onPreviewTextChange(e.target.value)}
            placeholder={DEFAULT_TTS_PREVIEW_TEXT}
            className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
        </div>

        <div className="flex items-center justify-between">
          <button
            onClick={() => onTest('tts', entry, previewText)}
            disabled={testingCategory === 'tts'}
            className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg bg-stone-900 border border-purple-500/30 hover:bg-stone-800 text-purple-400 transition-all cursor-pointer"
          >
            <Play className="w-3.5 h-3.5" />
            <span>{testingCategory === 'tts' ? 'Synthesizing...' : 'Test Speech Synthesis'}</span>
          </button>

          {testResult?.category === 'tts' && (
            <span
              className={`text-xs font-mono flex items-center gap-1 ${
                testResult.res.success ? 'text-emerald-400' : 'text-red-400'
              }`}
            >
              {testResult.res.success ? (
                <CheckCircle className="w-3.5 h-3.5" />
              ) : (
                <AlertCircle className="w-3.5 h-3.5" />
              )}
              <span>{testResult.res.message}</span>
            </span>
          )}
        </div>
      </div>
    )}

    {/* Voice Profiles Library Manager */}
    <div className="pt-4 border-t border-stone-800/80 space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Users className="w-4 h-4 text-purple-400" />
          <span className="font-sans text-xs uppercase font-bold text-stone-200">NPC Voice Profiles Library</span>
          <span className="text-xs font-mono text-stone-500">({entry.voice_profiles?.length || 0} archetypes)</span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {isKokoro && (
            <button
              onClick={() => {
                onChange({
                  ...entry,
                  voice_profiles: [...KOKORO_VOICE_PROFILES],
                });
              }}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded bg-purple-600/20 border border-purple-500/40 text-purple-300 hover:bg-purple-600/30 transition cursor-pointer"
              title="Autofill all 11 Kokoro voice profiles with gender and accent tags"
            >
              <Sparkles className="w-3 h-3" />
              <span>Load Kokoro Voices (11 Profiles)</span>
            </button>
          )}

          <button
            onClick={() => {
              onChange({
                ...entry,
                voice_profiles: [...DEFAULT_VOICE_PROFILES],
              });
            }}
            className="flex items-center gap-1 text-xs px-2 py-1 rounded bg-stone-900 border border-stone-700 text-stone-300 hover:text-purple-300 transition cursor-pointer"
            title="Restore default fantasy archetypes"
          >
            <RotateCcw className="w-3 h-3" />
            <span>Load Fantasy Defaults</span>
          </button>

          {(Boolean(inspect?.catalog?.voices?.length) || isGeminiTTS) && (
            <button
              type="button"
              onClick={() => onOpenVoiceCatalog()}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded bg-stone-900 border border-purple-500/40 text-purple-300 hover:bg-stone-800 transition cursor-pointer"
            >
              <Plus className="w-3 h-3" />
              <span>Import from Catalog</span>
            </button>
          )}

          <button
            onClick={() => {
              const currentProfiles = entry.voice_profiles || [];
              const newProfile: VoiceProfile = {
                id: `npc_voice_${currentProfiles.length + 1}`,
                name: 'New Archetype',
                voice_id: entry.default_voice || 'default',
                pitch: 1.0,
                speech_rate: 1.0,
                tags: ['npc'],
                description: 'Distinctive voice description for automatic GM matching.',
              };
              onChange({
                ...entry,
                voice_profiles: [...currentProfiles, newProfile],
              });
            }}
            className="flex items-center gap-1 text-xs px-2 py-1 rounded bg-purple-600/20 border border-purple-500/40 text-purple-300 hover:bg-purple-600/30 transition cursor-pointer"
          >
            <Plus className="w-3 h-3" />
            <span>Add Profile</span>
          </button>
        </div>
      </div>

      <p className="text-xs text-stone-400">
        The GM and world extractor match NPC descriptions against these voice archetypes and tags to assign unique
        speech parameters automatically.
      </p>

      <div className="space-y-2 max-h-72 overflow-y-auto pr-1">
        {(entry.voice_profiles || []).map((profile, idx) => (
          <div key={idx} className="p-3 rounded-lg bg-stone-950/70 border border-stone-800/80 space-y-2.5">
            <div className="flex items-center justify-between gap-2">
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 flex-1">
                <input
                  type="text"
                  placeholder="ID (e.g. elder_sage)"
                  value={profile.id}
                  onChange={(e) => {
                    const updated = [...(entry.voice_profiles || [])];
                    updated[idx] = { ...updated[idx], id: e.target.value };
                    onChange({ ...entry, voice_profiles: updated });
                  }}
                  className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs font-mono text-purple-300 focus:outline-none"
                />
                <input
                  type="text"
                  placeholder="Display Name"
                  value={profile.name}
                  onChange={(e) => {
                    const updated = [...(entry.voice_profiles || [])];
                    updated[idx] = { ...updated[idx], name: e.target.value };
                    onChange({ ...entry, voice_profiles: updated });
                  }}
                  className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
                />
                <VoiceCombobox
                  value={profile.voice_id}
                  onChange={(voiceID) => {
                    const updated = [...(entry.voice_profiles || [])];
                    updated[idx] = { ...updated[idx], voice_id: voiceID };
                    onChange({ ...entry, voice_profiles: updated });
                  }}
                  voices={voices}
                  placeholder="Voice ID (e.g. af_bella)"
                />
              </div>

              <div className="flex items-center gap-1.5">
                <button
                  onClick={() =>
                    onTest(
                      'tts',
                      {
                        ...entry,
                        default_voice: profile.voice_id,
                        pitch: profile.pitch,
                        speech_rate: profile.speech_rate,
                        options: profile.options,
                      },
                      previewText,
                    )
                  }
                  className="p-1.5 rounded bg-stone-900 border border-stone-800 text-purple-400 hover:text-purple-300 hover:border-purple-500/40 cursor-pointer"
                  aria-label="Test Voice Profile"
                  title="Test Voice Profile"
                >
                  <Play className="w-3.5 h-3.5" />
                </button>
                <button
                  onClick={() => {
                    const updated = (entry.voice_profiles || []).filter((_, i) => i !== idx);
                    onChange({ ...entry, voice_profiles: updated });
                  }}
                  className="p-1.5 rounded bg-stone-900 border border-stone-800 text-stone-500 hover:text-red-400 hover:border-red-500/40 cursor-pointer"
                  aria-label="Delete Profile"
                  title="Delete Profile"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
              <div className="space-y-1">
                <div className="flex justify-between text-xs text-stone-400">
                  <span>Pitch</span>
                  <span className="font-mono text-purple-400">{(profile.pitch ?? 1.0).toFixed(2)}x</span>
                </div>
                <input
                  type="range"
                  min="0.5"
                  max="1.5"
                  step="0.05"
                  value={profile.pitch ?? 1.0}
                  onChange={(e) => {
                    const updated = [...(entry.voice_profiles || [])];
                    updated[idx] = { ...updated[idx], pitch: parseFloat(e.target.value) };
                    onChange({ ...entry, voice_profiles: updated });
                  }}
                  className="w-full accent-purple-500"
                />
              </div>

              <div className="space-y-1">
                <div className="flex justify-between text-xs text-stone-400">
                  <span>Speed / Speech Rate</span>
                  <span className="font-mono text-purple-400">{(profile.speech_rate ?? 1.0).toFixed(2)}x</span>
                </div>
                <input
                  type="range"
                  min="0.5"
                  max="1.5"
                  step="0.05"
                  value={profile.speech_rate ?? 1.0}
                  onChange={(e) => {
                    const updated = [...(entry.voice_profiles || [])];
                    updated[idx] = { ...updated[idx], speech_rate: parseFloat(e.target.value) };
                    onChange({ ...entry, voice_profiles: updated });
                  }}
                  className="w-full accent-purple-500"
                />
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs">
              <input
                type="text"
                placeholder="Tags (comma-separated, e.g. elder, male, wise)"
                value={profile.tags?.join(', ') || ''}
                onChange={(e) => {
                  const tags = e.target.value
                    .split(',')
                    .map((s) => s.trim())
                    .filter(Boolean);
                  const updated = [...(entry.voice_profiles || [])];
                  updated[idx] = { ...updated[idx], tags };
                  onChange({ ...entry, voice_profiles: updated });
                }}
                className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-300 focus:outline-none"
              />
              <input
                type="text"
                placeholder="Description (e.g. Ancient wizards, village elders)"
                value={profile.description || ''}
                onChange={(e) => {
                  const updated = [...(entry.voice_profiles || [])];
                  updated[idx] = { ...updated[idx], description: e.target.value };
                  onChange({ ...entry, voice_profiles: updated });
                }}
                className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-300 focus:outline-none"
              />
            </div>

            {inspect && inspect.options && inspect.options.length > 0 && (
              <details className="text-xs">
                <summary className="cursor-pointer text-xs font-sans uppercase text-stone-400">
                  Provider Options
                </summary>
                <div className="pt-2">
                  <VoiceOptionsControl
                    schema={inspect.options}
                    values={profile.options ?? {}}
                    onChange={(key, value) => {
                      const updated = [...(entry.voice_profiles || [])];
                      updated[idx] = {
                        ...updated[idx],
                        options: { ...(updated[idx].options ?? {}), [key]: value },
                      };
                      onChange({ ...entry, voice_profiles: updated });
                    }}
                  />
                </div>
              </details>
            )}
          </div>
        ))}

        {(!entry.voice_profiles || entry.voice_profiles.length === 0) && (
          <div className="p-3 text-center text-xs text-stone-500 border border-dashed border-stone-800 rounded-lg">
            No voice profiles defined yet. Click "Load Fantasy Defaults" to initialize standard archetypes.
          </div>
        )}
      </div>
    </div>
  </div>
);

export default TTSProviderEditor;
