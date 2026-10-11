import React, { useEffect, useState } from 'react';
import { AlertCircle, CheckCircle, Cpu, Layers, Sliders, Users, Zap } from 'lucide-react';
import type { AgentRoleConfig, AppConfig, ProviderDescriptor, TestProviderResponse } from '../../types';
import { APIClient } from '../../api/client';
import { ProviderManager, type RoleListItem } from '../ProviderManager';
import { TierLegend, tierLabel } from '../providers/TierBadge';
import { SubTabs } from '../ui/SubTabs';
import { AdvancedSection } from '../ui/AdvancedSection';
import { ContextLimitsPanel } from './ContextLimitsPanel';
import { GenerationLimitsPanel } from './GenerationLimitsPanel';

const ROLE_LABELS: Record<string, string> = {
  gm: 'Game Master (GM / Storyteller)',
  narrator: 'Atmospheric Narrator',
  extractor: 'Entity Extractor (per-turn world state)',
};

// Suggested Gemini models for narration. This is a fallback used before the
// live catalogue is fetched, so the editor is never empty and offline.
const GEMINI_AGENT_MODELS = [
  'gemini-3.8-flash',
  'gemini-3.7-flash',
  'gemini-3.5-flash',
  'gemini-3.5-flash-lite',
  'gemini-3.1-flash-lite',
  'gemini-3.1-pro-preview',
  'gemini-3-flash-preview',
  'gemini-2.5-flash',
  'gemini-2.5-pro',
  'gemini-2.5-flash-lite',
  'gemma-4-31b-it',
  'gemma-4-26b-it',
  'gemma-4-12b-it',
  'gemma-3-27b-it',
];

// narrationModels narrows a live Gemini catalogue to models that can actually
// narrate a turn: text-out models, with image/audio/embedding variants dropped.
const narrationModels = (models: { id: string; supported_actions?: string[] }[]): string[] =>
  models
    .filter((m) => {
      const id = m.id.toLowerCase();
      if (/(tts|image|embedding|aqa|imagen|veo|robotics|learnlm)/.test(id)) return false;
      const actions = (m.supported_actions ?? []).join(' ').toLowerCase();
      if (actions && !actions.includes('generatecontent')) return false;
      return true;
    })
    .map((m) => m.id);

// A missing role falls back to inheriting gm for the extractor, which is what
// makes extraction work out of the box without a second configuration step.
const defaultRoleConfig = (role: string): AgentRoleConfig =>
  role === 'extractor' ? { type: 'inherit', inherit_from: 'gm' } : { type: 'disabled' };

// descriptorIDForRole maps a role's resolved adapter to its catalogue key, so
// the LLM role list can reuse the same label and tier vocabulary as the media
// families.
const descriptorIDForRole = (role: AgentRoleConfig | undefined): string | undefined => {
  if (!role) return undefined;
  if (role.type === 'gemini') return 'llm:gemini';
  if (role.type === 'http') return 'llm:openaichat';
  if (role.type === 'cli') return 'llm:cli';
  if (role.type === 'builtin' && role.builtin_name !== 'echo') return 'llm:narrative-oracle';
  return undefined;
};

// roleKeyPresent reports whether a role's adapter has the key it needs, counting
// the shared provider key as the role's own when it has no override.
const roleKeyPresent = (role: AgentRoleConfig, providers: AppConfig['providers']): boolean => {
  if (role.api_key) return true;
  if (role.type === 'gemini') return Boolean(providers?.gemini?.api_key);
  return false;
};

type PresetMap<T> = Record<string, { label: string; description: string; config: T; tier?: string; caveat?: string }>;

interface AgentsPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
  advanced: boolean;
  agentPresets: PresetMap<AgentRoleConfig>;
  llmDescriptors: ProviderDescriptor[];
  tierCaveats: Record<string, string>;
  handleTestProvider: (category: 'llm' | 'tts' | 'stt' | 'image', provider: unknown, testPrompt?: string) => void;
  testingCategory: string | null;
  testResult: { category: string; res: TestProviderResponse } | null;
}

