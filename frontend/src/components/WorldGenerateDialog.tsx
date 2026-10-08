import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, FolderOpen, Globe, Sparkles, Wand2, X } from 'lucide-react';
import { APIClient, HTTPError } from '../api/client';
import { TurnEvent, WorldDraftInfo, WorldEstimate, WorldGenStep } from '../types';

// LargeEstimateCalls is the call count above which Generate asks for a second
// confirmation, so a costly generation is a deliberate choice.
export const LargeEstimateCalls = 10;

export interface WorldGenerateDialogProps {
  onCancel: () => void;
  onDraft: (draft: WorldDraftInfo) => void;
  // estimateDelayMs debounces the dry run. Tests set it to zero.
  estimateDelayMs?: number;
}

type SourceMode = 'prompt' | 'folder' | 'url';

const STEP_LABELS: Record<string, string> = {
  outline: 'Outline',
  places: 'Places and factions',
  characters: 'Characters',
  link: 'Cross-link',
  extract: 'Read the source',
};

const stepLabel = (name: string) => STEP_LABELS[name] ?? name;

// WorldGenerateDialog collects a brief, shows what the generation will cost, and
// streams its progress. It never commits anything: the draft goes to review.
export const WorldGenerateDialog: React.FC<WorldGenerateDialogProps> = ({
  onCancel,
  onDraft,
  estimateDelayMs = 250,
}) => {
  const [premise, setPremise] = useState('');
  const [name, setName] = useState('');
  const [genre, setGenre] = useState('');
  const [locations, setLocations] = useState(4);
  const [factions, setFactions] = useState(3);
  const [characters, setCharacters] = useState(5);

  const [sourceMode, setSourceMode] = useState<SourceMode>('prompt');
  const [folderPath, setFolderPath] = useState('');
  const [urls, setUrls] = useState('');
  const [browsing, setBrowsing] = useState(false);
  const [browseUnavailable, setBrowseUnavailable] = useState(false);

  const [estimate, setEstimate] = useState<WorldEstimate | null>(null);
  const [needsConfirm, setNeedsConfirm] = useState(false);
  const [steps, setSteps] = useState<WorldGenStep[]>([]);
  const [generating, setGenerating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const urlList = useMemo(
    () =>
      urls
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean),
    [urls]
  );

  // A source decides how many entities exist, so counts are sent for a premise
  // generation only.
  const request = useMemo(
    () => ({
      premise,
      name: name || undefined,
      genre: genre || undefined,
      counts: sourceMode === 'prompt' ? { locations, factions, characters } : undefined,
      source:
        sourceMode === 'folder'
          ? { kind: 'folder' as const, path: folderPath }
          : sourceMode === 'url'
            ? { kind: 'url' as const, urls: urlList }
            : undefined,
    }),
    [premise, name, genre, locations, factions, characters, sourceMode, folderPath, urlList]
  );

  const sourceReady =
    sourceMode === 'prompt' || (sourceMode === 'folder' ? folderPath.trim() !== '' : urlList.length > 0);
  const canGenerate = sourceReady && !generating;

  // A dry run reports the call count and cost without calling a model, so the
  // spend is visible before the user commits to it.
  useEffect(() => {
    if (!sourceReady) {
      setEstimate(null);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      const run = async () => {
        try {
          await APIClient.generateWorld({ ...request, dry_run: true }, (event: TurnEvent) => {
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
  }, [request, sourceReady, estimateDelayMs]);

  useEffect(() => {
    return () => abortRef.current?.abort();
  }, []);

  const handleGenerate = useCallback(async () => {
    if (estimate && estimate.calls >= LargeEstimateCalls && !needsConfirm) {
      setNeedsConfirm(true);
      return;
    }
    setNeedsConfirm(false);
    setError(null);
    setSteps([]);
    setGenerating(true);

    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const emit = (event: TurnEvent) => {
        if (event.type === 'step' && event.step) {
          setSteps((prev) => [...prev.filter((s) => s.name !== event.step!.name), event.step!]);
        }
        if (event.type === 'draft' && event.draft) {
          onDraft(event.draft);
        }
        if (event.type === 'error') {
          setError(event.message || event.detail || 'the generation failed');
        }
      };
      if (sourceMode === 'prompt') {
        await APIClient.generateWorld(request, emit, controller.signal);
      } else {
        await APIClient.ingestWorld(request, emit, controller.signal);
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setGenerating(false);
      abortRef.current = null;
    }
  }, [estimate, needsConfirm, onDraft, request, sourceMode]);

  const handleCancel = useCallback(() => {
    abortRef.current?.abort();
    onCancel();
  }, [onCancel]);

  // browse opens the desktop window's native folder picker. A headless or
  // browser build has none, so the path field stays usable on its own.
  const handleBrowse = useCallback(async () => {
    setBrowsing(true);
    setError(null);
    try {
      const chosen = await APIClient.chooseDirectory('Choose a source folder');
      if (chosen) setFolderPath(chosen);
    } catch (err) {
      if (err instanceof HTTPError && err.status === 501) {
        setBrowseUnavailable(true);
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setBrowsing(false);
    }
  }, []);

  // formatCost renders an estimate's cost, or says plainly that the provider has
  // no published rate rather than implying the generation is free.
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
      <div className="relative w-full max-w-2xl overflow-hidden border border-white/10 rounded-2xl bg-neutral-900/90 shadow-2xl backdrop-blur-xl">
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-400">
              <Wand2 className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-white">Generate a world</h3>
              <p className="text-xs text-neutral-400">Draft a world from a brief, or import a source</p>
            </div>
          </div>
          <button
            onClick={handleCancel}
            disabled={generating}
            aria-label="Cancel generation"
            className="p-1.5 text-neutral-400 hover:text-white rounded-lg hover:bg-white/5 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="p-6 space-y-5 max-h-[70vh] overflow-y-auto">
          {error && (
            <div className="p-3 text-sm text-red-300 border border-red-500/30 rounded-xl bg-red-950/40">
              {error}
            </div>
          )}

          <div className="space-y-2">
            <label htmlFor="world-premise" className="text-xs font-medium text-neutral-300">
              Premise
            </label>
            <textarea
              id="world-premise"
              value={premise}
              onChange={(event) => setPremise(event.target.value)}
              rows={2}
              placeholder="A drowned kingdom where the tides never recede."
              className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-purple-500/50 focus:outline-none resize-none"
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-2">
              <label htmlFor="world-name" className="text-xs font-medium text-neutral-300">
                Name (optional)
              </label>
              <input
                id="world-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-purple-500/50 focus:outline-none"
              />
            </div>
            <div className="space-y-2">
              <label htmlFor="world-genre" className="text-xs font-medium text-neutral-300">
                Genre (optional)
              </label>
              <input
                id="world-genre"
                value={genre}
                onChange={(event) => setGenre(event.target.value)}
                className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-purple-500/50 focus:outline-none"
              />
            </div>
          </div>

          {sourceMode === 'prompt' ? (
            <div className="space-y-2">
              <div className="grid grid-cols-3 gap-3">
                <label className="space-y-2 text-xs font-medium text-neutral-300">
                  Locations
                  <input
                    type="number"
                    min={0}
                    max={10}
                    aria-label="Locations"
                    value={locations}
                    onChange={(event) => setLocations(Number(event.target.value))}
                    className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-purple-500/50 focus:outline-none"
                  />
                </label>
                <label className="space-y-2 text-xs font-medium text-neutral-300">
                  Factions
                  <input
                    type="number"
                    min={0}
                    max={10}
                    aria-label="Factions"
                    value={factions}
                    onChange={(event) => setFactions(Number(event.target.value))}
                    className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-purple-500/50 focus:outline-none"
                  />
                </label>
                <label className="space-y-2 text-xs font-medium text-neutral-300">
                  Characters
                  <input
                    type="number"
                    min={0}
                    max={10}
                    aria-label="Characters"
                    value={characters}
                    onChange={(event) => setCharacters(Number(event.target.value))}
                    className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-purple-500/50 focus:outline-none"
                  />
                </label>
              </div>
              <p className="text-[11px] text-neutral-500">
                How many of each the generation asks for, up to 10. These counts apply to the
                premise only.
              </p>
            </div>
          ) : (
            <p className="text-[11px] text-neutral-500">
              The source decides how many entities are extracted, so there are no counts to set.
            </p>
          )}

          <div className="space-y-2">
            <span className="text-xs font-medium text-neutral-300">Source</span>
            <div className="grid grid-cols-3 gap-2">
              {(
                [
                  { id: 'prompt', label: 'Premise', icon: Sparkles },
                  { id: 'folder', label: 'Folder', icon: FolderOpen },
                  { id: 'url', label: 'URLs', icon: Globe },
                ] as const
              ).map(({ id, label, icon: Icon }) => (
                <button
                  key={id}
                  type="button"
                  onClick={() => setSourceMode(id)}
                  aria-pressed={sourceMode === id}
                  className={`flex items-center justify-center gap-2 p-3 rounded-xl border text-xs transition-all ${
                    sourceMode === id
                      ? 'border-purple-500/50 bg-purple-500/10 text-white'
                      : 'border-white/5 bg-white/[0.02] text-neutral-400 hover:border-white/10'
                  }`}
                >
                  <Icon className="w-4 h-4" />
                  {label}
                </button>
              ))}
            </div>
          </div>

          {sourceMode === 'folder' && (
            <div className="space-y-2">
              <label htmlFor="world-folder" className="text-xs font-medium text-neutral-300">
                Folder path
              </label>
              <div className="flex items-center gap-2">
                <input
                  id="world-folder"
                  value={folderPath}
                  onChange={(event) => setFolderPath(event.target.value)}
                  placeholder="/home/you/setting-notes"
                  className="flex-1 min-w-0 px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-purple-500/50 focus:outline-none font-mono"
                />
                <button
                  type="button"
                  onClick={() => void handleBrowse()}
                  disabled={browsing}
                  className="flex items-center gap-1.5 px-3 py-2 text-xs text-neutral-200 rounded-xl border border-white/10 hover:bg-white/5 disabled:opacity-40 transition-colors whitespace-nowrap"
                >
                  <FolderOpen className="w-3.5 h-3.5" />
                  Browse
                </button>
              </div>
              <p className="text-[11px] text-neutral-500">
                {browseUnavailable
                  ? 'No native folder dialog is available here, so type the path instead.'
                  : 'Read locally. Nothing is fetched.'}
              </p>
            </div>
          )}

          {sourceMode === 'url' && (
            <div className="space-y-2">
              <label htmlFor="world-urls" className="text-xs font-medium text-neutral-300">
                URLs, one per line
              </label>
              <textarea
                id="world-urls"
                value={urls}
                onChange={(event) => setUrls(event.target.value)}
                rows={3}
                placeholder="https://example.org/wiki/saltmarch"
                className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-purple-500/50 focus:outline-none resize-none font-mono"
              />
              <p className="text-[11px] text-amber-400/80 flex items-center gap-1.5">
                <AlertTriangle className="w-3.5 h-3.5" />
                This fetches the pages you name. Only the URLs listed are fetched, and you are
                responsible for their licensing.
              </p>
            </div>
          )}

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
            <ul aria-label="Generation progress" className="space-y-1.5">
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
            className="px-4 py-2 text-sm text-neutral-300 rounded-xl hover:bg-white/5 transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={() => void handleGenerate()}
            disabled={!canGenerate}
            className="px-4 py-2 text-sm font-medium text-white rounded-xl bg-purple-600 hover:bg-purple-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            {generating ? 'Generating...' : needsConfirm ? 'Generate anyway' : 'Generate'}
          </button>
        </div>
      </div>
    </div>
  );
};

export default WorldGenerateDialog;
