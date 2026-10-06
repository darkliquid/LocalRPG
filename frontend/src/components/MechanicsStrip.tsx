import React from 'react';
import { TurnCheck } from '../types';

interface MechanicsStripProps {
  turn?: { engagement?: string; checks?: TurnCheck[] };
}

// MechanicsStrip is the one-line readout above the action console: which policy
// is in force and what the last turn's mechanics did, so a quiet turn still says
// the system is present. It renders nothing when mechanics are off.
export const MechanicsStrip: React.FC<MechanicsStripProps> = ({ turn }) => {
  const engagement = turn?.engagement;
  if (!engagement || engagement === 'off') return null;
  const checks = turn?.checks ?? [];
  const summary = checks.length === 0 ? 'checks: none' : `checks: ${checks.length}`;
  return (
    <div className="flex flex-wrap items-center gap-2 px-4 pb-1 text-xs font-sans text-stone-400">
      <span className="uppercase tracking-wider text-stone-500">Mechanics: {engagement}</span>
      <span className="opacity-40">&middot;</span>
      <span>{summary}</span>
      {checks.map((check, index) => {
        const notation = check.roll?.notation;
        const total = check.roll?.total;
        const dice = notation && total !== undefined ? `${notation} \u2192 ${total}` : notation;
        const stakes = check.position && check.effect ? `${check.position}/${check.effect}` : undefined;
        const detail = [dice, stakes, check.outcome].filter(Boolean).join(' ');
        return (
          <span key={index} className="rounded bg-white/5 px-1.5 py-0.5 font-mono text-[11px] text-stone-300">
            {detail}
          </span>
        );
      })}
    </div>
  );
};
