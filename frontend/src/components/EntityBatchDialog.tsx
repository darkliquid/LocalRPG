import React, { useCallback, useMemo, useRef, useState } from 'react';
import { AlertTriangle, Check, FolderOpen, Globe, PackagePlus, Pencil, RefreshCw, X } from 'lucide-react';
import { APIClient, HTTPError } from '../api/client';
import { useFolderPicker } from '../hooks/useFolderPicker';
import { limitFieldFor } from '../lib/generationLimit';
import { GenerationLimitNotice } from './GenerationLimitNotice';
import { ImportProgressPanel } from './ImportProgressPanel';
import {
  GenerationLimitsOverride,
  TurnEvent,
  WorldApplyResult,
  WorldDraftEntity,
  WorldEntityBatch,
  WorldEntityBatchRequest,
  WorldImportProgress,
  WorldSource,
} from '../types';

export interface EntityBatchDialogProps {
  worldId: string;
  onClose: () => void;
  onAccepted?: (result: WorldApplyResult) => void;
  // batch seeds the preview, so a caller that already generated one does not
  // generate it twice.
  batch?: WorldEntityBatch | null;
}

const KINDS = ['character', 'location', 'faction', 'item', 'concept'];

type SourceMode = 'instruction' | 'folder' | 'url';

// EntityBatchDialog generates or extracts a batch of entities for an existing
// world, shows the preview with its resolved links, and writes nothing until
// Accept.
export const EntityBatchDialog: React.FC<EntityBatchDialogProps> = ({
  worldId,
  onClose,
  onAccepted,
  batch: seeded = null,
}) => {
  const [sourceMode, setSourceMode] = useState<SourceMode>('instruction');
  const [instruction, setInstruction] = useState('');
  const [kind, setKind] = useState('faction');
  const [count, setCount] = useState(3);
  const [focus, setFocus] = useState('');
  const [folderPath, setFolderPath] = useState('');
  const [urls, setUrls] = useState('');

  const [batch, setBatch] = useState<WorldEntityBatch | null>(seeded);
  const [rename, setRename] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [accepted, setAccepted] = useState<string[] | null>(null);
  // A raise the user asked for stays in effect for this dialog, so a retry does
  // not have to be confirmed twice.
  const [limits, setLimits] = useState<GenerationLimitsOverride | undefined>(undefined);
  const [limitCode, setLimitCode] = useState<string | null>(null);
  // Progress through a source import, so a long read says what it is doing.
  const [progress, setProgress] = useState<WorldImportProgress | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const urlList = useMemo(
    () =>
      urls
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean),
    [urls]
  );

  const source: WorldSource | undefined =
    sourceMode === 'folder'
      ? { kind: 'folder', path: folderPath }
      : sourceMode === 'url'
        ? { kind: 'url', urls: urlList }
        : undefined;

  const sourceReady =
    sourceMode === 'instruction' ||
    (sourceMode === 'folder' ? folderPath.trim() !== '' : urlList.length > 0);

  // The native picker is a modal dialog, so the server opens it and this hook
  // polls for the result. A build with no dialog leaves the path field usable.
  const folderPicker = useFolderPicker();
  const handleBrowse = useCallback(async () => {
    const chosen = await folderPicker.pick('Choose a source folder');
    if (chosen) setFolderPath(chosen);
  }, [folderPicker]);

  const runExtraction = async (override?: GenerationLimitsOverride) => {
    setLoading(true);
    setError(null);
    setLimitCode(null);
    setAccepted(null);
    setProgress(null);

    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const req: WorldEntityBatchRequest = {
        instruction,
        source,
        kinds: source ? undefined : [kind],
        count: source ? undefined : count,
        focus: focus || undefined,
        limits: override ?? limits,
      };
      const emit = (event: TurnEvent) => {
        if (event.type === 'progress' && event.progress) setProgress(event.progress);
        if (event.type === 'error') {
          setError(event.message || event.detail || 'the extraction failed');
          setLimitCode(limitFieldFor(event.code) ? (event.code ?? null) : null);
        }
      };
      const finished = await APIClient.previewWorldEntitiesStream(worldId, req, emit, controller.signal);
      if (finished) setBatch(finished);
    } catch (err) {
      if (!controller.signal.aborted) {
        const message = err instanceof Error ? err.message : String(err);
        setError(message);
        setLimitCode(err instanceof HTTPError ? (limitFieldFor(err.code) ? (err.code ?? null) : null) : null);
      }
    } finally {
      setLoading(false);
      abortRef.current = null;
    }
  };

  const handleGenerate = () => void runExtraction();

  // Raising a limit and running again is one action, so a retry is a single
  // click rather than a trip through Settings and back.
  const handleRaiseAndRun = (raised: GenerationLimitsOverride) => {
    setLimits(raised);
    void runExtraction(raised);
  };

  const handleRaiseAndSave = (raised: GenerationLimitsOverride) => {
    APIClient.raiseGenerationLimit(raised)
      .then(() => handleRaiseAndRun(raised))
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)));
  };

  const handleAccept = async () => {
    if (!batch) return;
    setLoading(true);
    setError(null);
    try {
      const result = await APIClient.acceptWorldEntities(worldId, { entities: batch.entities, rename });
      setAccepted(result.written);
      setBatch(null);
      onAccepted?.(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  const entities: WorldDraftEntity[] = batch?.entities ?? [];
  const actionLabel = sourceMode === 'instruction' ? 'Generate' : 'Extract';
  // Naming the limit on the button keeps the raised value visible at the moment
  // it is used, rather than applying a number the user has to remember typing.
  const pendingLimit = limits ? limits.max_calls ?? limits.max_chunks : undefined;
  const limitSuffix = pendingLimit ? ` (limit ${pendingLimit})` : '';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div className="relative w-full max-w-2xl overflow-hidden border border-white/10 rounded-2xl bg-neutral-900/90 shadow-2xl backdrop-blur-xl">
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-xl bg-sky-500/10 border border-sky-500/20 text-sky-400">
              <PackagePlus className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-white">Add entities to this world</h3>
              <p className="text-xs text-neutral-400">
                Preview a batch before it is written. Nothing existing is changed.
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            aria-label="Close"
            className="p-1.5 text-neutral-400 hover:text-white rounded-lg hover:bg-white/5 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="p-6 space-y-4 max-h-[65vh] overflow-y-auto">
          {loading && progress && <ImportProgressPanel progress={progress} />}
          {batch?.oracle && (
            <p className="p-3 text-xs rounded-xl bg-amber-500/10 border border-amber-500/25 text-amber-200">
              No model provider is configured, so these came from the built-in template generator
              rather than from your source. Assign one in Settings → AI Agents to read a source.
            </p>
          )}
          {limitCode && error ? (
            <GenerationLimitNotice
              code={limitCode}
              message={error}
              busy={loading}
              onRetry={handleRaiseAndRun}
              onSave={handleRaiseAndSave}
              onChange={setLimits}
            />
          ) : (
            error && (
              <p className="p-3 text-sm rounded-xl bg-red-950/40 border border-red-500/30 text-red-300">
                {error}
              </p>
            )
          )}
          {accepted && (
            <p className="p-3 text-sm rounded-xl bg-emerald-500/10 border border-emerald-500/25 text-emerald-300">
              Wrote {accepted.length} {accepted.length === 1 ? 'entity' : 'entities'}: {accepted.join(', ')}
            </p>
          )}

          <div className="space-y-2">
            <span className="text-xs font-medium text-neutral-300">From</span>
            <div className="grid grid-cols-3 gap-2">
              {(
                [
                  { id: 'instruction', label: 'Instruction', icon: Pencil },
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
                      ? 'border-sky-500/50 bg-sky-500/10 text-white'
                      : 'border-white/5 bg-white/[0.02] text-neutral-400 hover:border-white/10'
                  }`}
                >
                  <Icon className="w-4 h-4" />
                  {label}
                </button>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <label htmlFor="batch-instruction" className="text-xs font-medium text-neutral-300">
              {sourceMode === 'instruction' ? 'Instruction' : 'Instruction (optional)'}
            </label>
            <textarea
              id="batch-instruction"
              value={instruction}
              onChange={(event) => setInstruction(event.target.value)}
              rows={2}
              placeholder={
                sourceMode === 'instruction'
                  ? 'three rival factions in the south'
                  : 'keep the names exactly as the source writes them'
              }
              className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-sky-500/50 focus:outline-none resize-none"
            />
          </div>

          {sourceMode === 'instruction' ? (
            <div className="grid grid-cols-3 gap-3">
              <label className="space-y-2 text-xs font-medium text-neutral-300">
                Kind
                <select
                  aria-label="Kind"
                  value={kind}
                  onChange={(event) => setKind(event.target.value)}
                  className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-sky-500/50 focus:outline-none"
                >
                  {KINDS.map((option) => (
                    <option key={option} value={option} className="bg-neutral-900">
                      {option}
                    </option>
                  ))}
                </select>
              </label>
              <label className="space-y-2 text-xs font-medium text-neutral-300">
                Count
                <input
                  type="number"
                  min={1}
                  max={10}
                  aria-label="Count"
                  value={count}
                  onChange={(event) => setCount(Number(event.target.value))}
                  className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-sky-500/50 focus:outline-none"
                />
              </label>
              <label className="space-y-2 text-xs font-medium text-neutral-300">
                Focus (optional)
                <input
                  aria-label="Focus"
                  value={focus}
                  onChange={(event) => setFocus(event.target.value)}
                  placeholder="Saltmarch"
                  className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-sky-500/50 focus:outline-none"
                />
              </label>
            </div>
          ) : (
            <p className="text-[11px] text-neutral-500">
              The source decides how many entities are added and what kind they are.
            </p>
          )}

          {sourceMode === 'folder' && (
            <div className="space-y-2">
              <label htmlFor="batch-folder" className="text-xs font-medium text-neutral-300">
                Folder path
              </label>
              <div className="flex items-center gap-2">
                <input
                  id="batch-folder"
                  value={folderPath}
                  onChange={(event) => setFolderPath(event.target.value)}
                  placeholder="/home/you/setting-notes"
                  className="flex-1 min-w-0 px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-sky-500/50 focus:outline-none font-mono"
                />
                <button
                  type="button"
                  onClick={() => void handleBrowse()}
                  disabled={folderPicker.picking}
                  className="flex items-center gap-1.5 px-3 py-2 text-xs text-neutral-200 rounded-xl border border-white/10 hover:bg-white/5 disabled:opacity-40 transition-colors whitespace-nowrap"
                >
                  <FolderOpen className="w-3.5 h-3.5" />
                  Browse
                </button>
              </div>
              <p className="text-[11px] text-neutral-500">
                {folderPicker.note ?? 'Read locally. Nothing is fetched.'}
              </p>
            </div>
          )}

          {sourceMode === 'url' && (
            <div className="space-y-2">
              <label htmlFor="batch-urls" className="text-xs font-medium text-neutral-300">
                URLs, one per line
              </label>
              <textarea
                id="batch-urls"
                value={urls}
                onChange={(event) => setUrls(event.target.value)}
                rows={3}
                placeholder="https://example.org/wiki/saltmarch"
                className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-sky-500/50 focus:outline-none resize-none font-mono"
              />
              <p className="text-[11px] text-amber-400/80 flex items-center gap-1.5">
                <AlertTriangle className="w-3.5 h-3.5" />
                This fetches the pages you name. Only the URLs listed are fetched, and you are
                responsible for their licensing.
              </p>
            </div>
          )}

          {entities.length > 0 && (
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-neutral-300">
                  Preview ({entities.length})
                </span>
                <label className="flex items-center gap-2 text-[11px] text-neutral-400">
                  <input
                    type="checkbox"
                    checked={rename}
                    onChange={(event) => setRename(event.target.checked)}
                  />
                  Rename on an id clash
                </label>
              </div>
              {entities.map((entity) => (
                <div key={entity.id} className="p-3 rounded-xl border border-white/10 bg-white/[0.03]">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-semibold text-white">{entity.name}</span>
                    <span className="text-[11px] uppercase tracking-wide text-neutral-500">
                      {entity.type}
                    </span>
                    <span className="ml-auto font-mono text-[11px] text-neutral-500">{entity.id}</span>
                  </div>
                  <p className="mt-1 text-xs text-neutral-300 line-clamp-3">{entity.body}</p>
                  {entity.source && (
                    <p className="mt-1 text-[11px] text-neutral-500">From: {entity.source}</p>
                  )}
                  {entity.links && entity.links.length > 0 && (
                    <p className="mt-1 text-[11px] text-neutral-500">Links: {entity.links.join(', ')}</p>
                  )}
                  {entity.dropped_links && entity.dropped_links.length > 0 && (
                    <p className="mt-1 text-[11px] text-amber-400/80 flex items-center gap-1.5">
                      <AlertTriangle className="w-3.5 h-3.5" />
                      Dropped unresolved links: {entity.dropped_links.join(', ')}
                    </p>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-3 p-6 border-t border-white/10">
          <button
            onClick={handleGenerate}
            disabled={loading || !sourceReady}
            className="flex items-center gap-2 px-4 py-2 text-sm text-neutral-200 rounded-xl border border-white/10 hover:bg-white/5 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            {entities.length > 0 ? `${actionLabel} again${limitSuffix}` : `${actionLabel}${limitSuffix}`}
          </button>
          <button
            onClick={() => void handleAccept()}
            disabled={loading || entities.length === 0}
            className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-white rounded-xl bg-sky-600 hover:bg-sky-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            <Check className="w-4 h-4" />
            Accept
          </button>
        </div>
      </div>
    </div>
  );
};

export default EntityBatchDialog;
