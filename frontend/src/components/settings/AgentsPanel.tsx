import React, { useEffect, useState } from 'react';
import { Cpu, Layers, Sliders, Users } from 'lucide-react';
import type { AgentRoleConfig, AppConfig, ProviderDescriptor, TestProviderResponse } from '../../types';
import { APIClient } from '../../api/client';
import { ProviderManager, type RoleListItem } from '../ProviderManager';
import { ROLE_LABELS } from '../../lib/roleLabels';
import { LLMRoleEditor } from '../providers/LLMRoleEditor';
import { TierLegend, tierLabel } from '../providers/TierBadge';
import { SubTabs } from '../ui/SubTabs';
import { ContextLimitsPanel } from './ContextLimitsPanel';
import { GenerationLimitsPanel } from './GenerationLimitsPanel';

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
          ...(advanced ? [{ id: 'advanced', label: 'Advanced', icon: <Sliders className="w-3.5 h-3.5" /> }] : []),
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
                <option value="" disabled>
                  ⚡ Load Preset...
                </option>
                {Object.entries(agentPresets).map(([id, p]) => (
                  <option key={id} value={id} title={p.caveat}>
                    {p.label}
                    {p.tier ? ` · ${tierLabel(p.tier)}` : ''}
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

          <LLMRoleEditor
            config={config}
            setConfig={setConfig}
            role={selectedRole}
            roleConfig={currentRoleConfig}
            roleNames={roleNames}
            updateRole={updateRole}
            advanced={advanced}
            geminiModels={geminiModels}
            fetchingGeminiModels={fetchingGeminiModels}
            geminiModelError={geminiModelError}
            fetchGeminiModels={fetchGeminiModels}
            onTest={handleTestProvider}
            testingCategory={testingCategory}
            testResult={testResult}
          />
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
