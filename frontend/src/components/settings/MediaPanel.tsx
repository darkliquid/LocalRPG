import React, { useMemo, useState } from 'react';
import {
  AlertCircle,
  CheckCircle,
  Mic,
  Play,
  Plus,
  RotateCcw,
  Sparkles,
  Trash2,
  Users,
  Volume2,
  Zap,
} from 'lucide-react';
import type {
  AppConfig,
  ImageConfig,
  ModelStatus,
  ProviderVoice,
  STTConfig,
  TestProviderResponse,
  TTSConfig,
  VoiceProfile,
} from '../../types';
import { APIClient } from '../../api/client';
import { ProviderManager } from '../ProviderManager';
import { VoiceCombobox } from '../VoiceCombobox';
import { VoiceOptionsControl } from '../VoiceOptionsControl';
import { VoiceCatalogModal } from '../VoiceCatalogModal';
import { SubTabs } from '../ui/SubTabs';
import { tierLabel } from '../providers/TierBadge';
import { DEFAULT_VOICE_PROFILES, KOKORO_VOICE_PROFILES } from '../../lib/voiceProfiles';
import { hasWebSpeechSupport } from '../../lib/webSpeech';
import { mediaEntryValue, setMediaEntry } from '../../lib/mediaProviders';
import { useTTSInspect } from '../../hooks/useTTSInspect';

// Long enough to expose cadence, pitch, and pacing differences between voice
// profiles. Mirrors defaultTTSPreviewText on the server.
const DEFAULT_TTS_PREVIEW_TEXT =
  'Local RPG can use a wide range of voices to bring life to your characters, NPCs, and story narration.';

type PresetMap<T> = Record<string, { label: string; description: string; config: T; tier?: string; caveat?: string }>;

interface MediaPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
  ttsPresets: PresetMap<TTSConfig>;
  sttPresets: PresetMap<STTConfig>;
  imagePresets: PresetMap<ImageConfig>;
  models: ModelStatus[];
  handleTestProvider: (category: 'llm' | 'tts' | 'stt' | 'image', provider: unknown, testPrompt?: string) => void;
  testingCategory: string | null;
  testResult: { category: string; res: TestProviderResponse } | null;
  setMissingModelPrompt: (prompt: { id: string; name: string; sizeBytes: number } | null) => void;
}

