import React from 'react';
import { Sliders } from 'lucide-react';
import type { AppConfig } from '../../types';
import { AdvancedSection } from '../ui/AdvancedSection';

interface ContextLimitsPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
  advanced: boolean;
}

// ContextLimitsPanel owns what the narrator is sent and how long a turn may
// take: the budget, the recall and retrieval windows, and the tool rounds.
export const ContextLimitsPanel: React.FC<ContextLimitsPanelProps> = ({ config, setConfig, advanced }) => (
  <AdvancedSection label="Advanced" defaultOpen={advanced}>
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
          <p className="text-xs text-stone-500">
            Estimated ceiling for the assembled prompt. 0 sends everything. When it is exceeded, the voice catalogue and
            the oldest remembered turns are dropped first; rules, lore, the scene, and your action are never dropped.
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
          <p className="text-xs text-stone-500">
            How many prior turns are replayed to the narrator. A larger window means better continuity and a larger
            prompt.
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
          <p className="text-xs text-stone-500">Cap on the text recalled from any one prior turn.</p>
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
          <p className="text-xs text-stone-500">What happened where the party is standing.</p>
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
          <p className="text-xs text-stone-500">Cap on the excerpt taken from one recalled turn.</p>
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
          <p className="text-xs text-stone-500">
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
          <p className="text-xs text-stone-500">Cap on the excerpt taken from one retrieved turn.</p>
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
          <p className="text-xs text-stone-500">
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
          <p className="text-xs text-stone-500">
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
          <p className="text-xs text-stone-500">
            How long the narrator may go quiet between chunks before the turn fails.
          </p>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Tool Rounds / Turn</label>
            <input
              type="number"
              min={0}
              max={100}
              value={config.agents.tool_rounds ?? 0}
              onChange={(e) => {
                const parsed = parseInt(e.target.value, 10);
                setConfig({
                  ...config,
                  agents: { ...config.agents, tool_rounds: Number.isNaN(parsed) ? 0 : parsed },
                });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <p className="text-xs text-stone-500">
              How many times a turn may look something up before answering (0 = unbounded).
            </p>
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
            <p className="text-xs text-stone-500">The most of one lookup the model is shown at once.</p>
          </div>
        </div>
      </div>
    </div>
  </AdvancedSection>
);

export default ContextLimitsPanel;
