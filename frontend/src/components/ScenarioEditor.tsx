import React, { useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import type { Scenario, ScenarioExpectations, ScenarioStep } from '../types';

interface ScenarioEditorProps {
  scenario: Scenario;
  onSave: (scenario: Scenario) => void;
  onCancel: () => void;
}

// A step that asserts nothing is a setup step, so the default is the common
// "check" action with no expectations.
const EMPTY_STEP: ScenarioStep = { action: 'check', input: '', expect: {} };

// numberOrUndefined treats an empty field as "unset", so a blank total does not
// become a zero bound.
function numberOrUndefined(value: string): number | undefined {
  const trimmed = value.trim();
  if (trimmed === '') return undefined;
  const parsed = Number(trimmed);
  return Number.isFinite(parsed) ? parsed : undefined;
}

// scenarioStatValue keeps a numeric statistic a number and anything else a string,
// so "Might 3" reads as a number and "cursed" reads as a string.
function scenarioStatValue(value: string): unknown {
  const trimmed = value.trim();
  if (trimmed === '') return undefined;
  const parsed = Number(trimmed);
  return Number.isFinite(parsed) ? parsed : trimmed;
}

// ScenarioEditor is a structured builder over the scenario a tests/ file holds,
// so a system author can define a test without writing YAML by hand.
export const ScenarioEditor: React.FC<ScenarioEditorProps> = ({ scenario, onSave, onCancel }) => {
  const [draft, setDraft] = useState<Scenario>(() => ({
    name: scenario.name,
    seed: scenario.seed ?? 0,
    setup: { player: { stats: scenario.setup?.player?.stats ?? {}, tags: scenario.setup?.player?.tags ?? [] } },
    steps: scenario.steps.length > 0 ? scenario.steps : [{ ...EMPTY_STEP }],
  }));

  const setSteps = (steps: ScenarioStep[]) => setDraft((prev) => ({ ...prev, steps }));

  const updateStep = (index: number, patch: Partial<ScenarioStep>) => {
    setSteps(draft.steps.map((step, i) => (i === index ? { ...step, ...patch } : step)));
  };

  const updateExpect = (index: number, patch: Partial<ScenarioExpectations>) => {
    const step = draft.steps[index];
    updateStep(index, { expect: { ...(step.expect ?? {}), ...patch } });
  };

  const stats = Object.entries(draft.setup?.player?.stats ?? {});
  const setStats = (entries: [string, unknown][]) => {
    const record: Record<string, unknown> = {};
    for (const [key, value] of entries) {
      if (key.trim() !== '') record[key.trim()] = value;
    }
    setDraft((prev) => ({ ...prev, setup: { player: { ...prev.setup?.player, stats: record } } }));
  };

  const canSave = draft.name.trim() !== '' && draft.steps.some((step) => step.action.trim() !== '');

  const handleSave = () => {
    if (!canSave) return;
    onSave({
      ...draft,
      name: draft.name.trim(),
      steps: draft.steps.filter((step) => step.action.trim() !== ''),
    });
  };

  return (
    <div className="space-y-4 rounded-xl border border-stone-800 bg-stone-950/60 p-4">
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <label className="space-y-1 sm:col-span-2">
          <span className="text-xs font-sans uppercase text-stone-300">Scenario name</span>
          <input
            value={draft.name}
            onChange={(e) => setDraft((prev) => ({ ...prev, name: e.target.value }))}
            placeholder="e.g. a strong hit on a standard check"
            className="w-full bg-stone-950 border border-stone-800 rounded-lg px-3 py-2 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-purple-500/60"
          />
        </label>
        <label className="space-y-1">
          <span className="text-xs font-sans uppercase text-stone-300">Seed</span>
          <input
            type="number"
            value={draft.seed}
            onChange={(e) => setDraft((prev) => ({ ...prev, seed: numberOrUndefined(e.target.value) ?? 0 }))}
            className="w-full bg-stone-950 border border-stone-800 rounded-lg px-3 py-2 text-sm font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
          />
        </label>
      </div>

      <div className="space-y-2">
        <span className="text-xs font-sans uppercase text-stone-300">Player stats</span>
        {stats.map(([key, value]) => (
          <div key={key} className="flex items-center gap-2">
            <input
              value={key}
              onChange={(e) => {
                const next = stats.map(([k, v]) => (k === key ? [e.target.value, v] : [k, v])) as [string, unknown][];
                setStats(next);
              }}
              placeholder="stat id"
              className="w-40 bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <input
              value={String(value ?? '')}
              onChange={(e) => {
                const next = stats.map(([k, v]) => (k === key ? [k, scenarioStatValue(e.target.value)] : [k, v])) as [string, unknown][];
                setStats(next);
              }}
              placeholder="value"
              className="w-28 bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <button
              type="button"
              aria-label={`Remove stat ${key}`}
              onClick={() => setStats(stats.filter(([k]) => k !== key))}
              className="p-1.5 rounded text-stone-400 hover:text-red-300 hover:bg-red-950/40 cursor-pointer"
            >
              <Trash2 className="w-3.5 h-3.5" />
            </button>
          </div>
        ))}
        <button
          type="button"
          onClick={() => setStats([...stats, [`stat_${stats.length + 1}`, 0]])}
          className="flex items-center gap-1 text-xs text-purple-300 hover:text-purple-200 cursor-pointer"
        >
          <Plus className="w-3.5 h-3.5" />
          Add stat
        </button>
      </div>

      <div className="space-y-3">
        <span className="text-xs font-sans uppercase text-stone-300">Steps</span>
        {draft.steps.map((step, index) => (
          <div key={index} className="space-y-2 rounded-lg border border-stone-800 bg-stone-900/40 p-3">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-mono text-stone-500">step {index + 1}</span>
              <button
                type="button"
                aria-label={`Remove step ${index + 1}`}
                onClick={() => setSteps(draft.steps.filter((_, i) => i !== index))}
                className="p-1 rounded text-stone-400 hover:text-red-300 hover:bg-red-950/40 cursor-pointer"
              >
                <Trash2 className="w-3.5 h-3.5" />
              </button>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
              <label className="space-y-1">
                <span className="text-[11px] text-stone-400">Action</span>
                <input
                  value={step.action}
                  onChange={(e) => updateStep(index, { action: e.target.value })}
                  placeholder="do"
                  className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
              </label>
              <label className="space-y-1">
                <span className="text-[11px] text-stone-400">Input</span>
                <input
                  value={step.input ?? ''}
                  onChange={(e) => updateStep(index, { input: e.target.value })}
                  placeholder="the check profile name"
                  className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
              </label>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-4 gap-2">
              <label className="space-y-1">
                <span className="text-[11px] text-stone-400">Outcome is</span>
                <input
                  value={step.expect?.outcome ?? ''}
                  onChange={(e) => updateExpect(index, { outcome: e.target.value })}
                  placeholder="strong"
                  className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
              </label>
              <label className="space-y-1">
                <span className="text-[11px] text-stone-400">or one of</span>
                <input
                  value={(step.expect?.outcome_one_of ?? []).join(', ')}
                  onChange={(e) =>
                    updateExpect(index, {
                      outcome_one_of: e.target.value
                        .split(',')
                        .map((value) => value.trim())
                        .filter(Boolean),
                    })
                  }
                  placeholder="strong, weak"
                  className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
              </label>
              <label className="space-y-1">
                <span className="text-[11px] text-stone-400">Total min</span>
                <input
                  type="number"
                  value={step.expect?.total?.min ?? ''}
                  onChange={(e) =>
                    updateExpect(index, {
                      total: { min: numberOrUndefined(e.target.value) ?? 0, max: step.expect?.total?.max ?? 0 },
                    })
                  }
                  className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
              </label>
              <label className="space-y-1">
                <span className="text-[11px] text-stone-400">Total max</span>
                <input
                  type="number"
                  value={step.expect?.total?.max ?? ''}
                  onChange={(e) =>
                    updateExpect(index, {
                      total: { min: step.expect?.total?.min ?? 0, max: numberOrUndefined(e.target.value) ?? 0 },
                    })
                  }
                  className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
                />
              </label>
            </div>
            <label className="space-y-1 block">
              <span className="text-[11px] text-stone-400">Message contains</span>
              <input
                value={step.expect?.message_contains ?? ''}
                onChange={(e) => updateExpect(index, { message_contains: e.target.value })}
                placeholder="a word the resolution message must carry"
                className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60"
              />
            </label>
          </div>
        ))}
        <button
          type="button"
          onClick={() => setSteps([...draft.steps, { ...EMPTY_STEP }])}
          className="flex items-center gap-1 text-xs text-purple-300 hover:text-purple-200 cursor-pointer"
        >
          <Plus className="w-3.5 h-3.5" />
          Add step
        </button>
      </div>

      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="text-xs px-3 py-1.5 rounded-lg border border-stone-700 text-stone-300 hover:text-white hover:bg-stone-800 cursor-pointer"
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={handleSave}
          disabled={!canSave}
          className="text-xs px-3 py-1.5 rounded-lg bg-purple-600 hover:bg-purple-500 text-white font-semibold disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
        >
          Save scenario
        </button>
      </div>
    </div>
  );
};

export default ScenarioEditor;