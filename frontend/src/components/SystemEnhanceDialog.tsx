import React, { useState } from 'react';
import { Check, Lightbulb, Sparkles, TriangleAlert, X } from 'lucide-react';
import { APIClient } from '../api/client';
import { SystemEnhanceApplyResult, SystemProposal } from '../types';

export interface SystemEnhanceDialogProps {
  systemId: string;
  onClose: () => void;
  onApplied?: (result: SystemEnhanceApplyResult) => void;
  // proposals seeds the diff, so a caller that already fetched it does not ask
  // the model twice.
  proposals?: SystemProposal[];
}

const KIND_LABELS: Record<string, string> = {
  stat: 'Stat',
  skill: 'Skill',
  profile: 'Resolution profile',
  advancement: 'Advancement',
};

function proposalSummary(proposal: SystemProposal): string {
  switch (proposal.kind) {
    case 'stat':
      return proposal.stat ? `Declares stat ${proposal.stat.id}` : '';
    case 'skill':
      return proposal.skill
        ? `Declares skill ${proposal.skill.id}${proposal.skill.stat ? ` on ${proposal.skill.stat}` : ''}`
        : '';
    case 'profile':
      if (!proposal.profile) return '';
      if (proposal.profile.dc) return `Adds profile ${proposal.profile.name} (DC ${proposal.profile.dc})`;
      if (proposal.profile.success_on) return `Adds pool profile ${proposal.profile.name}`;
      return `Adds ladder profile ${proposal.profile.name}`;
    case 'advancement':
      return proposal.advancement
        ? `Adds ${proposal.advancement.mode || 'spend'} advancement`
        : '';
    default:
      return '';
  }
}

// SystemEnhanceDialog proposes additive changes to an existing system as an
// accept-or-reject diff. Applying sends only the accepted, valid proposals.
export const SystemEnhanceDialog: React.FC<SystemEnhanceDialogProps> = ({
  systemId,
  onClose,
  onApplied,
  proposals: seeded,
}) => {
  const [instruction, setInstruction] = useState('');
  const [proposals, setProposals] = useState<SystemProposal[]>(seeded ?? []);
  const [rejected, setRejected] = useState<Set<number>>(new Set());
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [applied, setApplied] = useState<string[] | null>(null);
  const [oracle, setOracle] = useState(false);

  const accepted = proposals.filter(
    (proposal, index) => proposal.valid !== false && !rejected.has(index)
  );

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
      const response = await APIClient.enhanceSystem(systemId, { instruction });
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
      const result = await APIClient.applySystemEnhancements(systemId, { proposals: accepted });
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
              <h3 className="text-lg font-semibold text-white">Enhance this system</h3>
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
              No model provider is configured, so no proposals were generated. Assign one in Settings
              → AI Agents to propose additions for this system.
            </p>
          )}
          {applied && (
            <p className="p-3 text-sm rounded-xl bg-emerald-500/10 border border-emerald-500/25 text-emerald-300">
              Applied: {applied.join(', ')}
            </p>
          )}

          <div className="space-y-2">
            <label htmlFor="system-enhance-instruction" className="text-xs font-medium text-neutral-300">
              What should be added?
            </label>
            <textarea
              id="system-enhance-instruction"
              value={instruction}
              onChange={(event) => setInstruction(event.target.value)}
              rows={2}
              placeholder="add sanity and occult skills, and an advancement track"
              className="w-full px-3 py-2 text-sm text-white bg-white/[0.03] border border-white/10 rounded-xl focus:border-amber-500/50 focus:outline-none resize-none"
            />
          </div>

          {proposals.map((proposal, index) => {
            const isAccepted = proposal.valid !== false && !rejected.has(index);
            return (
              <div
                key={`${proposal.kind}-${proposal.title}-${index}`}
                className={`p-4 rounded-xl border transition-colors ${
                  isAccepted ? 'border-white/10 bg-white/[0.03]' : 'border-white/5 bg-white/[0.01] opacity-60'
                }`}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <h4 className="text-sm font-semibold text-white truncate">{proposal.title}</h4>
                    <span className="text-[11px] uppercase tracking-wide text-neutral-500">
                      {KIND_LABELS[proposal.kind] ?? proposal.kind}
                    </span>
                  </div>
                  <button
                    type="button"
                    aria-label={
                      isAccepted ? `Reject ${proposal.title}` : `Accept ${proposal.title}`
                    }
                    onClick={() => toggle(index)}
                    disabled={proposal.valid === false}
                    className={`p-1.5 rounded-lg hover:bg-white/5 shrink-0 disabled:opacity-40 ${
                      isAccepted ? 'text-emerald-400' : 'text-neutral-500'
                    }`}
                  >
                    {isAccepted ? <Check className="w-4 h-4" /> : <X className="w-4 h-4" />}
                  </button>
                </div>
                <p className="mt-2 text-xs text-neutral-300">{proposalSummary(proposal)}</p>
                {proposal.reason && (
                  <p className="mt-1 text-[11px] text-neutral-500 italic">Why: {proposal.reason}</p>
                )}
                {proposal.valid === false && (
                  <p className="mt-2 flex items-start gap-1.5 text-[11px] text-red-300">
                    <TriangleAlert className="w-3.5 h-3.5 shrink-0 mt-0.5" />
                    <span>{proposal.problems?.join('; ') || 'This addition would break the system.'}</span>
                  </p>
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

        <div className="flex items-center justify-end gap-3 p-6 border-t border-white/10">
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
  );
};

export default SystemEnhanceDialog;