// MediaPanel owns the three media engines and the voice catalogue. Which engine
// is on screen, the selected provider entry, and the inspected voice list are
// all its own state, so the studio shell carries none of it.
export const MediaPanel: React.FC<MediaPanelProps> = ({
  config,
  setConfig,
  ttsPresets,
  sttPresets,
  imagePresets,
  models,
  handleTestProvider,
  testingCategory,
  testResult,
  setMissingModelPrompt,
}) => {
  const [mediaTab, setMediaTab] = useState<'tts' | 'stt' | 'images'>('tts');
  // Web Speech runs in the browser, not the backend, so its availability is a
  // property of this window rather than of the provider catalogue.
  const webSpeechAvailable = hasWebSpeechSupport();
  const [ttsPreviewText, setTtsPreviewText] = useState<string>(DEFAULT_TTS_PREVIEW_TEXT);
  const [isCatalogModalOpen, setIsCatalogModalOpen] = useState(false);

  // The selected entry of each media family: "default" is the singleton, a name
  // is a map entry. The manager edits whichever entry is selected.
  const [selectedTts, setSelectedTts] = useState<string>('default');
  const [selectedStt, setSelectedStt] = useState<string>('default');
  const [selectedImage, setSelectedImage] = useState<string>('default');

  // The inspect describes the configuration in hand, so a provider is described
  // before it is saved. The fallback keeps the hook unconditional during load.
  const activeTtsConfig = config ? (mediaEntryValue(config, 'tts', selectedTts) as TTSConfig) : undefined;
  const inspectConfig = activeTtsConfig ?? { type: 'disabled' as const, auto_play: false, master_volume: 1 };
  const { inspect, loading: inspecting, refresh: refreshInspect, error: inspectError } = useTTSInspect(
    inspectConfig,
    Boolean(config)
  );

  const isBuiltinKokoro =
    activeTtsConfig?.type === 'builtin' &&
    (activeTtsConfig?.builtin_name === 'sherpa-onnx' || activeTtsConfig?.builtin_name === 'kokoro');
  const isKokoro =
    Boolean(isBuiltinKokoro) ||
    (activeTtsConfig?.type === 'http' &&
      (activeTtsConfig?.model === 'kokoro' || (activeTtsConfig?.endpoint || '').includes('8880')));

  const kokoroCatalogVoices: ProviderVoice[] = useMemo(
    () =>
      KOKORO_VOICE_PROFILES.map((p) => ({
        id: p.voice_id,
        name: p.name,
        tags: p.tags,
        description: p.description,
      })),
    []
  );

  const availableTTSVoices = useMemo(() => {
    if (inspect?.catalog?.voices && inspect.catalog.voices.length > 0) {
      return inspect.catalog.voices;
    }
    if (isKokoro) {
      return kokoroCatalogVoices;
    }
    return [];
  }, [inspect?.catalog?.voices, isKokoro, kokoroCatalogVoices]);

  const kokoroStatus = models.find((m) => m.id === 'kokoro-tts');
  const isGeminiTTS =
    activeTtsConfig?.type === 'gemini' ||
    (activeTtsConfig?.type === 'builtin' && activeTtsConfig?.builtin_name === 'gemini');
  const isElevenLabsTTS =
    activeTtsConfig?.type === 'builtin' && activeTtsConfig?.builtin_name === 'elevenlabs';
  const isCartesiaTTS =
    activeTtsConfig?.type === 'cartesia' ||
    (activeTtsConfig?.type === 'builtin' && activeTtsConfig?.builtin_name === 'cartesia');

  return (
    <>
  <div className="space-y-4 flex-1 overflow-y-auto pr-1">
    <SubTabs
      ariaLabel="Media Engines"
      tabs={[
        { id: 'tts', label: 'TTS', icon: <Volume2 className="w-3.5 h-3.5" /> },
        { id: 'stt', label: 'STT', icon: <Mic className="w-3.5 h-3.5" /> },
        { id: 'images', label: 'Images', icon: <Sparkles className="w-3.5 h-3.5" /> },
      ]}
      active={mediaTab}
      onChange={(id) => setMediaTab(id as 'tts' | 'stt' | 'images')}
    />

    {mediaTab === 'tts' && (
    <ProviderManager
      family="tts"
      config={config}
      onChange={setConfig}
      selected={selectedTts}
      onSelect={setSelectedTts}
      renderEditor={(_name, entry, onChange) => {
        const ttsEntry = entry as TTSConfig;
        return (
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
                    if (key && ttsPresets[key]) {
                      const preset = ttsPresets[key].config;
                      onChange({
                        ...preset,
                        builtin_name: preset.builtin_name,
                        options: preset.options,
                        endpoint: preset.endpoint,
                        model: preset.model,
                        api_key: preset.api_key,
                        auto_play: ttsEntry.auto_play,
                        voice_profiles: preset.voice_profiles ?? ttsEntry.voice_profiles,
                      });
                      e.target.value = '';
                    }
                  }}
                  className="bg-stone-900 border border-purple-500/30 text-purple-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
                  defaultValue=""
                >
                  <option value="" disabled>⚡ Load TTS Preset…</option>
                  {Object.entries(ttsPresets).map(([id, p]) => (
                    <option key={id} value={id} title={p.caveat}>
                      {p.label}{p.tier ? ` · ${tierLabel(p.tier)}` : ''}
                    </option>
                  ))}
                </select>

                <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={ttsEntry.auto_play}
                    onChange={(e) =>
                      onChange({ ...ttsEntry, auto_play: e.target.checked })
                    }
                    className="rounded bg-stone-950 border-stone-800 text-purple-600 focus:ring-0"
                  />
                  <span>Auto-play Narration</span>
                </label>
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-sans uppercase text-stone-300">Narration Markdown</label>
              <select
                value={ttsEntry.markdown || 'auto'}
                onChange={(e) =>
                  onChange({ ...ttsEntry, markdown: e.target.value as 'auto' | 'strip' | 'keep' })
                }
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
                    ttsEntry.type === 'gemini' ||
                    (ttsEntry.type === 'builtin' && ttsEntry.builtin_name === 'gemini')
                      ? 'gemini'
                      : ttsEntry.type === 'cartesia' ||
                        (ttsEntry.type === 'builtin' && ttsEntry.builtin_name === 'cartesia')
                      ? 'builtin:cartesia'
                      : ttsEntry.type === 'builtin'
                      ? `builtin:${ttsEntry.builtin_name || 'native-os'}`
                      : ttsEntry.type === 'fish-audio' ||
                        (ttsEntry.type === 'http' &&
                          (ttsEntry.model?.includes('fish') || ttsEntry.model?.includes('s2-pro')))
                      ? 'fish-audio'
                      : ttsEntry.type
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
                              model:
                                ttsEntry.model && ttsEntry.model.includes('eleven')
                                  ? ttsEntry.model
                                  : 'eleven_multilingual_v2',
                              default_voice:
                                ttsEntry.default_voice &&
                                ttsEntry.default_voice.startsWith('EXAV')
                                  ? ttsEntry.default_voice
                                  : 'EXAVITQu4vr4xnSDxMaL',
                            }
                          : builtinName === 'cartesia'
                          ? {
                              model:
                                ttsEntry.model && ttsEntry.model.includes('sonic')
                                  ? ttsEntry.model
                                  : 'sonic-3.6',
                              default_voice:
                                ttsEntry.default_voice || 'db6b0ed5-d5d3-463d-ae85-518a07d3c2b4',
                            }
                          : builtinName === 'sherpa-onnx'
                          ? {
                              default_voice:
                                ttsEntry.default_voice &&
                                ttsEntry.default_voice.startsWith('af_')
                                  ? ttsEntry.default_voice
                                  : 'af_bella',
                            }
                          : {};
                      onChange({
                        ...ttsEntry,
                        ...builtinDefaults,
                        type: 'builtin',
                        builtin_name: builtinName,
                        options: builtinName === 'elevenlabs' ? ttsEntry.options : undefined,
                      });
                    } else if (val === 'gemini') {
                      onChange({
                        ...ttsEntry,
                        type: 'gemini',
                        builtin_name: undefined,
                        model:
                          ttsEntry.model && ttsEntry.model.includes('gemini')
                            ? ttsEntry.model
                            : 'gemini-3.8-flash-tts',
                        default_voice:
                          ttsEntry.default_voice &&
                          !ttsEntry.default_voice.startsWith('EXAV') &&
                          !ttsEntry.default_voice.startsWith('af_')
                            ? ttsEntry.default_voice
                            : 'Aoede',
                        options: undefined,
                      });
                    } else if (val === 'fish-audio') {
                      onChange({
                        ...ttsEntry,
                        type: 'http',
                        builtin_name: undefined,
                        endpoint:
                          ttsEntry.endpoint && ttsEntry.endpoint.includes('8091')
                            ? ttsEntry.endpoint
                            : 'http://localhost:8091',
                        model: 'fishaudio/s2-pro',
                        default_voice: ttsEntry.default_voice || 'default',
                        options: undefined,
                      });
                    } else if (val === 'http') {
                      onChange({
                        ...ttsEntry,
                        type: 'http',
                        builtin_name: undefined,
                        endpoint: ttsEntry.endpoint || 'http://localhost:8880/v1/audio/speech',
                        model:
                          ttsEntry.model &&
                          !ttsEntry.model.includes('eleven') &&
                          !ttsEntry.model.includes('gemini') &&
                          !ttsEntry.model.includes('fish') &&
                          !ttsEntry.model.includes('s2-pro')
                            ? ttsEntry.model
                            : 'kokoro',
                        default_voice:
                          ttsEntry.default_voice &&
                          !ttsEntry.default_voice.startsWith('EXAV') &&
                          ttsEntry.default_voice !== 'Aoede'
                            ? ttsEntry.default_voice
                            : 'af_bella',
                        options: undefined,
                      });
                    } else {
                      onChange({ ...ttsEntry, type: val as any, builtin_name: undefined, options: undefined });
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

              {ttsEntry.type === 'http' && (
                <>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">Endpoint URL</label>
                    <input
                      type="text"
                      placeholder="e.g. http://localhost:8880"
                      value={ttsEntry.endpoint || ''}
                      onChange={(e) =>
                        onChange({ ...ttsEntry, endpoint: e.target.value })
                      }
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
                      value={ttsEntry.model || ''}
                      onChange={(e) =>
                        onChange({ ...ttsEntry, model: e.target.value })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">API Key (Optional)</label>
                    <input
                      type="password"
                      placeholder="Optional authorization token (e.g. for OpenAI)"
                      value={ttsEntry.api_key || ''}
                      onChange={(e) =>
                        onChange({ ...ttsEntry, api_key: e.target.value })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                </>
              )}

              {ttsEntry.type === 'cli' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Command / Binary</label>
                  <input
                    type="text"
                    placeholder="e.g. piper"
                    value={ttsEntry.command || ''}
                    onChange={(e) =>
                      onChange({ ...ttsEntry, command: e.target.value })
                    }
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                  />
                </div>
              )}

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Master Volume</span>
                  <span className="font-mono text-purple-400">
                    {Math.round((ttsEntry.master_volume || 1.0) * 100)}%
                  </span>
                </label>
                <input
                  type="range"
                  min="0"
                  max="1"
                  step="0.05"
                  value={ttsEntry.master_volume || 1.0}
                  onChange={(e) =>
                    onChange({ ...ttsEntry, master_volume: parseFloat(e.target.value) })
                  }
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
                      <div className="w-20 h-1.5 bg-stone-800 rounded-full overflow-hidden">
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
                      setMissingModelPrompt({
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
                  value={ttsEntry.model || 'gemini-3.8-flash-tts'}
                  onChange={(e) =>
                    onChange({ ...ttsEntry, model: e.target.value })
                  }
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
                      onClick={() =>
                        onChange({ ...ttsEntry, model: m.id })
                      }
                      className={`text-xs px-2.5 py-1 rounded-lg border font-mono transition cursor-pointer ${
                        (ttsEntry.model || 'gemini-3.8-flash-tts') === m.id
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
                  value={ttsEntry.model || 'sonic-3.6'}
                  onChange={(e) =>
                    onChange({ ...ttsEntry, model: e.target.value })
                  }
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
                      onClick={() =>
                        onChange({ ...ttsEntry, model: m.id })
                      }
                      className={`text-xs px-2.5 py-1 rounded-lg border font-mono transition cursor-pointer ${
                        (ttsEntry.model || 'sonic-3.6') === m.id
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
                  {isGeminiTTS && config.providers?.gemini?.api_key && !ttsEntry.api_key && (
                    <span className="text-xs text-emerald-400 font-mono">Using shared Gemini key</span>
                  )}
                  {isCartesiaTTS && config.providers?.cartesia?.api_key && !ttsEntry.api_key && (
                    <span className="text-xs text-emerald-400 font-mono">Using shared Cartesia key</span>
                  )}
                </label>
                <input
                  type="password"
                  placeholder={
                    isGeminiTTS && config.providers?.gemini?.api_key
                      ? 'Using shared key from providers.gemini.api_key'
                      : isCartesiaTTS && config.providers?.cartesia?.api_key
                      ? 'Using shared key from providers.cartesia.api_key'
                      : "Leave empty to use the provider's environment variable"
                  }
                  value={ttsEntry.api_key || ''}
                  onChange={(e) =>
                    onChange({ ...ttsEntry, api_key: e.target.value })
                  }
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
                  values={ttsEntry.options ?? {}}
                  onChange={(key, value) =>
                    onChange({ ...ttsEntry, options: { ...(ttsEntry.options ?? {}), [key]: value } })
                  }
                />
              </div>
            )}

            {inspect?.catalog?.available && (
              <div className="flex flex-wrap items-center justify-between gap-2 text-xs font-mono text-stone-400">
                <span>
                  {inspect.catalog.voices?.length ?? 0} voices
                  {inspect.catalog.fetched_at
                    ? ` - last checked ${new Date(inspect.catalog.fetched_at).toLocaleString()}`
                    : ''}
                  {inspect.catalog.stale ? ' (catalog unavailable, showing the last copy)' : ''}
                </span>
                <button
                  type="button"
                  onClick={refreshInspect}
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

            {ttsEntry.type !== 'disabled' && (
              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300">Default Voice</label>
                <VoiceCombobox
                  value={ttsEntry.default_voice || ''}
                  onChange={(voiceID) =>
                    onChange({ ...ttsEntry, default_voice: voiceID })
                  }
                  voices={availableTTSVoices}
                  placeholder="Select default provider voice..."
                />
                <p className="text-xs text-stone-500">
                  Fallback voice used for turn narration and unvoiced characters.
                </p>
              </div>
            )}

            {ttsEntry.type !== 'disabled' && (
              <div className="space-y-3 p-4 rounded-xl bg-stone-900/40 border border-stone-800">
                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <div className="text-xs font-sans font-bold text-stone-200">
                      Speech Steering & Acting Cues
                    </div>
                    <div className="text-xs text-stone-400">
                      Instruct the GM to use emotive directions (e.g. [whispers], [sighs]) when supported.
                    </div>
                  </div>
                  <input
                    type="checkbox"
                    checked={ttsEntry.speech_cues?.enabled ?? true}
                    onChange={(e) =>
                      onChange({
                        ...ttsEntry,
                        speech_cues: {
                          ...ttsEntry.speech_cues,
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
                        Supports bracketed vocal cues ({inspect.speech_cues.supported_tags?.slice(0, 5).map(t => `[${t}]`).join(', ')}...)
                      </span>
                    ) : inspect.speech_cues.markdown_emphasis ? (
                      <span className="text-stone-300">Supports Markdown emphasis (*emphasis*)</span>
                    ) : (
                      <span className="text-stone-500">Plain text only; vocal tags are stripped before synthesis.</span>
                    )}
                  </div>
                )}

                {(ttsEntry.speech_cues?.enabled ?? true) && (
                  <div className="space-y-1.5 pt-1">
                    <label className="text-xs font-sans uppercase text-stone-300">
                      Transcript Display Mode
                    </label>
                    <select
                      value={ttsEntry.speech_cues?.display_mode || 'stage_directions'}
                      onChange={(e) =>
                        onChange({
                          ...ttsEntry,
                          speech_cues: {
                            ...ttsEntry.speech_cues,
                            enabled: ttsEntry.speech_cues?.enabled ?? true,
                            display_mode: e.target.value as any,
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

            {ttsEntry.type !== 'disabled' && (
              <div className="pt-2 space-y-2 border-t border-stone-800/60">
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">
                    Preview Phrase
                  </label>
                  <input
                    type="text"
                    value={ttsPreviewText}
                    onChange={(e) => setTtsPreviewText(e.target.value)}
                    placeholder={DEFAULT_TTS_PREVIEW_TEXT}
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60"
                  />
                </div>

                <div className="flex items-center justify-between">
                  <button
                    onClick={() => handleTestProvider('tts', ttsEntry, ttsPreviewText)}
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
                      {testResult.res.success ? <CheckCircle className="w-3.5 h-3.5" /> : <AlertCircle className="w-3.5 h-3.5" />}
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
                  <span className="font-sans text-xs uppercase font-bold text-stone-200">
                    NPC Voice Profiles Library
                  </span>
                  <span className="text-xs font-mono text-stone-500">
                    ({ttsEntry.voice_profiles?.length || 0} archetypes)
                  </span>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {isKokoro && (
                    <button
                      onClick={() => {
                        onChange({
                          ...ttsEntry,
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
                        ...ttsEntry,
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
                      onClick={() => setIsCatalogModalOpen(true)}
                      className="flex items-center gap-1 text-xs px-2 py-1 rounded bg-stone-900 border border-purple-500/40 text-purple-300 hover:bg-stone-800 transition cursor-pointer"
                    >
                      <Plus className="w-3 h-3" />
                      <span>Import from Catalog</span>
                    </button>
                  )}

                  <button
                    onClick={() => {
                      const currentProfiles = ttsEntry.voice_profiles || [];
                      const newProfile: VoiceProfile = {
                        id: `npc_voice_${currentProfiles.length + 1}`,
                        name: 'New Archetype',
                        voice_id: ttsEntry.default_voice || 'default',
                        pitch: 1.0,
                        speech_rate: 1.0,
                        tags: ['npc'],
                        description: 'Distinctive voice description for automatic GM matching.',
                      };
                      onChange({
                        ...ttsEntry,
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
                The GM and world extractor match NPC descriptions against these voice archetypes and tags to assign unique speech parameters automatically.
              </p>

              <div className="space-y-2 max-h-72 overflow-y-auto pr-1">
                {(ttsEntry.voice_profiles || []).map((profile, idx) => (
                  <div
                    key={idx}
                    className="p-3 rounded-lg bg-stone-950/70 border border-stone-800/80 space-y-2.5"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 flex-1">
                        <input
                          type="text"
                          placeholder="ID (e.g. elder_sage)"
                          value={profile.id}
                          onChange={(e) => {
                            const updated = [...(ttsEntry.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], id: e.target.value };
                            onChange({ ...ttsEntry, voice_profiles: updated });
                          }}
                          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs font-mono text-purple-300 focus:outline-none"
                        />
                        <input
                          type="text"
                          placeholder="Display Name"
                          value={profile.name}
                          onChange={(e) => {
                            const updated = [...(ttsEntry.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], name: e.target.value };
                            onChange({ ...ttsEntry, voice_profiles: updated });
                          }}
                          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
                        />
                        <VoiceCombobox
                          value={profile.voice_id}
                          onChange={(voiceID) => {
                            const updated = [...(ttsEntry.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], voice_id: voiceID };
                            onChange({ ...ttsEntry, voice_profiles: updated });
                          }}
                          voices={availableTTSVoices}
                          placeholder="Voice ID (e.g. af_bella)"
                        />
                      </div>

                      <div className="flex items-center gap-1.5">
                        <button
                          onClick={() =>
                            handleTestProvider(
                              'tts',
                              {
                                ...ttsEntry,
                                default_voice: profile.voice_id,
                                pitch: profile.pitch,
                                speech_rate: profile.speech_rate,
                                options: profile.options,
                              },
                              ttsPreviewText
                            )
                          }
                          className="p-1.5 rounded bg-stone-900 border border-stone-800 text-purple-400 hover:text-purple-300 hover:border-purple-500/40 cursor-pointer"
                          title="Test Voice Profile"
                        >
                          <Play className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={() => {
                            const updated = (ttsEntry.voice_profiles || []).filter((_, i) => i !== idx);
                            onChange({ ...ttsEntry, voice_profiles: updated });
                          }}
                          className="p-1.5 rounded bg-stone-900 border border-stone-800 text-stone-500 hover:text-red-400 hover:border-red-500/40 cursor-pointer"
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
                            const updated = [...(ttsEntry.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], pitch: parseFloat(e.target.value) };
                            onChange({ ...ttsEntry, voice_profiles: updated });
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
                            const updated = [...(ttsEntry.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], speech_rate: parseFloat(e.target.value) };
                            onChange({ ...ttsEntry, voice_profiles: updated });
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
                          const tags = e.target.value.split(',').map((s) => s.trim()).filter(Boolean);
                          const updated = [...(ttsEntry.voice_profiles || [])];
                          updated[idx] = { ...updated[idx], tags };
                          onChange({ ...ttsEntry, voice_profiles: updated });
                        }}
                        className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-300 focus:outline-none"
                      />
                      <input
                        type="text"
                        placeholder="Description (e.g. Ancient wizards, village elders)"
                        value={profile.description || ''}
                        onChange={(e) => {
                          const updated = [...(ttsEntry.voice_profiles || [])];
                          updated[idx] = { ...updated[idx], description: e.target.value };
                          onChange({ ...ttsEntry, voice_profiles: updated });
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
                              const updated = [...(ttsEntry.voice_profiles || [])];
                              updated[idx] = {
                                ...updated[idx],
                                options: { ...(updated[idx].options ?? {}), [key]: value },
                              };
                              onChange({ ...ttsEntry, voice_profiles: updated });
                            }}
                          />
                        </div>
                      </details>
                    )}
                  </div>
                ))}

                {(!ttsEntry.voice_profiles || ttsEntry.voice_profiles.length === 0) && (
                  <div className="p-3 text-center text-xs text-stone-500 border border-dashed border-stone-800 rounded-lg">
                    No voice profiles defined yet. Click "Load Fantasy Defaults" to initialize standard archetypes.
                  </div>
                )}
              </div>
            </div>
          </div>
        );
      }}
    />
    )}

    {mediaTab === 'stt' && (
    <ProviderManager
      family="stt"
      config={config}
      onChange={setConfig}
      selected={selectedStt}
      onSelect={setSelectedStt}
      renderEditor={(_name, entry, onChange) => {
        const sttEntry = entry as STTConfig;
        return (
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
                    if (key && sttPresets[key]) {
                      const preset = sttPresets[key].config;
                      onChange({ ...preset });
                      e.target.value = '';
                    }
                  }}
                  className="bg-stone-900 border border-purple-500/30 text-purple-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
                  defaultValue=""
                >
                  <option value="" disabled>⚡ Load STT Preset…</option>
                  {Object.entries(sttPresets).map(([id, p]) => (
                    <option key={id} value={id} title={p.caveat}>
                      {p.label}{p.tier ? ` · ${tierLabel(p.tier)}` : ''}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300">STT Provider Type</label>
                <select
                  value={
                    sttEntry?.type === 'builtin'
                      ? `builtin:${sttEntry.builtin_name || 'cartesia'}`
                      : sttEntry?.type || 'disabled'
                  }
                  onChange={(e) => {
                    const val = e.target.value;
                    if (val.startsWith('builtin:')) {
                      const builtinName = val.split(':')[1];
                      onChange({
                        ...(sttEntry || { type: 'disabled' }),
                        type: 'builtin',
                        builtin_name: builtinName,
                        model:
                          builtinName === 'cartesia'
                            ? sttEntry?.model || 'ink-whisper'
                            : sttEntry?.model,
                      });
                    } else {
                      onChange({
                        ...(sttEntry || { type: 'disabled' }),
                        type: val as any,
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

              {sttEntry?.type === 'http' && (
                <>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">Transcription Endpoint URL</label>
                    <input
                      type="text"
                      placeholder="e.g. http://localhost:8000/v1/audio/transcriptions"
                      value={sttEntry.endpoint || ''}
                      onChange={(e) =>
                        onChange({ ...sttEntry, endpoint: e.target.value })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">Model Name</label>
                    <input
                      type="text"
                      placeholder="e.g. whisper-1"
                      value={sttEntry.model || ''}
                      onChange={(e) =>
                        onChange({ ...sttEntry, model: e.target.value })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                </>
              )}

              {sttEntry?.type === 'cli' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Command / Binary</label>
                  <input
                    type="text"
                    placeholder="e.g. whisper-cli"
                    value={sttEntry.command || ''}
                    onChange={(e) =>
                      onChange({ ...sttEntry, command: e.target.value })
                    }
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                  />
                </div>
              )}

              {(sttEntry?.type === 'cartesia' ||
                (sttEntry?.type === 'builtin' && sttEntry?.builtin_name === 'cartesia')) && (
                <>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">Model Name</label>
                    <input
                      type="text"
                      placeholder="e.g. ink-whisper"
                      value={sttEntry?.model || 'ink-whisper'}
                      onChange={(e) =>
                        onChange({ ...sttEntry, model: e.target.value })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                      <span>API Key Override</span>
                      {config.providers?.cartesia?.api_key && !sttEntry?.api_key && (
                        <span className="text-xs text-emerald-400 font-mono">Using shared Cartesia key</span>
                      )}
                    </label>
                    <input
                      type="password"
                      placeholder={
                        config.providers?.cartesia?.api_key
                          ? 'Using shared key from providers.cartesia.api_key'
                          : 'Optional override or CARTESIA_API_KEY env'
                      }
                      value={sttEntry?.api_key || ''}
                      onChange={(e) =>
                        onChange({ ...sttEntry, api_key: e.target.value })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                </>
              )}
            </div>

            {sttEntry?.type && sttEntry.type !== 'disabled' && (
              <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
                <button
                  onClick={() => handleTestProvider('stt', sttEntry)}
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
      }}
    />
    )}

    {mediaTab === 'images' && (
    <ProviderManager
      family="image"
      config={config}
      onChange={setConfig}
      selected={selectedImage}
      onSelect={setSelectedImage}
      renderEditor={(_name, entry, onChange) => {
        const imageEntry = entry as ImageConfig;
        return (
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
                    if (key && imagePresets[key]) {
                      const preset = imagePresets[key].config;
                      onChange({ ...preset, auto_generate: imageEntry.auto_generate });
                      e.target.value = '';
                    }
                  }}
                  className="bg-stone-900 border border-purple-500/30 text-purple-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
                  defaultValue=""
                >
                  <option value="" disabled>⚡ Load Image Preset…</option>
                  {Object.entries(imagePresets).map(([id, p]) => (
                    <option key={id} value={id} title={p.caveat}>
                      {p.label}{p.tier ? ` · ${tierLabel(p.tier)}` : ''}
                    </option>
                  ))}
                </select>

                <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={imageEntry.auto_generate}
                    onChange={(e) =>
                      onChange({ ...imageEntry, auto_generate: e.target.checked })
                    }
                    className="rounded bg-stone-950 border-stone-800 text-purple-600 focus:ring-0"
                  />
                  <span>Auto-generate Scene Art</span>
                </label>

                <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={imageEntry.builtin_fallback !== false}
                    onChange={(e) =>
                      onChange({ ...imageEntry, builtin_fallback: e.target.checked })
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
                  value={imageEntry.type}
                  onChange={(e) =>
                    onChange({ ...imageEntry, type: e.target.value as any })
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

              {imageEntry.type === 'builtin' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Built-in Art Engine</label>
                  <select
                    value={imageEntry.builtin_name || 'procedural-art'}
                    onChange={(e) =>
                      onChange({ ...imageEntry, builtin_name: e.target.value })
                    }
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
                  >
                    <option value="procedural-art">procedural-art (Pure-Go Vector Dark Fantasy SVG)</option>
                    <option value="echo">echo (Debug Mock)</option>
                  </select>
                </div>
              )}

              {imageEntry.type === 'http' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Image Endpoint URL</label>
                  <input
                    type="text"
                    placeholder="e.g. http://localhost:7860/v1/images/generations"
                    value={imageEntry.endpoint || ''}
                    onChange={(e) =>
                      onChange({ ...imageEntry, endpoint: e.target.value })
                    }
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                  />
                </div>
              )}

              {(imageEntry.type === 'gemini' ||
                (imageEntry.type === 'builtin' && imageEntry.builtin_name === 'gemini')) && (
                <>
                  <div className="space-y-1.5 col-span-1 md:col-span-2">
                    <label className="text-xs font-sans uppercase text-stone-300">Gemini / Imagen Model</label>
                    <input
                      type="text"
                      placeholder="e.g. imagen-3.0-generate-002 or gemini-3.1-flash-image"
                      value={imageEntry.model || 'imagen-3.0-generate-002'}
                      onChange={(e) =>
                        onChange({ ...imageEntry, model: e.target.value })
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
                            onChange({ ...imageEntry, model: m.id })
                          }
                          className={`px-2 py-0.5 text-xs font-mono rounded border transition-colors cursor-pointer ${
                            (imageEntry.model || 'imagen-3.0-generate-002') === m.id
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
                      value={imageEntry.aspect_ratio || '16:9'}
                      onChange={(e) =>
                        onChange({ ...imageEntry, aspect_ratio: e.target.value })
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
                      value={imageEntry.person_generation || 'ALLOW_ADULT'}
                      onChange={(e) =>
                        onChange({ ...imageEntry, person_generation: e.target.value })
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
                      {config.providers?.gemini?.api_key && (
                        <span className="text-xs text-emerald-400 font-mono">Shared key active</span>
                      )}
                    </label>
                    <input
                      type="password"
                      placeholder={config.providers?.gemini?.api_key ? 'Using shared key (leave blank)' : 'Optional override or GEMINI_API_KEY env'}
                      value={imageEntry.api_key || ''}
                      onChange={(e) =>
                        onChange({ ...imageEntry, api_key: e.target.value })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                </>
              )}
            </div>

            {imageEntry.type !== 'disabled' && (
              <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
                <button
                  onClick={() => handleTestProvider('image', imageEntry)}
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
      }}
    />
    )}
  </div>

      <VoiceCatalogModal
        isOpen={isCatalogModalOpen}
        onClose={() => setIsCatalogModalOpen(false)}
        voices={inspect?.catalog?.voices ?? []}
        providerKey={inspect?.provider_key || 'tts'}
        onSearch={
          isGeminiTTS
            ? async (query) => {
                const res = await APIClient.searchTTSVoices({
                  config: mediaEntryValue(config, 'tts', selectedTts) as TTSConfig,
                  query,
                });
                if (res.error) throw new Error(res.error);
                return res.voices;
              }
            : undefined
        }
        searchLabel="Search the Gemini extended voice library"
        onAddProfile={(newProfile) => {
          const entry = mediaEntryValue(config, 'tts', selectedTts) as TTSConfig;
          const current = entry.voice_profiles || [];
          setConfig(setMediaEntry(config, 'tts', selectedTts, { ...entry, voice_profiles: [...current, newProfile] }));
        }}
      />
    </>
  );
};

export default MediaPanel;
