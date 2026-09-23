import React, { useState, useEffect, useRef, useMemo } from 'react';
import { DebugPanel } from './DebugPanel';
import { APIClient } from '../api/client';
import { AppConfig, AgentRoleConfig, TestProviderResponse, VoiceProfile, ModelStatus, ProviderVoice } from '../types';
import { ModelDownloadModal } from './ModelDownloadModal';
import {
  Folder,
  Cpu,
  Volume2,
  Sliders,
  Save,
  CheckCircle,
  AlertCircle,
  Play,
  Sparkles,
  Zap,
  Mic,
  Plus,
  Trash2,
  Users,
  RotateCcw,
  Bug,
  Cloud,
} from 'lucide-react';
import {
  AGENT_PRESETS,
  TTS_PRESETS,
  STT_PRESETS,
  IMAGE_PRESETS,
  DEFAULT_VOICE_PROFILES,
  KOKORO_VOICE_PROFILES,
} from '../templates/providerPresets';
import { VoiceOptionsControl } from './VoiceOptionsControl';
import { useTTSInspect } from '../hooks/useTTSInspect';
import { VoiceCombobox } from './VoiceCombobox';
import { VoiceCatalogModal } from './VoiceCatalogModal';

interface SettingsStudioProps {
  isCompact?: boolean;
  onSaved?: () => void;
}

const ROLE_LABELS: Record<string, string> = {
  gm: 'Game Master (GM / Storyteller)',
  narrator: 'Atmospheric Narrator',
  extractor: 'Entity Extractor (per-turn world state)',
};

// Long enough to expose cadence, pitch, and pacing differences between voice
// profiles. Mirrors defaultTTSPreviewText on the server.
const DEFAULT_TTS_PREVIEW_TEXT =
  'Local RPG can use a wide range of voices to bring life to your characters, NPCs, and story narration.';

// A missing role falls back to inheriting gm for the extractor, which is what
// makes extraction work out of the box without a second configuration step.
const defaultRoleConfig = (role: string): AgentRoleConfig =>
  role === 'extractor' ? { type: 'inherit', inherit_from: 'gm' } : { type: 'disabled' };

