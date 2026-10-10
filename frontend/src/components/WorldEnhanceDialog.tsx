import React, { useState } from 'react';
import { Check, Lightbulb, Sparkles, X } from 'lucide-react';
import { APIClient } from '../api/client';
import { slugify } from '../lib/slug';
import { WorldApplyResult, WorldEnhancement } from '../types';

export interface WorldEnhanceDialogProps {
  worldId: string;
  onClose: () => void;
  onApplied?: (result: WorldApplyResult) => void;
  // proposals seeds the diff, so a caller that already fetched it does not ask
  // the model twice.
  proposals?: WorldEnhancement[];
}

const KIND_LABELS: Record<string, string> = {
  lore: 'Lore',
  entity: 'New entity',
  hook: 'Story hook',
};

// WorldEnhanceDialog proposes additions to an existing world as an
// accept-or-reject diff. Applying sends only the accepted proposals.
export const WorldEnhanceDialog: React.FC<WorldEnhanceDialogProps> = ({
  worldId,
  onClose,
  onApplied,
  proposals: seeded,
}) => {
  const [instruction, setInstruction] = useState('');
  const [proposals, setProposals] = useState<WorldEnhancement[]>(seeded ?? []);
  const [rejected, setRejected] = useState<Set<number>>(new Set());
  const [rename, setRename] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [applied, setApplied] = useState<string[] | null>(null);
  const [oracle, setOracle] = useState(false);

  const accepted = proposals.filter((_, index) => !rejected.has(index));

  const toggle = (index: number) => {
    setRejected((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  };

  const handleGenerate = async () => {
    setLoading(true);
    setError(null);
    setApplied(null);
    setOracle(false);
    try {
      const response = await APIClient.enhanceWorld(worldId, { instruction });
      setProposals(response.proposals);
      setOracle(response.oracle === true);
      setRejected(new Set());
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  const handleApply = async () => {
    if (accepted.length === 0) return;
    setLoading(true);
    setError(null);
    try {
      // A model-supplied id is not guaranteed to slug, and the backend rejects one
      // that does not, so normalise it here rather than fail the apply.
      const normalized = accepted.map((proposal) =>
        proposal.entity
          ? {
              ...proposal,
              entity: {
                ...proposal.entity,
                id: slugify(proposal.entity.id || proposal.entity.name),
              },
            }
          : proposal,
      );
      const result = await APIClient.applyWorldEnhancements(worldId, { proposals: normalized, rename });
      setApplied(result.written);
      setProposals([]);
      setRejected(new Set());
      onApplied?.(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div className="relative w-full max-w-2xl overflow-hidden border border-white/10 rounded-2xl bg-neutral-900/90 shadow-2xl backdrop-blur-xl">
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-xl bg-amber-500/10 border border-amber-500/20 text-amber-400">
              <Lightbulb className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-white">Enhance this world</h3>
              <p className="text-xs text-neutral-400">
                {accepted.length} of {proposals.length} accepted
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
          {error && (
            <p className="p-3 text-sm rounded-xl bg-red-950/40 border border-red-500/30 text-red-300">
              {error}
            </p>
          )}
          {oracle && (
            <p className="p-3 text-xs rounded-xl bg-amber-500/10 border border-amber-500/25 text-amber-200">
              No model provider is configured, so these came from the built-in template generator.
              Assign one in Settings → AI Agents for proposals drawn from this world.
            </p>
          )}
          {applied && (
            <p className="p-3 text-sm rounded-xl bg-emerald-500/10 border border-emerald-500/25 text-emerald-300">
              Applied: {applied.join(', ')}
            </p>
          )}

          <div className="space-y-2">
            <label htmlFor="enhance-instruction" className="text-xs font-medium text-neutral-300">
              What should change?
            </label>
            <textarea
              id="enhance-instruction"
              value={instruction}
              onChange={(event) => setInstruction(event.target.value)}
              rows={2}
              placeholder="deepen the history, and add a rival for the captain"
              className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-amber-500/50 focus:outline-none resize-none"
            />
          </div>

          {proposals.map((proposal, index) => {
            const isAccepted = !rejected.has(index);
            return (
              <div
                key={`${proposal.kind}-${proposal.title}-${index}`}
                className={`p-4 rounded-xl border transition-colors ${
                  isAccepted ? 'border-white/10 bg-white/[0.03]' : 'border-white/5 bg-white/[0.01] opacity-50'
                }`}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <h4 className="text-sm font-semibold text-white truncate">{proposal.title}</h4>
                    <span className="text-[11px] uppercase tracking-wide text-neutral-500">
                      {KIND_LABELS[proposal.kind] ?? proposal.kind}
                      {proposal.target ? ` · under ${proposal.target}` : ''}
                    </span>
                  </div>
                  <button
                    type="button"
                    aria-label={
                      isAccepted
                        ? `Reject ${KIND_LABELS[proposal.kind] ?? proposal.kind} ${proposal.title}`
                        : `Accept ${KIND_LABELS[proposal.kind] ?? proposal.kind} ${proposal.title}`
                    }
                    onClick={() => toggle(index)}
                    className={`p-1.5 rounded-lg hover:bg-white/5 shrink-0 ${
                      isAccepted ? 'text-emerald-400' : 'text-neutral-500'
                    }`}
                  >
                    {isAccepted ? <Check className="w-4 h-4" /> : <X className="w-4 h-4" />}
                  </button>
                </div>
                <p className="mt-2 text-xs text-neutral-300 whitespace-pre-wrap line-clamp-4">
                  {proposal.entity?.body ?? proposal.body}
                </p>
                {proposal.entity && (
                  <p className="mt-1 text-[11px] text-neutral-500">
                    Writes entity <span className="font-mono">{proposal.entity.id}</span> ({proposal.entity.type})
                  </p>
                )}
                {proposal.reason && (
                  <p className="mt-1 text-[11px] text-neutral-500 italic">Why: {proposal.reason}</p>
                )}
              </div>
            );
          })}

          {proposals.length === 0 && (
            <p className="flex items-center gap-2 text-xs text-neutral-500">
              <Sparkles className="w-4 h-4" />
              Ask for a change to see proposals.
            </p>
          )}
        </div>

        <div className="flex items-center justify-between gap-3 p-6 border-t border-white/10">
          <label className="flex items-center gap-2 text-[11px] text-neutral-400">
            <input type="checkbox" checked={rename} onChange={(event) => setRename(event.target.checked)} />
            Rename a new entity on an id clash
          </label>
          <div className="flex items-center gap-3">
            <button
              onClick={() => void handleGenerate()}
              disabled={loading}
              className="px-4 py-2 text-sm text-neutral-200 rounded-xl border border-white/10 hover:bg-white/5 disabled:opacity-40 transition-colors"
            >
              {proposals.length > 0 ? 'Regenerate' : 'Propose changes'}
            </button>
            <button
              onClick={() => void handleApply()}
              disabled={loading || accepted.length === 0}
              className="px-4 py-2 text-sm font-medium text-white rounded-xl bg-amber-600 hover:bg-amber-500 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Apply accepted
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default WorldEnhanceDialog;