// AgentsPanel owns everything about which model plays which role: the per-role
// editor, the routing list, and the limits that most people never touch. The
// role and sub-tab selection are its own state, so the studio shell does not
// have to carry them.
export const AgentsPanel: React.FC<AgentsPanelProps> = ({
  config,
  setConfig,
  advanced,
  agentPresets,
  llmDescriptors,
  tierCaveats,
  handleTestProvider,
  testingCategory,
  testResult,
}) => {
  const [agentsTab, setAgentsTab] = useState<'roles' | 'routing' | 'advanced'>('roles');
  const [selectedRole, setSelectedRole] = useState<string>('gm');

  // Gemini agent model suggestions. Null means show the static fallback; a
  // successful catalogue fetch replaces it with the account's live list.
  const [geminiModels, setGeminiModels] = useState<string[] | null>(null);
  const [fetchingGeminiModels, setFetchingGeminiModels] = useState(false);
  const [geminiModelError, setGeminiModelError] = useState<string | null>(null);

  // Turning the preference off has to leave the Advanced sub-tab, or the tab
  // bar would offer nothing to return to.
  useEffect(() => {
    if (!advanced) setAgentsTab((tab) => (tab === 'advanced' ? 'roles' : tab));
  }, [advanced]);

  const fetchGeminiModels = async () => {
    setFetchingGeminiModels(true);
    setGeminiModelError(null);
    try {
      const res = await APIClient.listGeminiModels();
      if (res.error) {
        setGeminiModelError(res.error);
      } else {
        const ids = narrationModels(res.models);
        setGeminiModels(ids.length > 0 ? ids : null);
      }
    } catch (err) {
      setGeminiModelError(err instanceof Error ? err.message : 'Failed to load the model catalogue');
    } finally {
      setFetchingGeminiModels(false);
    }
  };

  const currentRoleConfig: AgentRoleConfig = config.agents.roles[selectedRole] || defaultRoleConfig(selectedRole);

  const roleNames = Array.from(new Set([...Object.keys(config.agents.roles), 'extractor']));

  // The LLM role list mirrors the media families: one row per role, labelled
  // with the catalogue descriptor of the adapter it resolves to.
  const descriptorByID: Record<string, ProviderDescriptor> = {};
  for (const descriptor of llmDescriptors) descriptorByID[descriptor.id] = descriptor;
  const resolveRole = (role: string, seen: Set<string>): AgentRoleConfig | undefined => {
    const roleConfig = config.agents.roles[role];
    if (!roleConfig) return undefined;
    if (roleConfig.type === 'inherit' && roleConfig.inherit_from && !seen.has(roleConfig.inherit_from)) {
      seen.add(role);
      return resolveRole(roleConfig.inherit_from, seen) ?? roleConfig;
    }
    return roleConfig;
  };
  const roleListItems: RoleListItem[] = Object.keys(config.agents.roles).map((role) => {
    const resolved = resolveRole(role, new Set()) ?? config.agents.roles[role];
    const descriptor = descriptorByID[descriptorIDForRole(resolved) ?? ''];
    const keyRequired = Boolean(descriptor?.features.includes('key_required'));
    return {
      name: role,
      label: ROLE_LABELS[role] ?? role,
      providerLabel: descriptor?.label,
      tier: descriptor?.tier,
      caveat: descriptor?.caveat,
      keyRequired,
      keyPresent: keyRequired ? roleKeyPresent(resolved, config.providers) : false,
    };
  });

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
  <div className="space-y-4 flex-1 overflow-y-auto pr-1">
    <SubTabs
      ariaLabel="AI Agents"
      tabs={[
        { id: 'roles', label: 'Roles', icon: <Users className="w-3.5 h-3.5" /> },
        { id: 'routing', label: 'Routing', icon: <Layers className="w-3.5 h-3.5" /> },
        ...(advanced
          ? [{ id: 'advanced', label: 'Advanced', icon: <Sliders className="w-3.5 h-3.5" /> }]
          : []),
      ]}
      active={agentsTab}
      onChange={(id) => setAgentsTab(id as 'roles' | 'routing' | 'advanced')}
    />
    <label className="flex items-center justify-between gap-3 p-4 rounded-xl bg-glass-card border border-stone-800 cursor-pointer">
      <span className="text-sm font-sans text-stone-200">
        Restate the player's action
        <span className="block text-xs text-stone-400 font-normal">
          The narrator opens each turn by restating what the player did in the third person.
        </span>
      </span>
      <input
        type="checkbox"
        checked={config.agents.action_echo !== false}
        onChange={(e) => setConfig({ ...config, agents: { ...config.agents, action_echo: e.target.checked } })}
        className="w-4 h-4 accent-purple-600 cursor-pointer"
      />
    </label>

    {agentsTab === 'roles' && (
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
              if (key && agentPresets[key]) {
                const preset = agentPresets[key].config;
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
            {Object.entries(agentPresets).map(([id, p]) => (
              <option key={id} value={id} title={p.caveat}>
                {p.label}{p.tier ? ` · ${tierLabel(p.tier)}` : ''}
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
              <p className="text-xs text-stone-500">
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
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Gemini Model</span>
                  <button
                    type="button"
                    onClick={fetchGeminiModels}
                    disabled={fetchingGeminiModels}
                    className="text-xs font-sans px-2 py-0.5 rounded border border-purple-500/40 text-purple-300 hover:bg-purple-600/20 disabled:opacity-50 cursor-pointer"
                  >
                    {fetchingGeminiModels ? 'Loading...' : 'Fetch available models'}
                  </button>
                </label>
                <input
                  type="text"
                  placeholder="e.g. gemini-3.8-flash"
                  value={currentRoleConfig.model || 'gemini-3.8-flash'}
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
                  {(geminiModels ?? GEMINI_AGENT_MODELS).map((m) => (
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
                      className={`px-2 py-0.5 text-xs font-mono rounded border transition-colors cursor-pointer ${
                        (currentRoleConfig.model || 'gemini-3.8-flash') === m
                          ? 'bg-purple-500/20 border-purple-500/50 text-purple-300'
                          : 'bg-stone-950 border-stone-800 text-stone-400 hover:text-stone-200'
                      }`}
                    >
                      {m}
                    </button>
                  ))}
                </div>
                {geminiModels ? (
                  <p className="text-xs text-stone-500">
                    Showing {geminiModels.length} models available to your key.
                  </p>
                ) : (
                  <p className="text-xs text-stone-500">
                    Suggested models shown; fetch the catalogue to list everything your key can use.
                  </p>
                )}
                {geminiModelError && (
                  <p className="text-xs text-red-400 font-mono">{geminiModelError}</p>
                )}
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                  <span>Role API Key Override</span>
                  {config.providers?.gemini?.api_key && (
                    <span className="text-xs text-emerald-400 font-mono">Shared key active (from Providers tab)</span>
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

            </>
          )}
        </div>

        {currentRoleConfig.type !== 'disabled' && (
          <AdvancedSection label="Advanced" defaultOpen={advanced}>
            {(currentRoleConfig.type === 'gemini' || (currentRoleConfig.type === 'builtin' && currentRoleConfig.builtin_name === 'gemini')) && (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
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
                  <p className="text-xs text-stone-500">
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
              </div>
            )}

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
                <p className="text-xs text-stone-500">
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
                <p className="text-xs text-stone-500">Lower is steadier, which helps long-run continuity.</p>
              </div>
            </div>
          </AdvancedSection>
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
            <p className="text-xs text-stone-500">
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
    )}

    {agentsTab === 'routing' && (
    <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
      <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
        <Layers className="w-4 h-4" />
        <span>Role Routing</span>
      </h3>
      <p className="text-xs text-stone-400">
        Every role resolves to one adapter. Select a row to edit that role on the Roles tab.
      </p>

      {Object.keys(tierCaveats).length > 0 && (
        <div className="rounded-lg border border-stone-800 bg-stone-950/40 p-3">
          <p className="mb-2 font-sans text-[11px] uppercase text-stone-400">How providers run</p>
          <TierLegend caveats={tierCaveats} />
        </div>
      )}

      <ProviderManager
        family="llm"
        config={config}
        onChange={setConfig}
        selected={selectedRole}
        onSelect={setSelectedRole}
        roles={roleListItems}
      />
    </div>
    )}

    {agentsTab === 'advanced' && (
    <>
    {/* Context budget and timing: what the narrator is sent, and how long it may take */}
    <ContextLimitsPanel config={config} setConfig={setConfig} advanced={advanced} />

    <GenerationLimitsPanel config={config} setConfig={setConfig} advanced={advanced} />
    </>
    )}
  </div>
  );
};

export default AgentsPanel;