export const SettingsStudio: React.FC<SettingsStudioProps> = ({ isCompact, onSaved }) => {
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [activeFilePath, setActiveFilePath] = useState<string>('');
  const [isOverride, setIsOverride] = useState(false);
  const [activeSubTab, setActiveSubTab] = useState<'paths' | 'providers' | 'agents' | 'media' | 'preferences' | 'debug'>('paths');
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // Diagnostics test state
  const [testingCategory, setTestingCategory] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ category: string; res: TestProviderResponse } | null>(null);
  const previewAudioRef = useRef<HTMLAudioElement | null>(null);
  const [ttsPreviewText, setTtsPreviewText] = useState<string>(DEFAULT_TTS_PREVIEW_TEXT);

  // The inspect describes the configuration in hand, so a provider is described
  // before it is saved. The fallback keeps the hook unconditional during load.
  const inspectConfig = config?.media.tts ?? { type: 'disabled' as const, auto_play: false, master_volume: 1 };
  const { inspect, loading: inspecting, refresh: refreshInspect, error: inspectError } = useTTSInspect(inspectConfig, Boolean(config));

  // Selected agent role for editing
  const [selectedRole, setSelectedRole] = useState<string>('gm');

  // Model status state
  const [models, setModels] = useState<ModelStatus[]>([]);
  const [missingModelPrompt, setMissingModelPrompt] = useState<{
    id: string;
    name: string;
    sizeBytes: number;
  } | null>(null);
  const [isCatalogModalOpen, setIsCatalogModalOpen] = useState(false);

  useEffect(() => {
    loadSettings();
    APIClient.getModels().then(setModels).catch(console.error);
    const unsubscribe = APIClient.subscribeModelEvents((status) => {
      setModels((prev) => {
        const next = [...prev];
        const idx = next.findIndex((m) => m.id === status.id);
        if (idx >= 0) {
          next[idx] = status;
        } else {
          next.push(status);
        }
        return next;
      });
    });
    return () => unsubscribe();
  }, []);

  useEffect(
    () => () => {
      if (previewAudioRef.current) {
        previewAudioRef.current.pause();
        previewAudioRef.current = null;
      }
    },
    []
  );

  const playVoicePreview = (dataURI: string, volume: number) => {
    if (previewAudioRef.current) {
      previewAudioRef.current.pause();
    }
    const audio = new Audio(dataURI);
    audio.volume = Math.min(1, Math.max(0, volume));
    previewAudioRef.current = audio;
    audio.play().catch(() => {
      setFeedback({
        type: 'error',
        message: 'Audio generated, but the browser blocked playback. Click the page and retry.',
      });
    });
  };

  const loadSettings = async () => {
    setIsLoading(true);
    try {
      const res = await APIClient.getSettings();
      setConfig(res.config);
      setActiveFilePath(res.config_file_path);
      setIsOverride(res.is_local_override);
    } catch (err: any) {
      setFeedback({ type: 'error', message: err.message || 'Failed to load settings' });
    } finally {
      setIsLoading(false);
    }
  };

  const handleSave = async () => {
    if (!config) return;
    setIsSaving(true);
    setFeedback(null);
    try {
      const res = await APIClient.saveSettings(config);
      setConfig(res.config);
      setActiveFilePath(res.config_file_path);
      setIsOverride(res.is_local_override);
      setFeedback({ type: 'success', message: 'Settings saved and live-reloaded successfully.' });
      if (onSaved) onSaved();
    } catch (err: any) {
      setFeedback({ type: 'error', message: err.message || 'Failed to save settings' });
    } finally {
      setIsSaving(false);
    }
  };

  const handleTestProvider = async (category: 'llm' | 'tts' | 'stt' | 'image', provider: any, testPrompt?: string) => {
    if (category === 'tts') {
      const isTargetKokoro =
        provider?.type === 'builtin' &&
        (provider?.builtin_name === 'sherpa-onnx' || provider?.builtin_name === 'kokoro');
      const kokoroInstalled = models.find((m) => m.id === 'kokoro-tts')?.installed;
      if (isTargetKokoro && !kokoroInstalled) {
        setMissingModelPrompt({
          id: 'kokoro-tts',
          name: 'Kokoro Voice Pack',
          sizeBytes: 90177536,
        });
        return;
      }
    }

    setTestingCategory(category);
    setTestResult(null);
    try {
      const res = await APIClient.testProvider({
        category,
        provider,
        test_prompt: testPrompt ?? (category === 'llm' ? 'Are the stars shining?' : undefined),
      });
      setTestResult({ category, res });
      if (res.model_missing) {
        setMissingModelPrompt({
          id: res.model_id || 'kokoro-tts',
          name: 'Kokoro Voice Pack',
          sizeBytes: 90177536,
        });
      }
      if (category === 'tts' && res.success && res.audio_data_uri) {
        playVoicePreview(res.audio_data_uri, (provider as { master_volume?: number })?.master_volume ?? 1);
      }
    } catch (err: any) {
      setTestResult({
        category,
        res: { success: false, latency_ms: 0, message: err.message || 'Test failed' },
      });
    } finally {
      setTestingCategory(null);
    }
  };

  if (isLoading || !config) {
    return (
      <div className="p-8 text-center text-stone-400 font-mono text-sm animate-pulse">
        Loading system configuration...
      </div>
    );
  }

  const currentRoleConfig: AgentRoleConfig = config.agents.roles[selectedRole] || defaultRoleConfig(selectedRole);

  const roleNames = Array.from(new Set([...Object.keys(config.agents.roles), 'extractor']));

  const isBuiltinKokoro =
    config.media.tts.type === 'builtin' &&
    (config.media.tts.builtin_name === 'sherpa-onnx' || config.media.tts.builtin_name === 'kokoro');
  const isKokoro =
    isBuiltinKokoro ||
    (config.media.tts.type === 'http' &&
      (config.media.tts.model === 'kokoro' || (config.media.tts.endpoint || '').includes('8880')));
  const kokoroStatus = models.find((m) => m.id === 'kokoro-tts');
  const isGeminiTTS =
    config.media.tts.type === 'gemini' ||
    (config.media.tts.type === 'builtin' && config.media.tts.builtin_name === 'gemini');
  const isElevenLabsTTS =
    config.media.tts.type === 'builtin' && config.media.tts.builtin_name === 'elevenlabs';

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
    if (inspect?.catalog.voices && inspect.catalog.voices.length > 0) {
      return inspect.catalog.voices;
    }
    if (isKokoro) {
      return kokoroCatalogVoices;
    }
    return [];
  }, [inspect?.catalog.voices, isKokoro, kokoroCatalogVoices]);

  const updateRole = (updated: AgentRoleConfig) => {
    setConfig({
      ...config,
      agents: {
        ...config.agents,
        roles: { ...config.agents.roles, [selectedRole]: updated },
      },
    });
  };

  return (
    <div className={`flex flex-col h-full ${isCompact ? 'p-2 space-y-4' : 'space-y-6'}`}>
      {/* Settings Navigation & Status Header */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-800 pb-3">
        <div className="flex flex-wrap items-center gap-1 bg-stone-950/70 p-1 rounded-xl border border-stone-800">
          <button
            onClick={() => setActiveSubTab('paths')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'paths' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Folder className="w-3.5 h-3.5" />
            <span>Paths</span>
          </button>
          <button
            onClick={() => setActiveSubTab('providers')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'providers' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Cloud className="w-3.5 h-3.5" />
            <span>Providers</span>
          </button>
          <button
            onClick={() => setActiveSubTab('agents')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'agents' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Cpu className="w-3.5 h-3.5" />
            <span>AI Agents</span>
          </button>
          <button
            onClick={() => setActiveSubTab('media')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'media' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Volume2 className="w-3.5 h-3.5" />
            <span>Media Engines</span>
          </button>
          <button
            onClick={() => setActiveSubTab('preferences')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'preferences' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Sliders className="w-3.5 h-3.5" />
            <span>Preferences</span>
          </button>
          <button
            onClick={() => setActiveSubTab('debug')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
              activeSubTab === 'debug' ? 'bg-purple-600 text-white font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Bug className="w-3.5 h-3.5" />
            <span>Debug</span>
          </button>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[11px] font-mono text-stone-400 bg-stone-900 px-2.5 py-1 rounded-lg border border-stone-800">
            {isOverride ? 'Workspace Override' : 'Global User Config'}: {activeFilePath}
          </span>
          <button
            onClick={handleSave}
            disabled={isSaving}
            className="flex items-center gap-1.5 text-xs font-sans font-bold px-4 py-1.5 rounded-xl transition-all cursor-pointer bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white shadow active:scale-95"
          >
            <Save className="w-3.5 h-3.5" />
            <span>{isSaving ? 'Saving...' : 'Save Settings'}</span>
          </button>
        </div>
      </div>

      {feedback && (
        <div
          className={`p-3 rounded-xl text-xs flex items-center gap-2 ${
            feedback.type === 'success'
              ? 'bg-emerald-950/50 border border-emerald-500/40 text-emerald-200'
              : 'bg-red-950/50 border border-red-500/40 text-red-200'
          }`}
        >
          {feedback.type === 'success' ? <CheckCircle className="w-4 h-4" /> : <AlertCircle className="w-4 h-4" />}
          <span>{feedback.message}</span>
        </div>
      )}

      {/* Tab 1: Storage & Discovery Paths */}
      {activeSubTab === 'paths' && (
        <div className="space-y-4 flex-1 overflow-y-auto pr-1">
          <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
            <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
              <Folder className="w-4 h-4" />
              <span>Storage & Discovery Paths</span>
            </h3>
            <p className="text-xs text-stone-400">
              Configure directories where LocalRPG looks for rule systems, world lore, saved campaigns, and generated media caches.
            </p>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300">Rule Systems Directory</label>
                <input
                  type="text"
                  value={config.paths.systems}
                  onChange={(e) => setConfig({ ...config, paths: { ...config.paths, systems: e.target.value } })}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300">Worlds Directory</label>
                <input
                  type="text"
                  value={config.paths.worlds}
                  onChange={(e) => setConfig({ ...config, paths: { ...config.paths, worlds: e.target.value } })}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300">Saved Campaigns Directory</label>
                <input
                  type="text"
                  value={config.paths.games}
                  onChange={(e) => setConfig({ ...config, paths: { ...config.paths, games: e.target.value } })}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300">Media Cache Directory</label>
                <input
                  type="text"
                  value={config.paths.cache}
                  onChange={(e) => setConfig({ ...config, paths: { ...config.paths, cache: e.target.value } })}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Tab: Ecosystem Providers */}
      {activeSubTab === 'providers' && (
        <div className="space-y-4 flex-1 overflow-y-auto pr-1">
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
                    <p className="text-[11px] text-stone-400">Multi-modal intelligence: text reasoning, image creation, and vocal performance.</p>
                  </div>
                </div>
                <div>
                  {config.providers?.gemini?.api_key ? (
                    <span className="flex items-center gap-1 text-emerald-400 font-mono text-[11px] bg-emerald-950/40 border border-emerald-800/40 px-2 py-0.5 rounded-md">
                      <CheckCircle className="w-3.5 h-3.5" />
                      <span>Configured in Settings</span>
                    </span>
                  ) : (
                    <span className="text-[11px] font-mono text-stone-500 bg-stone-900 border border-stone-800 px-2 py-0.5 rounded-md">
                      Using env or unconfigured
                    </span>
                  )}
                </div>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Shared Gemini API Key</span>
                  <span className="text-[10px] text-stone-500 font-mono">
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
                <p className="text-[11px] text-stone-500">
                  Automatically inherited by Gemini LLM agents, Gemini/Imagen image generators, and Gemini TTS voice synthesis. Individual roles and media engines can still provide an override key.
                </p>
              </div>

              <div className="pt-2 border-t border-stone-800/50">
                <div className="text-[11px] font-sans uppercase text-stone-400 font-semibold mb-2">Connected Subsystems</div>
                <div className="flex flex-wrap gap-2">
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-[11px] text-stone-300 font-mono">
                    <Cpu className="w-3 h-3 text-purple-400" />
                    <span>AI Agents (GM, Narrator, Extractor)</span>
                  </span>
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-[11px] text-stone-300 font-mono">
                    <Sparkles className="w-3 h-3 text-purple-400" />
                    <span>Image Generation (Imagen 3, Nano Banana)</span>
                  </span>
                  <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-stone-900/80 border border-stone-800 text-[11px] text-stone-300 font-mono">
                    <Volume2 className="w-3 h-3 text-purple-400" />
                    <span>Voice Synthesis (Gemini 3.1 & 2.5 Flash/Pro TTS)</span>
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Tab 2: AI Agents & Roles */}
      {activeSubTab === 'agents' && (
        <div className="space-y-4 flex-1 overflow-y-auto pr-1">
          <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
                <Cpu className="w-4 h-4" />
                <span>AI Agents & Role Routing</span>
              </h3>
              <div className="flex flex-wrap items-center gap-2">
                <select
                  onChange={(e) => {
                    const key = e.target.value;
                    if (key && AGENT_PRESETS[key]) {
                      const preset = AGENT_PRESETS[key].config;
                      setConfig({
                        ...config,
                        agents: {
                          ...config.agents,
                          roles: { ...config.agents.roles, [selectedRole]: { ...preset } },
                        },
                      });
                      e.target.value = '';
                    }
                  }}
                  className="bg-stone-900 border border-purple-500/30 text-purple-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
                  defaultValue=""
                >
                  <option value="" disabled>⚡ Load Preset...</option>
                  {Object.entries(AGENT_PRESETS).map(([id, p]) => (
                    <option key={id} value={id}>
                      {p.label}
                    </option>
                  ))}
                </select>

                <span className="text-xs text-stone-400">Role:</span>
                <select
                  value={selectedRole}
                  onChange={(e) => setSelectedRole(e.target.value as any)}
                  className="bg-stone-950 border border-stone-800 rounded-lg pl-2.5 pr-7 py-1 text-xs text-purple-300 font-mono focus:outline-none cursor-pointer"
                >
                  {roleNames.map((role) => (
                    <option key={role} value={role}>
                      {ROLE_LABELS[role] ?? role}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="space-y-4 pt-2">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Provider Type</label>
                  <select
                    value={currentRoleConfig.type}
                    onChange={(e) => {
                      const updated = { ...currentRoleConfig, type: e.target.value as any };
                      setConfig({
                        ...config,
                        agents: {
                          ...config.agents,
                          roles: { ...config.agents.roles, [selectedRole]: updated },
                        },
                      });
                    }}
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
                  >
                    <option value="disabled">Disabled / Inactive</option>
                    <option value="http">HTTP / OpenAI-Compatible (Ollama, vLLM, OpenAI)</option>
                    <option value="cli">CLI Command (Local Binary e.g. llama-cli)</option>
                    <option value="builtin">Builtin / Internal Engine</option>
                    <option value="gemini">Google Gemini (GenAI Cloud)</option>
                    <option value="inherit">Inherit from another role</option>
                  </select>
                </div>

                {currentRoleConfig.type === 'inherit' && (
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">Inherit From</label>
                    <select
                      value={currentRoleConfig.inherit_from || 'gm'}
                      onChange={(e) => updateRole({ type: 'inherit', inherit_from: e.target.value })}
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-purple-300 font-mono focus:outline-none cursor-pointer"
                    >
                      {roleNames.map((role) => (
                        <option key={role} value={role}>
                          {ROLE_LABELS[role] ?? role}
                        </option>
                      ))}
                    </select>
                    <p className="text-[11px] text-stone-500">
                      Resolves to{' '}
                      {config.agents.roles[currentRoleConfig.inherit_from || 'gm']?.model ||
                        config.agents.roles[currentRoleConfig.inherit_from || 'gm']?.command ||
                        'the default provider'}
                      , so changing that role changes this one until you override it here.
                    </p>
                  </div>
                )}

                {currentRoleConfig.type === 'http' && (
                  <>
                    <div className="space-y-1.5">
                      <label className="text-xs font-sans uppercase text-stone-300">Endpoint URL</label>
                      <input
                        type="text"
                        placeholder="e.g. http://localhost:11434/v1"
                        value={currentRoleConfig.endpoint || ''}
                        onChange={(e) => {
                          const updated = { ...currentRoleConfig, endpoint: e.target.value };
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: { ...config.agents.roles, [selectedRole]: updated },
                            },
                          });
                        }}
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                      />
                    </div>

                    <div className="space-y-1.5">
                      <label className="text-xs font-sans uppercase text-stone-300">Model Name</label>
                      <input
                        type="text"
                        placeholder="e.g. llama3.2 or mistral"
                        value={currentRoleConfig.model || ''}
                        onChange={(e) => {
                          const updated = { ...currentRoleConfig, model: e.target.value };
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: { ...config.agents.roles, [selectedRole]: updated },
                            },
                          });
                        }}
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                      />
                    </div>

                    <div className="space-y-1.5">
                      <label className="text-xs font-sans uppercase text-stone-300">API Key (Optional)</label>
                      <input
                        type="password"
                        placeholder="Bearer token or leave empty for Ollama"
                        value={currentRoleConfig.api_key || ''}
                        onChange={(e) => {
                          const updated = { ...currentRoleConfig, api_key: e.target.value };
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: { ...config.agents.roles, [selectedRole]: updated },
                            },
                          });
                        }}
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                      />
                    </div>
                  </>
                )}

                {currentRoleConfig.type === 'cli' && (
                  <>
                    <div className="space-y-1.5">
                      <label className="text-xs font-sans uppercase text-stone-300">CLI Command / Binary</label>
                      <input
                        type="text"
                        placeholder="e.g. echo or /usr/local/bin/llama-cli"
                        value={currentRoleConfig.command || ''}
                        onChange={(e) => {
                          const updated = { ...currentRoleConfig, command: e.target.value };
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: { ...config.agents.roles, [selectedRole]: updated },
                            },
                          });
                        }}
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                      />
                    </div>

                    <div className="space-y-1.5">
                      <label className="text-xs font-sans uppercase text-stone-300">Command Arguments (comma separated)</label>
                      <input
                        type="text"
                        placeholder="e.g. --temp, 0.7, -m, model.gguf"
                        value={(currentRoleConfig.args || []).join(', ')}
                        onChange={(e) => {
                          const args = e.target.value.split(',').map((s) => s.trim()).filter(Boolean);
                          const updated = { ...currentRoleConfig, args };
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: { ...config.agents.roles, [selectedRole]: updated },
                            },
                          });
                        }}
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                      />
                    </div>
                  </>
                )}

                {currentRoleConfig.type === 'builtin' && (
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">Builtin Engine</label>
                    <select
                      value={currentRoleConfig.builtin_name || 'narrative-oracle'}
                      onChange={(e) => {
                        const updated = { ...currentRoleConfig, builtin_name: e.target.value };
                        setConfig({
                          ...config,
                          agents: {
                            ...config.agents,
                            roles: { ...config.agents.roles, [selectedRole]: updated },
                          },
                        });
                      }}
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
                    >
                      <option value="narrative-oracle">narrative-oracle (Deterministic Procedural Storyteller)</option>
                      <option value="echo">echo (Debug Provider)</option>
                    </select>
                  </div>
                )}

                {(currentRoleConfig.type === 'gemini' || (currentRoleConfig.type === 'builtin' && currentRoleConfig.builtin_name === 'gemini')) && (
                  <>
                    <div className="space-y-1.5">
                      <label className="text-xs font-sans uppercase text-stone-300">Gemini Model</label>
                      <input
                        type="text"
                        placeholder="e.g. gemini-2.5-flash"
                        value={currentRoleConfig.model || 'gemini-2.5-flash'}
                        onChange={(e) => {
                          const updated = { ...currentRoleConfig, model: e.target.value };
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: { ...config.agents.roles, [selectedRole]: updated },
                            },
                          });
                        }}
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                      />
                      <div className="flex flex-wrap gap-1.5 pt-1">
                        {['gemini-2.5-flash', 'gemini-2.5-pro', 'gemini-2.0-flash', 'gemini-2.0-flash-lite'].map((m) => (
                          <button
                            key={m}
                            type="button"
                            onClick={() => {
                              const updated = { ...currentRoleConfig, model: m };
                              setConfig({
                                ...config,
                                agents: {
                                  ...config.agents,
                                  roles: { ...config.agents.roles, [selectedRole]: updated },
                                },
                              });
                            }}
                            className={`px-2 py-0.5 text-[10px] font-mono rounded border transition-colors cursor-pointer ${
                              (currentRoleConfig.model || 'gemini-2.5-flash') === m
                                ? 'bg-purple-500/20 border-purple-500/50 text-purple-300'
                                : 'bg-stone-950 border-stone-800 text-stone-400 hover:text-stone-200'
                            }`}
                          >
                            {m}
                          </button>
                        ))}
                      </div>
                    </div>

                    <div className="space-y-1.5">
                      <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                        <span>Role API Key Override</span>
                        {config.providers?.gemini?.api_key && (
                          <span className="text-[10px] text-emerald-400 font-mono">Shared key active (from Providers tab)</span>
                        )}
                      </label>
                      <input
                        type="password"
                        placeholder={config.providers?.gemini?.api_key ? 'Using shared key from Providers tab (leave blank)' : 'Optional override or GEMINI_API_KEY env'}
                        value={currentRoleConfig.api_key || ''}
                        onChange={(e) => {
                          const updated = { ...currentRoleConfig, api_key: e.target.value };
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: { ...config.agents.roles, [selectedRole]: updated },
                            },
                          });
                        }}
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                      />
                    </div>

                    <div className="space-y-1.5 col-span-1 md:col-span-2">
                      <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                        <span>Thinking / Reasoning Budget</span>
                        <span className="font-mono text-purple-400">
                          {currentRoleConfig.thinking_budget === undefined
                            ? 'Default'
                            : currentRoleConfig.thinking_budget === 0
                            ? '0 (Disabled — Instant Narration)'
                            : currentRoleConfig.thinking_budget === -1
                            ? 'Dynamic (-1 — Model Decides)'
                            : `${currentRoleConfig.thinking_budget} tokens`}
                        </span>
                      </label>
                      <div className="flex gap-2">
                        <select
                          value={
                            currentRoleConfig.thinking_budget === undefined
                              ? 'default'
                              : currentRoleConfig.thinking_budget === 0
                              ? '0'
                              : currentRoleConfig.thinking_budget === -1
                              ? '-1'
                              : 'custom'
                          }
                          onChange={(e) => {
                            const val = e.target.value;
                            let budget: number | undefined;
                            if (val === '0') budget = 0;
                            else if (val === '-1') budget = -1;
                            else if (val === 'custom') budget = 2048;
                            else budget = undefined;

                            const updated = { ...currentRoleConfig, thinking_budget: budget };
                            setConfig({
                              ...config,
                              agents: {
                                ...config.agents,
                                roles: { ...config.agents.roles, [selectedRole]: updated },
                              },
                            });
                          }}
                          className="bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
                        >
                          <option value="0">Disabled (0 — Instant Narration)</option>
                          <option value="-1">Dynamic (-1 — Model Decides)</option>
                          <option value="default">Model Default</option>
                          <option value="custom">Custom Token Limit</option>
                        </select>
                        {(currentRoleConfig.thinking_budget ?? 0) > 0 && (
                          <input
                            type="number"
                            min={128}
                            step={256}
                            value={currentRoleConfig.thinking_budget}
                            onChange={(e) => {
                              const budget = parseInt(e.target.value, 10);
                              const updated = { ...currentRoleConfig, thinking_budget: isNaN(budget) ? 0 : budget };
                              setConfig({
                                ...config,
                                agents: {
                                  ...config.agents,
                                  roles: { ...config.agents.roles, [selectedRole]: updated },
                                },
                              });
                            }}
                            className="w-32 bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                          />
                        )}
                      </div>
                      <p className="text-[11px] text-stone-500">
                        Thinking tokens allow Gemini 2.5 to reason deeply before replying. Thought tokens are automatically filtered from the story chronicle.
                      </p>
                    </div>

                    <div className="space-y-1.5">
                      <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                        <span>Top-P</span>
                        <span className="font-mono text-purple-400">{(currentRoleConfig.top_p ?? 0.95).toFixed(2)}</span>
                      </label>
                      <input
                        type="number"
                        min={0}
                        max={1}
                        step={0.05}
                        value={currentRoleConfig.top_p ?? 0.95}
                        onChange={(e) => {
                          const top_p = parseFloat(e.target.value);
                          const updated = { ...currentRoleConfig, top_p: isNaN(top_p) ? undefined : top_p };
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: { ...config.agents.roles, [selectedRole]: updated },
                            },
                          });
                        }}
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                      />
                    </div>

                    <div className="space-y-1.5">
                      <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                        <span>Top-K</span>
                        <span className="font-mono text-purple-400">{currentRoleConfig.top_k ?? 40}</span>
                      </label>
                      <input
                        type="number"
                        min={1}
                        max={100}
                        step={1}
                        value={currentRoleConfig.top_k ?? 40}
                        onChange={(e) => {
                          const top_k = parseInt(e.target.value, 10);
                          const updated = { ...currentRoleConfig, top_k: isNaN(top_k) ? undefined : top_k };
                          setConfig({
                            ...config,
                            agents: {
                              ...config.agents,
                              roles: { ...config.agents.roles, [selectedRole]: updated },
                            },
                          });
                        }}
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                      />
                    </div>
                  </>
                )}
              </div>

              {currentRoleConfig.type !== 'disabled' && (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                      <span>Response Limit (max tokens)</span>
                      <span className="font-mono text-purple-400">{currentRoleConfig.max_tokens ?? 1024}</span>
                    </label>
                    <input
                      type="number"
                      min={64}
                      step={64}
                      value={currentRoleConfig.max_tokens ?? 1024}
                      onChange={(e) => {
                        const max_tokens = parseInt(e.target.value, 10);
                        const updated = { ...currentRoleConfig, max_tokens: Number.isNaN(max_tokens) ? undefined : max_tokens };
                        setConfig({
                          ...config,
                          agents: { ...config.agents, roles: { ...config.agents.roles, [selectedRole]: updated } },
                        });
                      }}
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                    <p className="text-[11px] text-stone-500">
                      How long a single reply may be. Raise it for longer scenes; the reply is marked as cut off when it
                      hits this.
                    </p>
                  </div>

                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                      <span>Temperature</span>
                      <span className="font-mono text-purple-400">{(currentRoleConfig.temperature ?? 0.7).toFixed(2)}</span>
                    </label>
                    <input
                      type="range"
                      min="0"
                      max="1.5"
                      step="0.05"
                      value={currentRoleConfig.temperature ?? 0.7}
                      onChange={(e) => {
                        const updated = { ...currentRoleConfig, temperature: parseFloat(e.target.value) };
                        setConfig({
                          ...config,
                          agents: { ...config.agents, roles: { ...config.agents.roles, [selectedRole]: updated } },
                        });
                      }}
                      className="w-full accent-purple-500"
                    />
                    <p className="text-[11px] text-stone-500">Lower is steadier, which helps long-run continuity.</p>
                  </div>
                </div>
              )}

              {currentRoleConfig.type !== 'inherit' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Tool Calling</label>
                  <select
                    value={currentRoleConfig.supports_tools ?? 'auto'}
                    onChange={(e) => {
                      const updated = { ...currentRoleConfig, supports_tools: e.target.value as 'auto' | 'yes' | 'no' };
                      setConfig({
                        ...config,
                        agents: { ...config.agents, roles: { ...config.agents.roles, [selectedRole]: updated } },
                      });
                    }}
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
                  >
                    <option value="auto">Auto (HTTP providers only)</option>
                    <option value="yes">Yes (force tools)</option>
                    <option value="no">No (suppress tools)</option>
                  </select>
                  <p className="text-[11px] text-stone-500">
                    Whether this role may look things up mid-turn. Only the gm role is offered tools.
                  </p>
                </div>
              )}

              {currentRoleConfig.type !== 'disabled' && (
                <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
                  <button
                    onClick={() => handleTestProvider('llm', currentRoleConfig)}
                    disabled={testingCategory === 'llm'}
                    className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg bg-stone-900 border border-purple-500/30 hover:bg-stone-800 text-purple-400 transition-all cursor-pointer"
                  >
                    <Zap className="w-3.5 h-3.5" />
                    <span>{testingCategory === 'llm' ? 'Testing Connection...' : 'Test Connection'}</span>
                  </button>

                  {testResult?.category === 'llm' && (
                    <span
                      className={`text-xs font-mono flex items-center gap-1 ${
                        testResult.res.success ? 'text-emerald-400' : 'text-red-400'
                      }`}
                    >
                      {testResult.res.success ? <CheckCircle className="w-3.5 h-3.5" /> : <AlertCircle className="w-3.5 h-3.5" />}
                      <span>
                        {testResult.res.message} ({testResult.res.latency_ms}ms)
                      </span>
                    </span>
                  )}
                </div>
              )}
            </div>
          </div>

          {/* Context budget and timing: what the narrator is sent, and how long it may take */}
          <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
            <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
              <Sliders className="w-4 h-4" />
              <span>Context &amp; Response Limits</span>
            </h3>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Context Budget (tokens)</span>
                  <span className="font-mono text-purple-400">
                    {(config.agents.context_token_budget ?? 0) === 0 ? 'unbounded' : config.agents.context_token_budget}
                  </span>
                </label>
                <input
                  type="number"
                  min={0}
                  step={1000}
                  value={config.agents.context_token_budget ?? 0}
                  onChange={(e) => {
                    const parsed = parseInt(e.target.value, 10);
                    setConfig({
                      ...config,
                      agents: { ...config.agents, context_token_budget: Number.isNaN(parsed) ? 0 : parsed },
                    });
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  Estimated ceiling for the assembled prompt. 0 sends everything. When it is exceeded, the voice
                  catalogue and the oldest remembered turns are dropped first; rules, lore, the scene, and your action
                  are never dropped.
                </p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Remembered Turns</span>
                  <span className="font-mono text-purple-400">{config.agents.recent_turn_window ?? 6}</span>
                </label>
                <input
                  type="number"
                  min={0}
                  max={100}
                  step={1}
                  value={config.agents.recent_turn_window ?? 6}
                  onChange={(e) => {
                    const parsed = parseInt(e.target.value, 10);
                    setConfig({
                      ...config,
                      agents: { ...config.agents, recent_turn_window: Number.isNaN(parsed) ? 6 : parsed },
                    });
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  How many prior turns are replayed to the narrator. A larger window means better continuity and a
                  larger prompt.
                </p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Excerpt Length (characters)</span>
                  <span className="font-mono text-purple-400">{config.agents.recent_turn_char_limit ?? 1200}</span>
                </label>
                <input
                  type="number"
                  min={200}
                  step={100}
                  value={config.agents.recent_turn_char_limit ?? 1200}
                  onChange={(e) => {
                    const parsed = parseInt(e.target.value, 10);
                    setConfig({
                      ...config,
                      agents: { ...config.agents, recent_turn_char_limit: Number.isNaN(parsed) ? 1200 : parsed },
                    });
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">Cap on the text recalled from any one prior turn.</p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Turns Recalled At This Location</span>
                  <span className="font-mono text-purple-400">{config.agents.scene_recall_turns ?? 4}</span>
                </label>
                <input
                  type="number"
                  min={0}
                  max={20}
                  value={config.agents.scene_recall_turns ?? 4}
                  onChange={(e) => {
                    const parsed = parseInt(e.target.value, 10);
                    setConfig({
                      ...config,
                      agents: { ...config.agents, scene_recall_turns: Number.isNaN(parsed) ? 4 : parsed },
                    });
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">What happened where the party is standing.</p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Recalled Excerpt Length</span>
                  <span className="font-mono text-purple-400">{config.agents.scene_recall_chars ?? 800}</span>
                </label>
                <input
                  type="number"
                  min={100}
                  step={100}
                  value={config.agents.scene_recall_chars ?? 800}
                  onChange={(e) => {
                    const parsed = parseInt(e.target.value, 10);
                    setConfig({
                      ...config,
                      agents: { ...config.agents, scene_recall_chars: Number.isNaN(parsed) ? 800 : parsed },
                    });
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">Cap on the excerpt taken from one recalled turn.</p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Turns Retrieved By Entity</span>
                  <span className="font-mono text-purple-400">{config.agents.retrieval_turns ?? 3}</span>
                </label>
                <input
                  type="number"
                  min={0}
                  max={20}
                  value={config.agents.retrieval_turns ?? 3}
                  onChange={(e) => {
                    const parsed = parseInt(e.target.value, 10);
                    setConfig({
                      ...config,
                      agents: { ...config.agents, retrieval_turns: Number.isNaN(parsed) ? 3 : parsed },
                    });
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  Past turns that share characters with the ones in play, wherever they happened.
                </p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Retrieved Excerpt Length</span>
                  <span className="font-mono text-purple-400">{config.agents.retrieval_chars ?? 800}</span>
                </label>
                <input
                  type="number"
                  min={100}
                  step={100}
                  value={config.agents.retrieval_chars ?? 800}
                  onChange={(e) => {
                    const parsed = parseInt(e.target.value, 10);
                    setConfig({
                      ...config,
                      agents: { ...config.agents, retrieval_chars: Number.isNaN(parsed) ? 800 : parsed },
                    });
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">Cap on the excerpt taken from one retrieved turn.</p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Retrieval Recency Half-Life</span>
                  <span className="font-mono text-purple-400">{config.agents.retrieval_halflife_turns ?? 12}</span>
                </label>
                <input
                  type="number"
                  min={1}
                  max={200}
                  value={config.agents.retrieval_halflife_turns ?? 12}
                  onChange={(e) => {
                    const parsed = parseInt(e.target.value, 10);
                    setConfig({
                      ...config,
                      agents: { ...config.agents, retrieval_halflife_turns: Number.isNaN(parsed) ? 12 : parsed },
                    });
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  Turns after which a retrieved turn's recency weight halves. Lower favours the recent.
                </p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Turn Timeout (seconds)</span>
                  <span className="font-mono text-purple-400">{config.agents.turn_timeout_seconds ?? 300}</span>
                </label>
                <input
                  type="number"
                  min={10}
                  step={10}
                  value={config.agents.turn_timeout_seconds ?? 300}
                  onChange={(e) => {
                    const parsed = parseInt(e.target.value, 10);
                    setConfig({
                      ...config,
                      agents: { ...config.agents, turn_timeout_seconds: Number.isNaN(parsed) ? 300 : parsed },
                    });
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  Wall clock for a whole turn. Raise it for slower local models and long contexts.
                </p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Silence Timeout (seconds)</span>
                  <span className="font-mono text-purple-400">{config.agents.chunk_timeout_seconds ?? 60}</span>
                </label>
                <input
                  type="number"
                  min={5}
                  step={5}
                  value={config.agents.chunk_timeout_seconds ?? 60}
                  onChange={(e) => {
                    const parsed = parseInt(e.target.value, 10);
                    setConfig({
                      ...config,
                      agents: { ...config.agents, chunk_timeout_seconds: Number.isNaN(parsed) ? 60 : parsed },
                    });
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  How long the narrator may go quiet between chunks before the turn fails.
                </p>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Tool Rounds / Turn</label>
                  <input
                    type="number"
                    min={0}
                    max={20}
                    value={config.agents.tool_rounds ?? 4}
                    onChange={(e) => {
                      const parsed = parseInt(e.target.value, 10);
                      setConfig({
                        ...config,
                        agents: { ...config.agents, tool_rounds: Number.isNaN(parsed) ? 4 : parsed },
                      });
                    }}
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                  />
                  <p className="text-[11px] text-stone-500">How many times a turn may look something up before answering.</p>
                </div>

                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Tool Result Characters</label>
                  <input
                    type="number"
                    min={0}
                    max={50000}
                    step={500}
                    value={config.agents.tool_result_chars ?? 4000}
                    onChange={(e) => {
                      const parsed = parseInt(e.target.value, 10);
                      setConfig({
                        ...config,
                        agents: { ...config.agents, tool_result_chars: Number.isNaN(parsed) ? 4000 : parsed },
                      });
                    }}
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                  />
                  <p className="text-[11px] text-stone-500">The most of one lookup the model is shown at once.</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Tab 3: Media Engines (TTS / STT / Image) */}
      {activeSubTab === 'media' && (
        <div className="space-y-4 flex-1 overflow-y-auto pr-1">
          {/* TTS Section */}
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
                    if (key && TTS_PRESETS[key]) {
                      const preset = TTS_PRESETS[key].config;
                      setConfig({
                        ...config,
                        media: {
                          ...config.media,
                          tts: {
                            ...preset,
                            builtin_name: preset.builtin_name,
                            options: preset.options,
                            endpoint: preset.endpoint,
                            model: preset.model,
                            api_key: preset.api_key,
                            auto_play: config.media.tts.auto_play,
                            voice_profiles: preset.voice_profiles ?? config.media.tts.voice_profiles,
                          },
                        },
                      });
                      e.target.value = '';
                    }
                  }}
                  className="bg-stone-900 border border-purple-500/30 text-purple-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
                  defaultValue=""
                >
                  <option value="" disabled>⚡ Load TTS Preset...</option>
                  {Object.entries(TTS_PRESETS).map(([id, p]) => (
                    <option key={id} value={id}>
                      {p.label}
                    </option>
                  ))}
                </select>

                <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={config.media.tts.auto_play}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: { ...config.media, tts: { ...config.media.tts, auto_play: e.target.checked } },
                      })
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
                value={config.media.tts.markdown || 'auto'}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    media: {
                      ...config.media,
                      tts: { ...config.media.tts, markdown: e.target.value as 'auto' | 'strip' | 'keep' },
                    },
                  })
                }
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60"
              >
                <option value="auto">Auto - reduce formatting unless the provider understands it</option>
                <option value="strip">Always reduce formatting to plain speech</option>
                <option value="keep">Keep formatting as written</option>
              </select>
              <p className="text-[11px] text-stone-500">
                Markdown emphasis, headings, lists and wikilinks are otherwise read aloud by engines that do not interpret them.
              </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300">TTS Engine</label>
                <select
                  value={
                    config.media.tts.type === 'gemini' ||
                    (config.media.tts.type === 'builtin' && config.media.tts.builtin_name === 'gemini')
                      ? 'gemini'
                      : config.media.tts.type === 'builtin'
                      ? `builtin:${config.media.tts.builtin_name || 'native-os'}`
                      : config.media.tts.type
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
                                config.media.tts.model && config.media.tts.model.includes('eleven')
                                  ? config.media.tts.model
                                  : 'eleven_multilingual_v2',
                              default_voice:
                                config.media.tts.default_voice &&
                                config.media.tts.default_voice.startsWith('EXAV')
                                  ? config.media.tts.default_voice
                                  : 'EXAVITQu4vr4xnSDxMaL',
                            }
                          : builtinName === 'sherpa-onnx'
                          ? {
                              default_voice:
                                config.media.tts.default_voice &&
                                config.media.tts.default_voice.startsWith('af_')
                                  ? config.media.tts.default_voice
                                  : 'af_bella',
                            }
                          : {};
                      setConfig({
                        ...config,
                        media: {
                          ...config.media,
                          tts: {
                            ...config.media.tts,
                            ...builtinDefaults,
                            type: 'builtin',
                            builtin_name: builtinName,
                            options: builtinName === 'elevenlabs' ? config.media.tts.options : undefined,
                          },
                        },
                      });
                    } else if (val === 'gemini') {
                      setConfig({
                        ...config,
                        media: {
                          ...config.media,
                          tts: {
                            ...config.media.tts,
                            type: 'gemini',
                            builtin_name: undefined,
                            model:
                              config.media.tts.model && config.media.tts.model.includes('gemini')
                                ? config.media.tts.model
                                : 'gemini-3.8-flash-tts',
                            default_voice:
                              config.media.tts.default_voice &&
                              !config.media.tts.default_voice.startsWith('EXAV') &&
                              !config.media.tts.default_voice.startsWith('af_')
                                ? config.media.tts.default_voice
                                : 'Aoede',
                            options: undefined,
                          },
                        },
                      });
                    } else if (val === 'http') {
                      setConfig({
                        ...config,
                        media: {
                          ...config.media,
                          tts: {
                            ...config.media.tts,
                            type: 'http',
                            builtin_name: undefined,
                            endpoint: config.media.tts.endpoint || 'http://localhost:8880/v1/audio/speech',
                            model:
                              config.media.tts.model &&
                              !config.media.tts.model.includes('eleven') &&
                              !config.media.tts.model.includes('gemini')
                                ? config.media.tts.model
                                : 'kokoro',
                            default_voice:
                              config.media.tts.default_voice &&
                              !config.media.tts.default_voice.startsWith('EXAV') &&
                              config.media.tts.default_voice !== 'Aoede'
                                ? config.media.tts.default_voice
                                : 'af_bella',
                            options: undefined,
                          },
                        },
                      });
                    } else {
                      setConfig({
                        ...config,
                        media: {
                          ...config.media,
                          tts: { ...config.media.tts, type: val as any, builtin_name: undefined, options: undefined },
                        },
                      });
                    }
                  }}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
                >
                  <option value="disabled">Disabled</option>
                  <option value="gemini">Google Gemini TTS (Cloud, metered)</option>
                  <option value="builtin:sherpa-onnx">Built-in: Sherpa-ONNX (Kokoro Neural Voice)</option>
                  <option value="builtin:native-os">Built-in: Native OS Speech (spd-say / SAPI / procedural)</option>
                  <option value="builtin:elevenlabs">Built-in: ElevenLabs (Cloud, metered)</option>
                  <option value="http">HTTP Endpoint (Kokoro-FastAPI, AllTalk, OpenAI Speech)</option>
                  <option value="cli">CLI Command (e.g. piper)</option>
                </select>
              </div>

              {config.media.tts.type === 'http' && (
                <>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">Speech Endpoint URL</label>
                    <input
                      type="text"
                      placeholder="e.g. http://localhost:8880/v1/audio/speech"
                      value={config.media.tts.endpoint || ''}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: { ...config.media, tts: { ...config.media.tts, endpoint: e.target.value } },
                        })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">Model Name</label>
                    <input
                      type="text"
                      placeholder="e.g. kokoro or tts-1"
                      value={config.media.tts.model || ''}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: { ...config.media, tts: { ...config.media.tts, model: e.target.value } },
                        })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">API Key (Optional)</label>
                    <input
                      type="password"
                      placeholder="Optional authorization token (e.g. for OpenAI)"
                      value={config.media.tts.api_key || ''}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: { ...config.media, tts: { ...config.media.tts, api_key: e.target.value } },
                        })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                </>
              )}

              {config.media.tts.type === 'cli' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Command / Binary</label>
                  <input
                    type="text"
                    placeholder="e.g. piper"
                    value={config.media.tts.command || ''}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: { ...config.media, tts: { ...config.media.tts, command: e.target.value } },
                      })
                    }
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                  />
                </div>
              )}

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Master Volume</span>
                  <span className="font-mono text-purple-400">
                    {Math.round((config.media.tts.master_volume || 1.0) * 100)}%
                  </span>
                </label>
                <input
                  type="range"
                  min="0"
                  max="1"
                  step="0.05"
                  value={config.media.tts.master_volume || 1.0}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      media: {
                        ...config.media,
                        tts: { ...config.media.tts, master_volume: parseFloat(e.target.value) },
                      },
                    })
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
                    <span className="flex items-center gap-1 text-emerald-400 font-mono text-[11px] bg-emerald-950/40 border border-emerald-800/40 px-2 py-0.5 rounded-md">
                      <CheckCircle className="w-3.5 h-3.5" />
                      <span>Installed</span>
                    </span>
                  ) : kokoroStatus?.downloading ? (
                    <div className="flex items-center gap-2 text-purple-400 font-mono text-[11px]">
                      <span>Downloading {Math.round(kokoroStatus.progress * 100)}%</span>
                      <div className="w-20 h-1.5 bg-stone-800 rounded-full overflow-hidden">
                        <div
                          className="h-full bg-purple-500"
                          style={{ width: `${Math.round(kokoroStatus.progress * 100)}%` }}
                        />
                      </div>
                    </div>
                  ) : (
                    <span className="flex items-center gap-1 text-purple-400/90 font-mono text-[11px] bg-purple-950/40 border border-purple-800/40 px-2 py-0.5 rounded-md">
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
                  value={config.media.tts.model || 'gemini-3.8-flash-tts'}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      media: { ...config.media, tts: { ...config.media.tts, model: e.target.value } },
                    })
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
                        setConfig({
                          ...config,
                          media: { ...config.media, tts: { ...config.media.tts, model: m.id } },
                        })
                      }
                      className={`text-[11px] px-2.5 py-1 rounded-lg border font-mono transition cursor-pointer ${
                        (config.media.tts.model || 'gemini-3.8-flash-tts') === m.id
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
              <div className="flex items-center gap-2 text-[11px] font-mono text-purple-400/90">
                <span className="px-1.5 py-0.5 rounded border border-purple-500/40 bg-purple-500/10">METERED</span>
                <span>This provider charges per request. Cached clips are reused.</span>
              </div>
            )}

            {inspect?.key_required && !inspect.key_present && (
              <div className="text-[11px] font-mono text-stone-400">
                {isGeminiTTS
                  ? 'No Gemini API key configured. Enter one below, or set GEMINI_API_KEY / GOOGLE_API_KEY in the environment.'
                  : isElevenLabsTTS
                  ? 'No API key configured. Enter one below, or set ELEVENLABS_API_KEY in the environment.'
                  : 'No API key configured. Enter one below.'}
              </div>
            )}

            {inspect?.key_required && (
              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>API Key</span>
                  {isGeminiTTS && config.providers?.gemini?.api_key && !config.media.tts.api_key && (
                    <span className="text-[11px] text-emerald-400 font-mono">Using shared Gemini key</span>
                  )}
                </label>
                <input
                  type="password"
                  placeholder={
                    isGeminiTTS && config.providers?.gemini?.api_key
                      ? 'Using shared key from providers.gemini.api_key'
                      : "Leave empty to use the provider's environment variable"
                  }
                  value={config.media.tts.api_key || ''}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      media: { ...config.media, tts: { ...config.media.tts, api_key: e.target.value } },
                    })
                  }
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  {isGeminiTTS
                    ? 'Stored in your configuration file. Set GEMINI_API_KEY instead to keep it off disk.'
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
                  values={config.media.tts.options ?? {}}
                  onChange={(key, value) =>
                    setConfig({
                      ...config,
                      media: {
                        ...config.media,
                        tts: { ...config.media.tts, options: { ...(config.media.tts.options ?? {}), [key]: value } },
                      },
                    })
                  }
                />
              </div>
            )}

            {inspect?.catalog.available && (
              <div className="flex flex-wrap items-center justify-between gap-2 text-[11px] font-mono text-stone-400">
                <span>
                  {inspect.catalog.voices.length} voices
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
              <div className="p-2.5 rounded-lg bg-red-950/40 border border-red-900/60 text-[11px] text-red-300 font-mono">
                {inspect?.error || inspectError}
              </div>
            )}

            {config.media.tts.type !== 'disabled' && (
              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300">Default Voice</label>
                <VoiceCombobox
                  value={config.media.tts.default_voice || ''}
                  onChange={(voiceID) =>
                    setConfig({
                      ...config,
                      media: {
                        ...config.media,
                        tts: { ...config.media.tts, default_voice: voiceID },
                      },
                    })
                  }
                  voices={availableTTSVoices}
                  placeholder="Select default provider voice..."
                />
                <p className="text-[11px] text-stone-500">
                  Fallback voice used for turn narration and unvoiced characters.
                </p>
              </div>
            )}

            {config.media.tts.type !== 'disabled' && (
              <div className="space-y-3 p-4 rounded-xl bg-stone-900/40 border border-stone-800">
                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <div className="text-xs font-sans font-bold text-stone-200">
                      Speech Steering & Acting Cues
                    </div>
                    <div className="text-[11px] text-stone-400">
                      Instruct the GM to use emotive directions (e.g. [whispers], [sighs]) when supported.
                    </div>
                  </div>
                  <input
                    type="checkbox"
                    checked={config.media.tts.speech_cues?.enabled ?? true}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: {
                          ...config.media,
                          tts: {
                            ...config.media.tts,
                            speech_cues: {
                              ...config.media.tts.speech_cues,
                              enabled: e.target.checked,
                            },
                          },
                        },
                      })
                    }
                    className="w-4 h-4 rounded border-stone-700 bg-stone-900 text-purple-500 focus:ring-purple-500/40 cursor-pointer"
                  />
                </div>

                {inspect?.speech_cues && (
                  <div className="text-[11px] p-2.5 rounded-lg bg-stone-950/60 border border-stone-800/80 text-stone-400">
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

                {(config.media.tts.speech_cues?.enabled ?? true) && (
                  <div className="space-y-1.5 pt-1">
                    <label className="text-xs font-sans uppercase text-stone-300">
                      Transcript Display Mode
                    </label>
                    <select
                      value={config.media.tts.speech_cues?.display_mode || 'stage_directions'}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: {
                            ...config.media,
                            tts: {
                              ...config.media.tts,
                              speech_cues: {
                                ...config.media.tts.speech_cues,
                                enabled: config.media.tts.speech_cues?.enabled ?? true,
                                display_mode: e.target.value as any,
                              },
                            },
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

            {config.media.tts.type !== 'disabled' && (
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
                    onClick={() => handleTestProvider('tts', config.media.tts, ttsPreviewText)}
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
                  <span className="text-[10px] font-mono text-stone-500">
                    ({config.media.tts.voice_profiles?.length || 0} archetypes)
                  </span>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {isKokoro && (
                    <button
                      onClick={() => {
                        setConfig({
                          ...config,
                          media: {
                            ...config.media,
                            tts: {
                              ...config.media.tts,
                              voice_profiles: [...KOKORO_VOICE_PROFILES],
                            },
                          },
                        });
                      }}
                      className="flex items-center gap-1 text-[11px] px-2 py-1 rounded bg-purple-600/20 border border-purple-500/40 text-purple-300 hover:bg-purple-600/30 transition cursor-pointer"
                      title="Autofill all 11 Kokoro voice profiles with gender and accent tags"
                    >
                      <Sparkles className="w-3 h-3" />
                      <span>Load Kokoro Voices (11 Profiles)</span>
                    </button>
                  )}

                  <button
                    onClick={() => {
                      setConfig({
                        ...config,
                        media: {
                          ...config.media,
                          tts: {
                            ...config.media.tts,
                            voice_profiles: [...DEFAULT_VOICE_PROFILES],
                          },
                        },
                      });
                    }}
                    className="flex items-center gap-1 text-[11px] px-2 py-1 rounded bg-stone-900 border border-stone-700 text-stone-300 hover:text-purple-300 transition cursor-pointer"
                    title="Restore default fantasy archetypes"
                  >
                    <RotateCcw className="w-3 h-3" />
                    <span>Load Fantasy Defaults</span>
                  </button>

                  {Boolean(inspect?.catalog.voices?.length) && (
                    <button
                      type="button"
                      onClick={() => setIsCatalogModalOpen(true)}
                      className="flex items-center gap-1 text-[11px] px-2 py-1 rounded bg-stone-900 border border-purple-500/40 text-purple-300 hover:bg-stone-800 transition cursor-pointer"
                    >
                      <Plus className="w-3 h-3" />
                      <span>Import from Catalog</span>
                    </button>
                  )}

                  <button
                    onClick={() => {
                      const currentProfiles = config.media.tts.voice_profiles || [];
                      const newProfile: VoiceProfile = {
                        id: `npc_voice_${currentProfiles.length + 1}`,
                        name: 'New Archetype',
                        voice_id: config.media.tts.default_voice || 'default',
                        pitch: 1.0,
                        speech_rate: 1.0,
                        tags: ['npc'],
                        description: 'Distinctive voice description for automatic GM matching.',
                      };
                      setConfig({
                        ...config,
                        media: {
                          ...config.media,
                          tts: {
                            ...config.media.tts,
                            voice_profiles: [...currentProfiles, newProfile],
                          },
                        },
                      });
                    }}
                    className="flex items-center gap-1 text-[11px] px-2 py-1 rounded bg-purple-600/20 border border-purple-500/40 text-purple-300 hover:bg-purple-600/30 transition cursor-pointer"
                  >
                    <Plus className="w-3 h-3" />
                    <span>Add Profile</span>
                  </button>
                </div>
              </div>

              <p className="text-[11px] text-stone-400">
                The GM and world extractor match NPC descriptions against these voice archetypes and tags to assign unique speech parameters automatically.
              </p>

              <div className="space-y-2 max-h-72 overflow-y-auto pr-1">
                {(config.media.tts.voice_profiles || []).map((profile, idx) => (
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
                            const updated = [...(config.media.tts.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], id: e.target.value };
                            setConfig({
                              ...config,
                              media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                            });
                          }}
                          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs font-mono text-purple-300 focus:outline-none"
                        />
                        <input
                          type="text"
                          placeholder="Display Name"
                          value={profile.name}
                          onChange={(e) => {
                            const updated = [...(config.media.tts.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], name: e.target.value };
                            setConfig({
                              ...config,
                              media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                            });
                          }}
                          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-200 focus:outline-none"
                        />
                        <VoiceCombobox
                          value={profile.voice_id}
                          onChange={(voiceID) => {
                            const updated = [...(config.media.tts.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], voice_id: voiceID };
                            setConfig({
                              ...config,
                              media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                            });
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
                                ...config.media.tts,
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
                            const updated = (config.media.tts.voice_profiles || []).filter((_, i) => i !== idx);
                            setConfig({
                              ...config,
                              media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                            });
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
                        <div className="flex justify-between text-[11px] text-stone-400">
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
                            const updated = [...(config.media.tts.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], pitch: parseFloat(e.target.value) };
                            setConfig({
                              ...config,
                              media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                            });
                          }}
                          className="w-full accent-purple-500"
                        />
                      </div>

                      <div className="space-y-1">
                        <div className="flex justify-between text-[11px] text-stone-400">
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
                            const updated = [...(config.media.tts.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], speech_rate: parseFloat(e.target.value) };
                            setConfig({
                              ...config,
                              media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                            });
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
                          const updated = [...(config.media.tts.voice_profiles || [])];
                          updated[idx] = { ...updated[idx], tags };
                          setConfig({
                            ...config,
                            media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                          });
                        }}
                        className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-300 focus:outline-none"
                      />
                      <input
                        type="text"
                        placeholder="Description (e.g. Ancient wizards, village elders)"
                        value={profile.description || ''}
                        onChange={(e) => {
                          const updated = [...(config.media.tts.voice_profiles || [])];
                          updated[idx] = { ...updated[idx], description: e.target.value };
                          setConfig({
                            ...config,
                            media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                          });
                        }}
                        className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs text-stone-300 focus:outline-none"
                      />
                    </div>

                    {inspect && inspect.options && inspect.options.length > 0 && (
                      <details className="text-xs">
                        <summary className="cursor-pointer text-[11px] font-sans uppercase text-stone-400">
                          Provider Options
                        </summary>
                        <div className="pt-2">
                          <VoiceOptionsControl
                            schema={inspect.options}
                            values={profile.options ?? {}}
                            onChange={(key, value) => {
                              const updated = [...(config.media.tts.voice_profiles || [])];
                              updated[idx] = {
                                ...updated[idx],
                                options: { ...(updated[idx].options ?? {}), [key]: value },
                              };
                              setConfig({
                                ...config,
                                media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                              });
                            }}
                          />
                        </div>
                      </details>
                    )}
                  </div>
                ))}

                {(!config.media.tts.voice_profiles || config.media.tts.voice_profiles.length === 0) && (
                  <div className="p-3 text-center text-xs text-stone-500 border border-dashed border-stone-800 rounded-lg">
                    No voice profiles defined yet. Click "Load Fantasy Defaults" to initialize standard archetypes.
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Speech-to-Text (STT) Section */}
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
                    if (key && STT_PRESETS[key]) {
                      const preset = STT_PRESETS[key].config;
                      setConfig({
                        ...config,
                        media: { ...config.media, stt: { ...preset } },
                      });
                      e.target.value = '';
                    }
                  }}
                  className="bg-stone-900 border border-purple-500/30 text-purple-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
                  defaultValue=""
                >
                  <option value="" disabled>⚡ Load STT Preset...</option>
                  {Object.entries(STT_PRESETS).map(([id, p]) => (
                    <option key={id} value={id}>
                      {p.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300">STT Provider Type</label>
                <select
                  value={config.media.stt?.type || 'disabled'}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      media: {
                        ...config.media,
                        stt: { ...(config.media.stt || { type: 'disabled' }), type: e.target.value as any },
                      },
                    })
                  }
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
                >
                  <option value="disabled">Disabled</option>
                  <option value="web-speech">Web Speech API (Browser Native)</option>
                  <option value="http">HTTP (Faster-Whisper, OpenAI Whisper)</option>
                  <option value="cli">CLI Command (e.g. whisper-cli)</option>
                  <option value="builtin">Builtin / Mock</option>
                </select>
              </div>

              {config.media.stt?.type === 'http' && (
                <>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">Transcription Endpoint URL</label>
                    <input
                      type="text"
                      placeholder="e.g. http://localhost:8000/v1/audio/transcriptions"
                      value={config.media.stt.endpoint || ''}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: { ...config.media, stt: { ...config.media.stt, endpoint: e.target.value } },
                        })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-xs font-sans uppercase text-stone-300">Model Name</label>
                    <input
                      type="text"
                      placeholder="e.g. whisper-1"
                      value={config.media.stt.model || ''}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: { ...config.media, stt: { ...config.media.stt, model: e.target.value } },
                        })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                </>
              )}

              {config.media.stt?.type === 'cli' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Command / Binary</label>
                  <input
                    type="text"
                    placeholder="e.g. whisper-cli"
                    value={config.media.stt.command || ''}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: { ...config.media, stt: { ...config.media.stt, command: e.target.value } },
                      })
                    }
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                  />
                </div>
              )}
            </div>

            {config.media.stt?.type && config.media.stt.type !== 'disabled' && (
              <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
                <button
                  onClick={() => handleTestProvider('stt', config.media.stt)}
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

          {/* Image Generation Section */}
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
                    if (key && IMAGE_PRESETS[key]) {
                      const preset = IMAGE_PRESETS[key].config;
                      setConfig({
                        ...config,
                        media: {
                          ...config.media,
                          image: { ...preset, auto_generate: config.media.image.auto_generate },
                        },
                      });
                      e.target.value = '';
                    }
                  }}
                  className="bg-stone-900 border border-purple-500/30 text-purple-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
                  defaultValue=""
                >
                  <option value="" disabled>⚡ Load Image Preset...</option>
                  {Object.entries(IMAGE_PRESETS).map(([id, p]) => (
                    <option key={id} value={id}>
                      {p.label}
                    </option>
                  ))}
                </select>

                <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={config.media.image.auto_generate}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: { ...config.media, image: { ...config.media.image, auto_generate: e.target.checked } },
                      })
                    }
                    className="rounded bg-stone-950 border-stone-800 text-purple-600 focus:ring-0"
                  />
                  <span>Auto-generate Scene Art</span>
                </label>

                <label className="flex items-center gap-2 text-xs text-stone-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={config.media.image.builtin_fallback !== false}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: { ...config.media, image: { ...config.media.image, builtin_fallback: e.target.checked } },
                      })
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
                  value={config.media.image.type}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      media: { ...config.media, image: { ...config.media.image, type: e.target.value as any } },
                    })
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

              {config.media.image.type === 'builtin' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Built-in Art Engine</label>
                  <select
                    value={config.media.image.builtin_name || 'procedural-art'}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: { ...config.media, image: { ...config.media.image, builtin_name: e.target.value } },
                      })
                    }
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
                  >
                    <option value="procedural-art">procedural-art (Pure-Go Vector Dark Fantasy SVG)</option>
                    <option value="echo">echo (Debug Mock)</option>
                  </select>
                </div>
              )}

              {config.media.image.type === 'http' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-sans uppercase text-stone-300">Image Endpoint URL</label>
                  <input
                    type="text"
                    placeholder="e.g. http://localhost:7860/v1/images/generations"
                    value={config.media.image.endpoint || ''}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: { ...config.media, image: { ...config.media.image, endpoint: e.target.value } },
                      })
                    }
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                  />
                </div>
              )}

              {(config.media.image.type === 'gemini' ||
                (config.media.image.type === 'builtin' && config.media.image.builtin_name === 'gemini')) && (
                <>
                  <div className="space-y-1.5 col-span-1 md:col-span-2">
                    <label className="text-xs font-sans uppercase text-stone-300">Gemini / Imagen Model</label>
                    <input
                      type="text"
                      placeholder="e.g. imagen-3.0-generate-002 or gemini-3.1-flash-image"
                      value={config.media.image.model || 'imagen-3.0-generate-002'}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: { ...config.media, image: { ...config.media.image, model: e.target.value } },
                        })
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
                            setConfig({
                              ...config,
                              media: { ...config.media, image: { ...config.media.image, model: m.id } },
                            })
                          }
                          className={`px-2 py-0.5 text-[10px] font-mono rounded border transition-colors cursor-pointer ${
                            (config.media.image.model || 'imagen-3.0-generate-002') === m.id
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
                      value={config.media.image.aspect_ratio || '16:9'}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: { ...config.media, image: { ...config.media.image, aspect_ratio: e.target.value } },
                        })
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
                      value={config.media.image.person_generation || 'ALLOW_ADULT'}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: { ...config.media, image: { ...config.media.image, person_generation: e.target.value } },
                        })
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
                        <span className="text-[10px] text-emerald-400 font-mono">Shared key active</span>
                      )}
                    </label>
                    <input
                      type="password"
                      placeholder={config.providers?.gemini?.api_key ? 'Using shared key (leave blank)' : 'Optional override or GEMINI_API_KEY env'}
                      value={config.media.image.api_key || ''}
                      onChange={(e) =>
                        setConfig({
                          ...config,
                          media: { ...config.media, image: { ...config.media.image, api_key: e.target.value } },
                        })
                      }
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                    />
                  </div>
                </>
              )}
            </div>

            {config.media.image.type !== 'disabled' && (
              <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
                <button
                  onClick={() => handleTestProvider('image', config.media.image)}
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
        </div>
      )}

      {/* Tab 4: Preferences & Appearance */}
      {activeSubTab === 'preferences' && (
        <div className="space-y-4 flex-1 overflow-y-auto pr-1">
          <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
            <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
              <Sliders className="w-4 h-4" />
              <span>App Preferences & Appearance</span>
            </h3>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <label className="text-xs font-sans uppercase text-stone-300">Cinematic Backdrop Overlays</label>
                <p className="text-[11px] text-stone-400">
                  Enable atmospheric vignette darkening and cinematic film noise textures.
                </p>
                <button
                  type="button"
                  onClick={() =>
                    setConfig({
                      ...config,
                      preferences: {
                        ...config.preferences,
                        cinematic_effects: !config.preferences.cinematic_effects,
                      },
                    })
                  }
                  className={`px-4 py-2 rounded-xl text-xs font-sans font-bold transition-all cursor-pointer border ${
                    config.preferences.cinematic_effects
                      ? 'bg-purple-600/30 border-purple-500 text-purple-300'
                      : 'bg-stone-900 border-stone-800 text-stone-400'
                  }`}
                >
                  {config.preferences.cinematic_effects ? 'Enabled' : 'Disabled'}
                </button>
              </div>

              <div className="space-y-2">
                <label className="text-xs font-sans uppercase text-stone-300">Typography Scaling</label>
                <p className="text-[11px] text-stone-400">Select font scaling across story chronicles and dialogue.</p>
                <div className="flex gap-2">
                  {(['small', 'medium', 'large'] as const).map((scale) => (
                    <button
                      key={scale}
                      type="button"
                      onClick={() =>
                        setConfig({
                          ...config,
                          preferences: { ...config.preferences, font_scale: scale },
                        })
                      }
                      className={`px-3 py-1.5 rounded-lg text-xs font-sans capitalize transition-all cursor-pointer border ${
                        config.preferences.font_scale === scale
                          ? 'bg-purple-600 text-white font-bold border-purple-500 shadow'
                          : 'bg-stone-900 border-stone-800 text-stone-400 hover:text-stone-200'
                      }`}
                    >
                      {scale}
                    </button>
                  ))}
                </div>
              </div>

              <div className="space-y-2">
                <label className="text-xs font-sans uppercase text-stone-300">Story Token Streaming</label>
                <p className="text-[11px] text-stone-400">
                  Stream narrative text word-by-word as generated by the storyteller model.
                </p>
                <button
                  type="button"
                  onClick={() =>
                    setConfig({
                      ...config,
                      preferences: {
                        ...config.preferences,
                        streaming: !config.preferences.streaming,
                      },
                    })
                  }
                  className={`px-4 py-2 rounded-xl text-xs font-sans font-bold transition-all cursor-pointer border ${
                    config.preferences.streaming
                      ? 'bg-purple-600/30 border-purple-500 text-purple-300'
                      : 'bg-stone-900 border-stone-800 text-stone-400'
                  }`}
                >
                  {config.preferences.streaming ? 'Streaming Enabled' : 'Instant Render'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Tab 5: Debug (developer view over the trace) */}
      {activeSubTab === 'debug' && <DebugPanel config={config} setConfig={setConfig} />}

      {missingModelPrompt && (
        <ModelDownloadModal
          modelId={missingModelPrompt.id}
          modelName={missingModelPrompt.name}
          sizeBytes={missingModelPrompt.sizeBytes}
          onClose={() => setMissingModelPrompt(null)}
        />
      )}

      {isCatalogModalOpen && (
        <VoiceCatalogModal
          isOpen={isCatalogModalOpen}
          onClose={() => setIsCatalogModalOpen(false)}
          voices={inspect?.catalog.voices ?? []}
          providerKey={inspect?.provider_key || 'tts'}
          onAddProfile={(newProfile) => {
            if (config) {
              const current = config.media.tts.voice_profiles || [];
              setConfig({
                ...config,
                media: {
                  ...config.media,
                  tts: {
                    ...config.media.tts,
                    voice_profiles: [...current, newProfile],
                  },
                },
              });
            }
          }}
        />
      )}
    </div>
  );
};
