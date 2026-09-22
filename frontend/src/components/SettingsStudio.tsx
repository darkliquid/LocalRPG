import React, { useState, useEffect, useRef } from 'react';
import { DebugPanel } from './DebugPanel';
import { APIClient } from '../api/client';
import { AppConfig, AgentRoleConfig, TestProviderResponse, VoiceProfile } from '../types';
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
} from 'lucide-react';
import {
  AGENT_PRESETS,
  TTS_PRESETS,
  STT_PRESETS,
  IMAGE_PRESETS,
  DEFAULT_VOICE_PROFILES,
} from '../templates/providerPresets';

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
  const [activeSubTab, setActiveSubTab] = useState<'paths' | 'agents' | 'media' | 'preferences' | 'debug'>('paths');
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // Diagnostics test state
  const [testingCategory, setTestingCategory] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ category: string; res: TestProviderResponse } | null>(null);
  const previewAudioRef = useRef<HTMLAudioElement | null>(null);
  const [ttsPreviewText, setTtsPreviewText] = useState<string>(DEFAULT_TTS_PREVIEW_TEXT);

  // Selected agent role for editing
  const [selectedRole, setSelectedRole] = useState<string>('gm');

  useEffect(() => {
    loadSettings();
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
    setTestingCategory(category);
    setTestResult(null);
    try {
      const res = await APIClient.testProvider({
        category,
        provider,
        test_prompt: testPrompt ?? (category === 'llm' ? 'Are the stars shining?' : undefined),
      });
      setTestResult({ category, res });
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
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeSubTab === 'paths' ? 'bg-amber-600 text-stone-950 font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Folder className="w-3.5 h-3.5" />
            <span>Paths</span>
          </button>
          <button
            onClick={() => setActiveSubTab('agents')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeSubTab === 'agents' ? 'bg-amber-600 text-stone-950 font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Cpu className="w-3.5 h-3.5" />
            <span>AI Agents</span>
          </button>
          <button
            onClick={() => setActiveSubTab('media')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeSubTab === 'media' ? 'bg-amber-600 text-stone-950 font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Volume2 className="w-3.5 h-3.5" />
            <span>Media Engines</span>
          </button>
          <button
            onClick={() => setActiveSubTab('preferences')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeSubTab === 'preferences' ? 'bg-amber-600 text-stone-950 font-bold shadow' : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Sliders className="w-3.5 h-3.5" />
            <span>Preferences</span>
          </button>
          <button
            onClick={() => setActiveSubTab('debug')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeSubTab === 'debug' ? 'bg-amber-600 text-stone-950 font-bold shadow' : 'text-stone-400 hover:text-stone-200'
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
            className="flex items-center gap-1.5 text-xs font-cinzel font-bold px-4 py-1.5 rounded-xl transition-all cursor-pointer bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-stone-950 shadow active:scale-95"
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
            <h3 className="font-cinzel text-sm font-bold text-amber-400 flex items-center gap-2">
              <Folder className="w-4 h-4" />
              <span>Storage & Discovery Paths</span>
            </h3>
            <p className="text-xs text-stone-400">
              Configure directories where LocalRPG looks for rule systems, world lore, saved campaigns, and generated media caches.
            </p>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300">Rule Systems Directory</label>
                <input
                  type="text"
                  value={config.paths.systems}
                  onChange={(e) => setConfig({ ...config, paths: { ...config.paths, systems: e.target.value } })}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300">Worlds Directory</label>
                <input
                  type="text"
                  value={config.paths.worlds}
                  onChange={(e) => setConfig({ ...config, paths: { ...config.paths, worlds: e.target.value } })}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300">Saved Campaigns Directory</label>
                <input
                  type="text"
                  value={config.paths.games}
                  onChange={(e) => setConfig({ ...config, paths: { ...config.paths, games: e.target.value } })}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300">Media Cache Directory</label>
                <input
                  type="text"
                  value={config.paths.cache}
                  onChange={(e) => setConfig({ ...config, paths: { ...config.paths, cache: e.target.value } })}
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
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
              <h3 className="font-cinzel text-sm font-bold text-amber-400 flex items-center gap-2">
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
                  className="bg-stone-900 border border-amber-500/30 text-amber-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
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
                  className="bg-stone-950 border border-stone-800 rounded-lg pl-2.5 pr-7 py-1 text-xs text-amber-300 font-mono focus:outline-none cursor-pointer"
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
                  <label className="text-xs font-cinzel uppercase text-stone-300">Provider Type</label>
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
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
                  >
                    <option value="disabled">Disabled / Inactive</option>
                    <option value="http">HTTP / OpenAI-Compatible (Ollama, vLLM, OpenAI)</option>
                    <option value="cli">CLI Command (Local Binary e.g. llama-cli)</option>
                    <option value="builtin">Builtin / Internal Engine</option>
                    <option value="inherit">Inherit from another role</option>
                  </select>
                </div>

                {currentRoleConfig.type === 'inherit' && (
                  <div className="space-y-1.5">
                    <label className="text-xs font-cinzel uppercase text-stone-300">Inherit From</label>
                    <select
                      value={currentRoleConfig.inherit_from || 'gm'}
                      onChange={(e) => updateRole({ type: 'inherit', inherit_from: e.target.value })}
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-amber-300 font-mono focus:outline-none cursor-pointer"
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
                      <label className="text-xs font-cinzel uppercase text-stone-300">Endpoint URL</label>
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
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                      />
                    </div>

                    <div className="space-y-1.5">
                      <label className="text-xs font-cinzel uppercase text-stone-300">Model Name</label>
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
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                      />
                    </div>

                    <div className="space-y-1.5">
                      <label className="text-xs font-cinzel uppercase text-stone-300">API Key (Optional)</label>
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
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                      />
                    </div>
                  </>
                )}

                {currentRoleConfig.type === 'cli' && (
                  <>
                    <div className="space-y-1.5">
                      <label className="text-xs font-cinzel uppercase text-stone-300">CLI Command / Binary</label>
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
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                      />
                    </div>

                    <div className="space-y-1.5">
                      <label className="text-xs font-cinzel uppercase text-stone-300">Command Arguments (comma separated)</label>
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
                        className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                      />
                    </div>
                  </>
                )}

                {currentRoleConfig.type === 'builtin' && (
                  <div className="space-y-1.5">
                    <label className="text-xs font-cinzel uppercase text-stone-300">Builtin Engine</label>
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
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
                    >
                      <option value="narrative-oracle">narrative-oracle (Deterministic Procedural Storyteller)</option>
                      <option value="echo">echo (Debug Provider)</option>
                    </select>
                  </div>
                )}
              </div>

              {currentRoleConfig.type !== 'disabled' && (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div className="space-y-1.5">
                    <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                      <span>Response Limit (max tokens)</span>
                      <span className="font-mono text-amber-400">{currentRoleConfig.max_tokens ?? 1024}</span>
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
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                    />
                    <p className="text-[11px] text-stone-500">
                      How long a single reply may be. Raise it for longer scenes; the reply is marked as cut off when it
                      hits this.
                    </p>
                  </div>

                  <div className="space-y-1.5">
                    <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                      <span>Temperature</span>
                      <span className="font-mono text-amber-400">{(currentRoleConfig.temperature ?? 0.7).toFixed(2)}</span>
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
                      className="w-full accent-amber-500"
                    />
                    <p className="text-[11px] text-stone-500">Lower is steadier, which helps long-run continuity.</p>
                  </div>
                </div>
              )}

              {currentRoleConfig.type !== 'disabled' && (
                <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
                  <button
                    onClick={() => handleTestProvider('llm', currentRoleConfig)}
                    disabled={testingCategory === 'llm'}
                    className="flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-lg bg-stone-900 border border-amber-500/30 hover:bg-stone-800 text-amber-400 transition-all cursor-pointer"
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
            <h3 className="font-cinzel text-sm font-bold text-amber-400 flex items-center gap-2">
              <Sliders className="w-4 h-4" />
              <span>Context &amp; Response Limits</span>
            </h3>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Context Budget (tokens)</span>
                  <span className="font-mono text-amber-400">
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  Estimated ceiling for the assembled prompt. 0 sends everything. When it is exceeded, the voice
                  catalogue and the oldest remembered turns are dropped first; rules, lore, the scene, and your action
                  are never dropped.
                </p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Remembered Turns</span>
                  <span className="font-mono text-amber-400">{config.agents.recent_turn_window ?? 6}</span>
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  How many prior turns are replayed to the narrator. A larger window means better continuity and a
                  larger prompt.
                </p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Excerpt Length (characters)</span>
                  <span className="font-mono text-amber-400">{config.agents.recent_turn_char_limit ?? 1200}</span>
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">Cap on the text recalled from any one prior turn.</p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Turns Recalled At This Location</span>
                  <span className="font-mono text-amber-400">{config.agents.scene_recall_turns ?? 4}</span>
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">What happened where the party is standing.</p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Recalled Excerpt Length</span>
                  <span className="font-mono text-amber-400">{config.agents.scene_recall_chars ?? 800}</span>
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">Cap on the excerpt taken from one recalled turn.</p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Turns Retrieved By Entity</span>
                  <span className="font-mono text-amber-400">{config.agents.retrieval_turns ?? 3}</span>
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  Past turns that share characters with the ones in play, wherever they happened.
                </p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Retrieved Excerpt Length</span>
                  <span className="font-mono text-amber-400">{config.agents.retrieval_chars ?? 800}</span>
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">Cap on the excerpt taken from one retrieved turn.</p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Retrieval Recency Half-Life</span>
                  <span className="font-mono text-amber-400">{config.agents.retrieval_halflife_turns ?? 12}</span>
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  Turns after which a retrieved turn's recency weight halves. Lower favours the recent.
                </p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Turn Timeout (seconds)</span>
                  <span className="font-mono text-amber-400">{config.agents.turn_timeout_seconds ?? 300}</span>
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  Wall clock for a whole turn. Raise it for slower local models and long contexts.
                </p>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Silence Timeout (seconds)</span>
                  <span className="font-mono text-amber-400">{config.agents.chunk_timeout_seconds ?? 60}</span>
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                />
                <p className="text-[11px] text-stone-500">
                  How long the narrator may go quiet between chunks before the turn fails.
                </p>
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
              <h3 className="font-cinzel text-sm font-bold text-amber-400 flex items-center gap-2">
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
                            auto_play: config.media.tts.auto_play,
                            voice_profiles: config.media.tts.voice_profiles,
                          },
                        },
                      });
                      e.target.value = '';
                    }
                  }}
                  className="bg-stone-900 border border-amber-500/30 text-amber-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
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
                    className="rounded bg-stone-950 border-stone-800 text-amber-600 focus:ring-0"
                  />
                  <span>Auto-play Narration</span>
                </label>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300">TTS Provider Type</label>
                <select
                  value={config.media.tts.type}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      media: { ...config.media, tts: { ...config.media.tts, type: e.target.value as any } },
                    })
                  }
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
                >
                  <option value="disabled">Disabled</option>
                  <option value="http">HTTP (Kokoro-FastAPI, AllTalk, OpenAI Speech)</option>
                  <option value="cli">CLI Command (e.g. piper)</option>
                  <option value="builtin">Builtin (native-os / procedural audio)</option>
                </select>
              </div>

              {config.media.tts.type === 'builtin' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-cinzel uppercase text-stone-300">Built-in Engine</label>
                  <select
                    value={config.media.tts.builtin_name || 'native-os'}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: { ...config.media, tts: { ...config.media.tts, builtin_name: e.target.value } },
                      })
                    }
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
                  >
                    <option value="native-os">native-os (OS Speech Synthesizer / Procedural Audio)</option>
                    <option value="echo">echo (Debug Mock)</option>
                  </select>
                </div>
              )}

              {config.media.tts.type === 'http' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-cinzel uppercase text-stone-300">Speech Endpoint URL</label>
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
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                  />
                </div>
              )}

              {config.media.tts.type === 'cli' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-cinzel uppercase text-stone-300">Command / Binary</label>
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
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                  />
                </div>
              )}

              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300 flex items-center justify-between">
                  <span>Master Volume</span>
                  <span className="font-mono text-amber-400">
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
                  className="w-full accent-amber-500"
                />
              </div>
            </div>

            {config.media.tts.type !== 'disabled' && (
              <div className="pt-2 space-y-2 border-t border-stone-800/60">
                <div className="space-y-1.5">
                  <label className="text-xs font-cinzel uppercase text-stone-300">
                    Preview Phrase
                  </label>
                  <input
                    type="text"
                    value={ttsPreviewText}
                    onChange={(e) => setTtsPreviewText(e.target.value)}
                    placeholder={DEFAULT_TTS_PREVIEW_TEXT}
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60"
                  />
                </div>

                <div className="flex items-center justify-between">
                  <button
                    onClick={() => handleTestProvider('tts', config.media.tts, ttsPreviewText)}
                    disabled={testingCategory === 'tts'}
                    className="flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-lg bg-stone-900 border border-amber-500/30 hover:bg-stone-800 text-amber-400 transition-all cursor-pointer"
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
                  <Users className="w-4 h-4 text-amber-400" />
                  <span className="font-cinzel text-xs uppercase font-bold text-stone-200">
                    NPC Voice Profiles Library
                  </span>
                  <span className="text-[10px] font-mono text-stone-500">
                    ({config.media.tts.voice_profiles?.length || 0} archetypes)
                  </span>
                </div>
                <div className="flex flex-wrap items-center gap-2">
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
                    className="flex items-center gap-1 text-[11px] px-2 py-1 rounded bg-stone-900 border border-stone-700 text-stone-300 hover:text-amber-300 transition cursor-pointer"
                    title="Restore default fantasy archetypes"
                  >
                    <RotateCcw className="w-3 h-3" />
                    <span>Load Fantasy Defaults</span>
                  </button>

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
                    className="flex items-center gap-1 text-[11px] px-2 py-1 rounded bg-amber-600/20 border border-amber-500/40 text-amber-300 hover:bg-amber-600/30 transition cursor-pointer"
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
                          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs font-mono text-amber-300 focus:outline-none"
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
                        <input
                          type="text"
                          placeholder="Voice ID (e.g. af_bella)"
                          value={profile.voice_id}
                          onChange={(e) => {
                            const updated = [...(config.media.tts.voice_profiles || [])];
                            updated[idx] = { ...updated[idx], voice_id: e.target.value };
                            setConfig({
                              ...config,
                              media: { ...config.media, tts: { ...config.media.tts, voice_profiles: updated } },
                            });
                          }}
                          className="bg-stone-900 border border-stone-800 rounded px-2 py-1 text-xs font-mono text-stone-200 focus:outline-none"
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
                              },
                              ttsPreviewText
                            )
                          }
                          className="p-1.5 rounded bg-stone-900 border border-stone-800 text-amber-400 hover:text-amber-300 hover:border-amber-500/40 cursor-pointer"
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
                          <span className="font-mono text-amber-400">{(profile.pitch ?? 1.0).toFixed(2)}x</span>
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
                          className="w-full accent-amber-500"
                        />
                      </div>

                      <div className="space-y-1">
                        <div className="flex justify-between text-[11px] text-stone-400">
                          <span>Speed / Speech Rate</span>
                          <span className="font-mono text-amber-400">{(profile.speech_rate ?? 1.0).toFixed(2)}x</span>
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
                          className="w-full accent-amber-500"
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
              <h3 className="font-cinzel text-sm font-bold text-amber-400 flex items-center gap-2">
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
                  className="bg-stone-900 border border-amber-500/30 text-amber-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
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
                <label className="text-xs font-cinzel uppercase text-stone-300">STT Provider Type</label>
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
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
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
                    <label className="text-xs font-cinzel uppercase text-stone-300">Transcription Endpoint URL</label>
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
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-xs font-cinzel uppercase text-stone-300">Model Name</label>
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
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                    />
                  </div>
                </>
              )}

              {config.media.stt?.type === 'cli' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-cinzel uppercase text-stone-300">Command / Binary</label>
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
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                  />
                </div>
              )}
            </div>

            {config.media.stt?.type && config.media.stt.type !== 'disabled' && (
              <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
                <button
                  onClick={() => handleTestProvider('stt', config.media.stt)}
                  disabled={testingCategory === 'stt'}
                  className="flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-lg bg-stone-900 border border-amber-500/30 hover:bg-stone-800 text-amber-400 transition-all cursor-pointer"
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
              <h3 className="font-cinzel text-sm font-bold text-amber-400 flex items-center gap-2">
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
                  className="bg-stone-900 border border-amber-500/30 text-amber-400 rounded-lg pl-2.5 pr-7 py-1 text-xs font-mono focus:outline-none cursor-pointer"
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
                    className="rounded bg-stone-950 border-stone-800 text-amber-600 focus:ring-0"
                  />
                  <span>Auto-generate Scene Art</span>
                </label>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-cinzel uppercase text-stone-300">Image Provider Type</label>
                <select
                  value={config.media.image.type}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      media: { ...config.media, image: { ...config.media.image, type: e.target.value as any } },
                    })
                  }
                  className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
                >
                  <option value="disabled">Disabled</option>
                  <option value="http">HTTP (ComfyUI, Automatic1111, LocalAI, DALL-E)</option>
                  <option value="comfyui">ComfyUI Dedicated (Port 8188)</option>
                  <option value="cli">CLI Command (e.g. sd-cli)</option>
                  <option value="builtin">Builtin (procedural-art / mock)</option>
                </select>
              </div>

              {config.media.image.type === 'builtin' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-cinzel uppercase text-stone-300">Built-in Art Engine</label>
                  <select
                    value={config.media.image.builtin_name || 'procedural-art'}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        media: { ...config.media, image: { ...config.media.image, builtin_name: e.target.value } },
                      })
                    }
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60 cursor-pointer"
                  >
                    <option value="procedural-art">procedural-art (Pure-Go Vector Dark Fantasy SVG)</option>
                    <option value="echo">echo (Debug Mock)</option>
                  </select>
                </div>
              )}

              {config.media.image.type === 'http' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-cinzel uppercase text-stone-300">Image Endpoint URL</label>
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
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-amber-500/60"
                  />
                </div>
              )}
            </div>

            {config.media.image.type !== 'disabled' && (
              <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
                <button
                  onClick={() => handleTestProvider('image', config.media.image)}
                  disabled={testingCategory === 'image'}
                  className="flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-lg bg-stone-900 border border-amber-500/30 hover:bg-stone-800 text-amber-400 transition-all cursor-pointer"
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
            <h3 className="font-cinzel text-sm font-bold text-amber-400 flex items-center gap-2">
              <Sliders className="w-4 h-4" />
              <span>App Preferences & Appearance</span>
            </h3>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="space-y-2">
                <label className="text-xs font-cinzel uppercase text-stone-300">Cinematic Backdrop Overlays</label>
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
                  className={`px-4 py-2 rounded-xl text-xs font-cinzel font-bold transition-all cursor-pointer border ${
                    config.preferences.cinematic_effects
                      ? 'bg-amber-600/30 border-amber-500 text-amber-300'
                      : 'bg-stone-900 border-stone-800 text-stone-400'
                  }`}
                >
                  {config.preferences.cinematic_effects ? 'Enabled' : 'Disabled'}
                </button>
              </div>

              <div className="space-y-2">
                <label className="text-xs font-cinzel uppercase text-stone-300">Typography Scaling</label>
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
                      className={`px-3 py-1.5 rounded-lg text-xs font-cinzel capitalize transition-all cursor-pointer border ${
                        config.preferences.font_scale === scale
                          ? 'bg-amber-600 text-stone-950 font-bold border-amber-500 shadow'
                          : 'bg-stone-900 border-stone-800 text-stone-400 hover:text-stone-200'
                      }`}
                    >
                      {scale}
                    </button>
                  ))}
                </div>
              </div>

              <div className="space-y-2">
                <label className="text-xs font-cinzel uppercase text-stone-300">Story Token Streaming</label>
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
                  className={`px-4 py-2 rounded-xl text-xs font-cinzel font-bold transition-all cursor-pointer border ${
                    config.preferences.streaming
                      ? 'bg-amber-600/30 border-amber-500 text-amber-300'
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
    </div>
  );
};
