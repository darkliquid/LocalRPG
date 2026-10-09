import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Wand2, X } from 'lucide-react';
import { APIClient } from '../api/client';
import { limitFieldFor } from '../lib/generationLimit';
import { GenerationLimitNotice } from './GenerationLimitNotice';
import {
  GenerationLimitsOverride,
  SystemDraftInfo,
  SystemGenerateRequest,
  TurnEvent,
  WorldEstimate,
  WorldGenStep,
} from '../types';

export const LargeEstimateCalls = 10;

export interface SystemGenerateDialogProps {
  onCancel: () => void;
  onDraft: (draft: SystemDraftInfo) => void;
  // estimateDelayMs debounces the dry run. Tests set it to zero.
  estimateDelayMs?: number;
}

const STEP_LABELS: Record<string, string> = {
  shape: 'Mechanical shape',
  schema: 'Stats, skills and checks',
  hooks: 'Script hooks',
  rules: 'Rules guide',
  verify: 'Smoke test verification',
};

const stepLabel = (name: string) => STEP_LABELS[name] ?? name;

export const SystemGenerateDialog: React.FC<SystemGenerateDialogProps> = ({
  onCancel,
  onDraft,
  estimateDelayMs = 250,
}) => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');

  const [estimate, setEstimate] = useState<WorldEstimate | null>(null);
  const [needsConfirm, setNeedsConfirm] = useState(false);
  const [steps, setSteps] = useState<WorldGenStep[]>([]);
  const [generating, setGenerating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [limits, setLimits] = useState<GenerationLimitsOverride | undefined>(undefined);
  const [limitCode, setLimitCode] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const request = useMemo<SystemGenerateRequest>(
    () => ({
      description,
      name: name || undefined,
      limits,
    }),
    [description, name, limits]
  );

  const canGenerate = description.trim() !== '' && !generating;

  // A dry run reports the call count and cost without calling a model.
  useEffect(() => {
    if (!description.trim()) {
      setEstimate(null);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      const run = async () => {
        try {
          await APIClient.generateSystem({ ...request, dry_run: true }, (event: TurnEvent) => {
            if (!cancelled && event.type === 'estimate' && event.estimate) {
              setEstimate(event.estimate);
            }
          });
        } catch {
          if (!cancelled) setEstimate(null);
        }
      };
      void run();
    }, estimateDelayMs);

    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [request, description, estimateDelayMs]);

  useEffect(() => {
    return () => abortRef.current?.abort();
  }, []);

  const runGeneration = useCallback(
    async (override?: GenerationLimitsOverride) => {
      setNeedsConfirm(false);
      setError(null);
      setLimitCode(null);
      setSteps([]);
      setGenerating(true);
      const body = override ? { ...request, limits: override } : request;

      const controller = new AbortController();
      abortRef.current = controller;
      try {
        const emit = (event: TurnEvent) => {
          if (event.type === 'step' && event.step) {
            setSteps((prev) => [...prev.filter((s) => s.name !== event.step!.name), event.step!]);
          }
          if (event.type === 'draft' && event.system_draft) {
            onDraft(event.system_draft);
          }
          if (event.type === 'error') {
            setError(event.message || event.detail || 'the generation failed');
            setLimitCode(limitFieldFor(event.code) ? (event.code ?? null) : null);
          }
        };
        await APIClient.generateSystem(body, emit, controller.signal);
      } catch (err) {
        if (!controller.signal.aborted) {
          setError(err instanceof Error ? err.message : String(err));
        }
      } finally {
        setGenerating(false);
        abortRef.current = null;
      }
    },
    [onDraft, request]
  );

  const handleGenerate = useCallback(async () => {
    if (estimate && estimate.calls >= LargeEstimateCalls && !needsConfirm) {
      setNeedsConfirm(true);
      return;
    }
    await runGeneration();
  }, [estimate, needsConfirm, runGeneration]);

  const handleRaiseAndRun = useCallback(
    (raised: GenerationLimitsOverride) => {
      setLimits(raised);
      void runGeneration(raised);
    },
    [runGeneration]
  );

  const handleRaiseAndSave = useCallback(
    (raised: GenerationLimitsOverride) => {
      APIClient.raiseGenerationLimit(raised)
        .then(() => handleRaiseAndRun(raised))
        .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)));
    },
    [handleRaiseAndRun]
  );

  const handleCancel = useCallback(() => {
    abortRef.current?.abort();
    onCancel();
  }, [onCancel]);

  const formatCost = (micros: number) => {
    const units = micros / 1_000_000;
    return units < 0.01 ? '<$0.01' : `$${units.toFixed(2)}`;
  };
  const costLabel = estimate
    ? estimate.priced
      ? formatCost(estimate.cost_micros ?? 0)
      : 'unpriced'
    : '';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div className="relative w-full max-w-xl overflow-hidden border border-white/10 rounded-2xl bg-neutral-900/90 shadow-2xl backdrop-blur-xl">
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-400">
              <Wand2 className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-white">Generate a system</h3>
              <p className="text-xs text-neutral-400">Draft rules, mechanics, and checks from natural language</p>
            </div>
          </div>
          <button
            onClick={handleCancel}
            disabled={generating}
            aria-label="Cancel generation"
            className="p-1.5 text-neutral-400 hover:text-white rounded-lg hover:bg-white/5 transition-colors cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="p-6 space-y-4 max-h-[70vh] overflow-y-auto">
          {limitCode && error ? (
            <GenerationLimitNotice
              code={limitCode}
              message={error}
              busy={generating}
              onRetry={handleRaiseAndRun}
              onSave={handleRaiseAndSave}
              onChange={setLimits}
            />
          ) : (
            error && (
              <div className="p-3 text-xs rounded-xl bg-red-500/10 border border-red-500/20 text-red-300">
                {error}
              </div>
            )
          )}

          <div className="space-y-1.5">
            <label htmlFor="system-name" className="text-xs font-medium text-neutral-300">
              Preferred Name <span className="text-neutral-500">(optional)</span>
            </label>
            <input
              id="system-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Iron & Steam"
              className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-purple-500/50 focus:outline-none"
            />
          </div>

          <div className="space-y-1.5">
            <label htmlFor="system-description" className="text-xs font-medium text-neutral-300">
              System Brief <span className="text-purple-400">*</span>
            </label>
            <textarea
              id="system-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={4}
              placeholder="Describe the setting, resolution style (e.g. d20, 2d6 ladder, dice pool), core stats, skills, and mechanics..."
              className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-purple-500/50 focus:outline-none resize-none"
            />
          </div>

          {estimate && (
            <div
              data-testid="generation-estimate"
              className="flex items-center justify-between p-3 rounded-xl bg-white/[0.03] border border-white/5 text-xs"
            >
              <span className="text-neutral-400">Estimated</span>
              <span className="text-neutral-200">
                {estimate.calls} {estimate.calls === 1 ? 'call' : 'calls'} · {costLabel}
              </span>
            </div>
          )}

          {needsConfirm && (
            <div
              data-testid="generation-confirm"
              className="p-3 text-xs rounded-xl bg-amber-500/10 border border-amber-500/25 text-amber-200"
            >
              This generation will make {estimate?.calls} calls. Press Generate again to confirm.
            </div>
          )}

          {steps.length > 0 && (
            <ul aria-label="Generation progress" className="space-y-1.5 pt-2 border-t border-white/5">
              {steps.map((step) => (
                <li key={step.name} className="flex items-center gap-2 text-xs">
                  <span
                    className={`w-1.5 h-1.5 rounded-full ${
                      step.status === 'error' ? 'bg-red-400' : 'bg-emerald-400'
                    }`}
                  />
                  <span className="text-neutral-300">{stepLabel(step.name)}</span>
                  {step.detail && <span className="text-neutral-500">{step.detail}</span>}
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="flex items-center justify-end gap-3 p-6 border-t border-white/10">
          <button
            onClick={handleCancel}
            className="px-4 py-2 text-sm text-neutral-300 rounded-xl hover:bg-white/5 transition-colors cursor-pointer"
          >
            Cancel
          </button>
          <button
            onClick={() => void handleGenerate()}
            disabled={!canGenerate}
            className="px-4 py-2 text-sm font-medium text-white rounded-xl bg-purple-600 hover:bg-purple-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
          >
            {generating
              ? 'Generating...'
              : `${needsConfirm ? 'Generate anyway' : 'Generate'}${
                  limits ? ` (limit ${limits.max_calls})` : ''
                }`}
          </button>
        </div>
      </div>
    </div>
  );
};

export default SystemGenerateDialog;
