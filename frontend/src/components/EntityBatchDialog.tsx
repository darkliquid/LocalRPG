import React, { useState } from 'react';
import { AlertTriangle, Check, PackagePlus, RefreshCw, X } from 'lucide-react';
import { APIClient } from '../api/client';
import { WorldApplyResult, WorldDraftEntity, WorldEntityBatch, WorldEntityBatchRequest } from '../types';

export interface EntityBatchDialogProps {
  worldId: string;
  onClose: () => void;
  onAccepted?: (result: WorldApplyResult) => void;
  // batch seeds the preview, so a caller that already generated one does not
  // generate it twice.
  batch?: WorldEntityBatch | null;
}

const KINDS = ['character', 'location', 'faction', 'item', 'concept'];

// EntityBatchDialog generates a batch of entities for an existing world, shows
// the preview with its resolved links, and writes nothing until Accept.
export const EntityBatchDialog: React.FC<EntityBatchDialogProps> = ({
  worldId,
  onClose,
  onAccepted,
  batch: seeded = null,
}) => {
  const [instruction, setInstruction] = useState('');
  const [kind, setKind] = useState('faction');
  const [count, setCount] = useState(3);
  const [focus, setFocus] = useState('');
  const [batch, setBatch] = useState<WorldEntityBatch | null>(seeded);
  const [rename, setRename] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [accepted, setAccepted] = useState<string[] | null>(null);

  const handleGenerate = async () => {
    setLoading(true);
    setError(null);
    setAccepted(null);
    try {
      const req: WorldEntityBatchRequest = {
        instruction,
        kinds: [kind],
        count,
        focus: focus || undefined,
      };
      setBatch(await APIClient.previewWorldEntities(worldId, req));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
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

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div className="relative w-full max-w-2xl overflow-hidden border border-white/10 rounded-2xl bg-neutral-900/90 shadow-2xl backdrop-blur-xl">
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-xl bg-sky-500/10 border border-sky-500/20 text-sky-400">
              <PackagePlus className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-white">Generate entities</h3>
              <p className="text-xs text-neutral-400">Preview a batch before it is written</p>
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
          {error && (
            <p className="p-3 text-sm rounded-xl bg-red-950/40 border border-red-500/30 text-red-300">
              {error}
            </p>
          )}
          {accepted && (
            <p className="p-3 text-sm rounded-xl bg-emerald-500/10 border border-emerald-500/25 text-emerald-300">
              Wrote {accepted.length} {accepted.length === 1 ? 'entity' : 'entities'}: {accepted.join(', ')}
            </p>
          )}

          <div className="space-y-2">
            <label htmlFor="batch-instruction" className="text-xs font-medium text-neutral-300">
              Instruction
            </label>
            <textarea
              id="batch-instruction"
              value={instruction}
              onChange={(event) => setInstruction(event.target.value)}
              rows={2}
              placeholder="three rival factions in the south"
              className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-sky-500/50 focus:outline-none resize-none"
            />
          </div>

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
            disabled={loading}
            className="flex items-center gap-2 px-4 py-2 text-sm text-neutral-200 rounded-xl border border-white/10 hover:bg-white/5 disabled:opacity-40 transition-colors"
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            {entities.length > 0 ? 'Regenerate' : 'Generate'}
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
