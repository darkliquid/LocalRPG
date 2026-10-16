import { Pencil, Plus, Trash2 } from 'lucide-react';
import { Scenario } from '../../../types';
import { ScenarioEditor } from '../../ScenarioEditor';

export interface SystemTestsTabProps {
  /** savedID is null until the system has been saved, and scenarios need one. */
  savedID: string | null;
  scenarios: Scenario[];
  scenarioError: string | null;
  isLoadingScenarios: boolean;
  editingScenario: Scenario | null;
  setEditingScenario: (scenario: Scenario | null) => void;
  handleSaveScenario: (scenario: Scenario) => void | Promise<void>;
  handleDeleteScenario: (name: string) => void | Promise<void>;
}

// SystemTestsTab lists a system's stored scenarios and edits one at a time. Each
// runs the system's own mechanics without a provider, so an author can check a
// rules change before it reaches a campaign.
export function SystemTestsTab({
  savedID,
  scenarios,
  scenarioError,
  isLoadingScenarios,
  editingScenario,
  setEditingScenario,
  handleSaveScenario,
  handleDeleteScenario,
}: SystemTestsTabProps) {
  return (
    <div className="flex-1 flex flex-col gap-3 min-h-0 overflow-y-auto pr-1">
      <div className="flex items-center justify-between shrink-0">
        <span className="text-xs font-mono text-stone-400 px-1">
          {scenarios.length} scenario{scenarios.length === 1 ? '' : 's'}
        </span>
        <button
          type="button"
          disabled={!savedID}
          onClick={() => setEditingScenario({ name: '', seed: 0, steps: [] })}
          title={savedID ? 'Add a scenario' : 'Save the system before adding scenarios'}
          className="flex items-center gap-1 text-xs px-2 py-1 rounded bg-purple-600/20 border border-purple-500/40 text-purple-300 hover:bg-purple-600/30 disabled:opacity-50 cursor-pointer"
        >
          <Plus className="w-3 h-3" />
          <span>New scenario</span>
        </button>
      </div>

      {scenarioError && <p className="text-xs text-red-300 px-1">{scenarioError}</p>}

      {!savedID && <p className="text-xs text-stone-500 px-1">Save the system before adding scenarios.</p>}

      {editingScenario && (
        <ScenarioEditor
          scenario={editingScenario}
          onSave={(next) => void handleSaveScenario(next)}
          onCancel={() => setEditingScenario(null)}
        />
      )}

      {!editingScenario && savedID && !isLoadingScenarios && scenarios.length === 0 && (
        <p className="text-xs text-stone-500 px-1">
          No scenarios yet. Add one to test this system's mechanics without a provider.
        </p>
      )}

      {!editingScenario && scenarios.length > 0 && (
        <ul className="space-y-1">
          {scenarios.map((scenario) => (
            <li
              key={scenario.name}
              className="flex items-center justify-between gap-2 rounded-lg border border-stone-800 bg-stone-900/40 px-3 py-2"
            >
              <div className="min-w-0">
                <div className="text-sm text-stone-200 truncate">{scenario.name}</div>
                <div className="text-[11px] font-mono text-stone-500">
                  {scenario.steps.length} step{scenario.steps.length === 1 ? '' : 's'}
                  {scenario.seed ? `, seed ${scenario.seed}` : ''}
                </div>
              </div>
              <div className="flex items-center gap-1 shrink-0">
                <button
                  type="button"
                  onClick={() => setEditingScenario(scenario)}
                  aria-label={`Edit ${scenario.name}`}
                  title={`Edit ${scenario.name}`}
                  className="p-1.5 rounded text-stone-400 hover:text-purple-300 hover:bg-purple-950/40 cursor-pointer"
                >
                  <Pencil className="w-3.5 h-3.5" />
                </button>
                <button
                  type="button"
                  onClick={() => void handleDeleteScenario(scenario.name)}
                  aria-label={`Delete ${scenario.name}`}
                  title={`Delete ${scenario.name}`}
                  className="p-1.5 rounded text-stone-400 hover:text-red-300 hover:bg-red-950/40 cursor-pointer"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
