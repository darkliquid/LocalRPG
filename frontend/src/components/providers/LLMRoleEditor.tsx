import type { TestProviderRequest } from '../../types';
import React from 'react';
import { AlertCircle, CheckCircle, Zap } from 'lucide-react';
import type { AgentRoleConfig, AppConfig, TestProviderResponse } from '../../types';
import { AdvancedSection } from '../ui/AdvancedSection';
import { ROLE_LABELS } from '../../lib/roleLabels';
import { DEFAULT_GEMINI_AGENT_MODELS } from '../../lib/constants';

interface LLMRoleEditorProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
  role: string;
  roleConfig: AgentRoleConfig;
  roleNames: string[];
  updateRole: (updated: AgentRoleConfig) => void;
  advanced: boolean;
  geminiModels: string[] | null;
  fetchingGeminiModels: boolean;
  geminiModelError: string | null;
  fetchGeminiModels: () => void;
  onTest: (
    category: 'llm' | 'tts' | 'stt' | 'image',
    provider: TestProviderRequest['provider'],
    testPrompt?: string,
  ) => void;
  testingCategory: string | null;
  testResult: { category: string; res: TestProviderResponse } | null;
}

// LLMRoleEditor edits one role's own settings: which adapter plays it, the
// model and key that adapter uses, whether it may look things up, and the
// sampling and reasoning fields behind the Advanced disclosure.
export const LLMRoleEditor: React.FC<LLMRoleEditorProps> = ({
  config,
  setConfig,
  role,
  roleConfig,
  roleNames,
  updateRole,
  advanced,
  geminiModels,
  fetchingGeminiModels,
  geminiModelError,
  fetchGeminiModels,
  onTest,
  testingCategory,
  testResult,
}) => (
  <div className="space-y-4 pt-2">
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase text-stone-300">Provider Type</label>
        <select
          value={roleConfig.type}
          onChange={(e) => {
            const updated = { ...roleConfig, type: e.target.value as (typeof roleConfig)['type'] };
            setConfig({
              ...config,
              agents: {
                ...config.agents,
                roles: { ...config.agents.roles, [role]: updated },
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
          <option value="openai">OpenAI (Responses API)</option>
          <option value="inherit">Inherit from another role</option>
        </select>
      </div>

      {roleConfig.type === 'inherit' && (
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Inherit From</label>
          <select
            value={roleConfig.inherit_from || 'gm'}
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
            {config.agents.roles[roleConfig.inherit_from || 'gm']?.model ||
              config.agents.roles[roleConfig.inherit_from || 'gm']?.command ||
              'the default provider'}
            , so changing that role changes this one until you override it here.
          </p>
        </div>
      )}

      {(roleConfig.type === 'http' || roleConfig.type === 'openai') && (
        <>
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Endpoint URL</label>
            <input
              type="text"
              placeholder="e.g. http://localhost:11434/v1"
              value={roleConfig.endpoint || ''}
              onChange={(e) => {
                const updated = { ...roleConfig, endpoint: e.target.value };
                setConfig({
                  ...config,
                  agents: {
                    ...config.agents,
                    roles: { ...config.agents.roles, [role]: updated },
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
              value={roleConfig.model || ''}
              onChange={(e) => {
                const updated = { ...roleConfig, model: e.target.value };
                setConfig({
                  ...config,
                  agents: {
                    ...config.agents,
                    roles: { ...config.agents.roles, [role]: updated },
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
              value={roleConfig.api_key || ''}
              onChange={(e) => {
                const updated = { ...roleConfig, api_key: e.target.value };
                setConfig({
                  ...config,
                  agents: {
                    ...config.agents,
                    roles: { ...config.agents.roles, [role]: updated },
                  },
                });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
        </>
      )}

      {roleConfig.type === 'cli' && (
        <>
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">CLI Command / Binary</label>
            <input
              type="text"
              placeholder="e.g. echo or /usr/local/bin/llama-cli"
              value={roleConfig.command || ''}
              onChange={(e) => {
                const updated = { ...roleConfig, command: e.target.value };
                setConfig({
                  ...config,
                  agents: {
                    ...config.agents,
                    roles: { ...config.agents.roles, [role]: updated },
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
              value={(roleConfig.args || []).join(', ')}
              onChange={(e) => {
                const args = e.target.value
                  .split(',')
                  .map((s) => s.trim())
                  .filter(Boolean);
                const updated = { ...roleConfig, args };
                setConfig({
                  ...config,
                  agents: {
                    ...config.agents,
                    roles: { ...config.agents.roles, [role]: updated },
                  },
                });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
        </>
      )}

      {roleConfig.type === 'builtin' && (
        <div className="space-y-1.5">
          <label className="text-xs font-sans uppercase text-stone-300">Builtin Engine</label>
          <select
            value={roleConfig.builtin_name || 'narrative-oracle'}
            onChange={(e) => {
              const updated = { ...roleConfig, builtin_name: e.target.value };
              setConfig({
                ...config,
                agents: {
                  ...config.agents,
                  roles: { ...config.agents.roles, [role]: updated },
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

      {(roleConfig.type === 'gemini' || (roleConfig.type === 'builtin' && roleConfig.builtin_name === 'gemini')) && (
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
              value={roleConfig.model || 'gemini-3.8-flash'}
              onChange={(e) => {
                const updated = { ...roleConfig, model: e.target.value };
                setConfig({
                  ...config,
                  agents: {
                    ...config.agents,
                    roles: { ...config.agents.roles, [role]: updated },
                  },
                });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <div className="flex flex-wrap gap-1.5 pt-1">
              {(geminiModels ?? DEFAULT_GEMINI_AGENT_MODELS).map((m) => (
                <button
                  key={m}
                  type="button"
                  onClick={() => {
                    const updated = { ...roleConfig, model: m };
                    setConfig({
                      ...config,
                      agents: {
                        ...config.agents,
                        roles: { ...config.agents.roles, [role]: updated },
                      },
                    });
                  }}
                  className={`px-2 py-0.5 text-xs font-mono rounded border transition-colors cursor-pointer ${
                    (roleConfig.model || 'gemini-3.8-flash') === m
                      ? 'bg-purple-500/20 border-purple-500/50 text-purple-300'
                      : 'bg-stone-950 border-stone-800 text-stone-400 hover:text-stone-200'
                  }`}
                >
                  {m}
                </button>
              ))}
            </div>
            {geminiModels ? (
              <p className="text-xs text-stone-500">Showing {geminiModels.length} models available to your key.</p>
            ) : (
              <p className="text-xs text-stone-500">
                Suggested models shown; fetch the catalogue to list everything your key can use.
              </p>
            )}
            {geminiModelError && <p className="text-xs text-red-400 font-mono">{geminiModelError}</p>}
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
              placeholder={
                config.providers?.gemini?.api_key
                  ? 'Using shared key from Providers tab (leave blank)'
                  : 'Optional override or GEMINI_API_KEY env'
              }
              value={roleConfig.api_key || ''}
              onChange={(e) => {
                const updated = { ...roleConfig, api_key: e.target.value };
                setConfig({
                  ...config,
                  agents: {
                    ...config.agents,
                    roles: { ...config.agents.roles, [role]: updated },
                  },
                });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
          </div>
        </>
      )}
    </div>

    {roleConfig.type !== 'disabled' && (
      <AdvancedSection label="Advanced" defaultOpen={advanced}>
        {(roleConfig.type === 'gemini' || (roleConfig.type === 'builtin' && roleConfig.builtin_name === 'gemini')) && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-1.5 col-span-1 md:col-span-2">
              <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                <span>Thinking / Reasoning Budget</span>
                <span className="font-mono text-purple-400">
                  {roleConfig.thinking_budget === undefined
                    ? 'Default'
                    : roleConfig.thinking_budget === 0
                      ? '0 (Disabled — Instant Narration)'
                      : roleConfig.thinking_budget === -1
                        ? 'Dynamic (-1 — Model Decides)'
                        : `${roleConfig.thinking_budget} tokens`}
                </span>
              </label>
              <div className="flex gap-2">
                <select
                  value={
                    roleConfig.thinking_budget === undefined
                      ? 'default'
                      : roleConfig.thinking_budget === 0
                        ? '0'
                        : roleConfig.thinking_budget === -1
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

                    const updated = { ...roleConfig, thinking_budget: budget };
                    setConfig({
                      ...config,
                      agents: {
                        ...config.agents,
                        roles: { ...config.agents.roles, [role]: updated },
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
                {(roleConfig.thinking_budget ?? 0) > 0 && (
                  <input
                    type="number"
                    min={128}
                    step={256}
                    value={roleConfig.thinking_budget}
                    onChange={(e) => {
                      const budget = parseInt(e.target.value, 10);
                      const updated = { ...roleConfig, thinking_budget: isNaN(budget) ? 0 : budget };
                      setConfig({
                        ...config,
                        agents: {
                          ...config.agents,
                          roles: { ...config.agents.roles, [role]: updated },
                        },
                      });
                    }}
                    className="w-32 bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                  />
                )}
              </div>
              <p className="text-xs text-stone-500">
                Thinking tokens allow Gemini 2.5 to reason deeply before replying. Thought tokens are automatically
                filtered from the story chronicle.
              </p>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                <span>Top-P</span>
                <span className="font-mono text-purple-400">{(roleConfig.top_p ?? 0.95).toFixed(2)}</span>
              </label>
              <input
                type="number"
                min={0}
                max={1}
                step={0.05}
                value={roleConfig.top_p ?? 0.95}
                onChange={(e) => {
                  const top_p = parseFloat(e.target.value);
                  const updated = { ...roleConfig, top_p: isNaN(top_p) ? undefined : top_p };
                  setConfig({
                    ...config,
                    agents: {
                      ...config.agents,
                      roles: { ...config.agents.roles, [role]: updated },
                    },
                  });
                }}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
                <span>Top-K</span>
                <span className="font-mono text-purple-400">{roleConfig.top_k ?? 40}</span>
              </label>
              <input
                type="number"
                min={1}
                max={100}
                step={1}
                value={roleConfig.top_k ?? 40}
                onChange={(e) => {
                  const top_k = parseInt(e.target.value, 10);
                  const updated = { ...roleConfig, top_k: isNaN(top_k) ? undefined : top_k };
                  setConfig({
                    ...config,
                    agents: {
                      ...config.agents,
                      roles: { ...config.agents.roles, [role]: updated },
                    },
                  });
                }}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
              />
            </div>
          </div>
        )}

        {(roleConfig.type === 'openai' || roleConfig.type === 'anthropic') && (
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Reasoning Effort</label>
            <select
              value={roleConfig.effort || ''}
              onChange={(e) => {
                const updated = { ...roleConfig, effort: e.target.value || undefined };
                setConfig({
                  ...config,
                  agents: {
                    ...config.agents,
                    roles: { ...config.agents.roles, [role]: updated },
                  },
                });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
            >
              <option value="">Model default</option>
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
              <option value="xhigh">xhigh</option>
              <option value="max">max</option>
            </select>
            <p className="text-xs text-stone-500">
              When set, the request switches to the reasoning shape and temperature is omitted.
            </p>
          </div>
        )}

        {roleConfig.type === 'anthropic' && (
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Prompt Caching</label>
            <select
              value={roleConfig.prompt_cache || 'auto'}
              onChange={(e) => {
                const updated = { ...roleConfig, prompt_cache: e.target.value === 'auto' ? undefined : e.target.value };
                setConfig({
                  ...config,
                  agents: {
                    ...config.agents,
                    roles: { ...config.agents.roles, [role]: updated },
                  },
                });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
            >
              <option value="auto">auto (recommended)</option>
              <option value="off">off</option>
            </select>
            <p className="text-xs text-stone-500">
              Auto sends Anthropic's cache_control so repeated narration context reads at a fraction of input cost.
            </p>
          </div>
        )}

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
              <span>Response Limit (max tokens)</span>
              <span className="font-mono text-purple-400">{roleConfig.max_tokens ?? 1024}</span>
            </label>
            <input
              type="number"
              min={64}
              step={64}
              value={roleConfig.max_tokens ?? 1024}
              onChange={(e) => {
                const max_tokens = parseInt(e.target.value, 10);
                const updated = { ...roleConfig, max_tokens: Number.isNaN(max_tokens) ? undefined : max_tokens };
                setConfig({
                  ...config,
                  agents: { ...config.agents, roles: { ...config.agents.roles, [role]: updated } },
                });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <p className="text-xs text-stone-500">
              How long a single reply may be. Raise it for longer scenes; the reply is marked as cut off when it hits
              this.
            </p>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
              <span>Temperature</span>
              <span className="font-mono text-purple-400">{(roleConfig.temperature ?? 0.7).toFixed(2)}</span>
            </label>
            <input
              type="range"
              min="0"
              max="1.5"
              step="0.05"
              value={roleConfig.temperature ?? 0.7}
              onChange={(e) => {
                const updated = { ...roleConfig, temperature: parseFloat(e.target.value) };
                setConfig({
                  ...config,
                  agents: { ...config.agents, roles: { ...config.agents.roles, [role]: updated } },
                });
              }}
              className="w-full accent-purple-500"
            />
            <p className="text-xs text-stone-500">Lower is steadier, which helps long-run continuity.</p>
          </div>
        </div>
      </AdvancedSection>
    )}

    {roleConfig.type !== 'inherit' && (
      <div className="space-y-1.5">
        <label className="text-xs font-sans uppercase text-stone-300">Tool Calling</label>
        <select
          value={roleConfig.supports_tools ?? 'auto'}
          onChange={(e) => {
            const updated = { ...roleConfig, supports_tools: e.target.value as 'auto' | 'yes' | 'no' };
            setConfig({
              ...config,
              agents: { ...config.agents, roles: { ...config.agents.roles, [role]: updated } },
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

    {roleConfig.type !== 'disabled' && (
      <div className="pt-2 flex items-center justify-between border-t border-stone-800/60">
        <button
          onClick={() => onTest('llm', roleConfig)}
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
);

export default LLMRoleEditor;
