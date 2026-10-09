import React from 'react';
import { TurnCheck } from '../types';

interface MechanicsStripProps {
  turn?: { engagement?: string; checks?: TurnCheck[] };
  // budget is the campaign's image allowance, shown when one is set so a player
  // can see how many images remain before a generation is skipped.
  budget?: { max_images?: number; used_images?: number };
}

// imageBudgetLabel names the images a campaign has left, or null when the budget
// is unlimited.
function imageBudgetLabel(budget?: { max_images?: number; used_images?: number }): string | null {
  const max = budget?.max_images ?? 0;
  if (max <= 0) return null;
  const left = Math.max(0, max - (budget?.used_images ?? 0));
  return left === 0 ? 'images: budget spent' : `images: ${left} left`;
}

// MechanicsStrip is the one-line readout above the action console: which policy
// is in force and what the last turn's mechanics did, so a quiet turn still says
// the system is present. It renders nothing when mechanics are off.
export const MechanicsStrip: React.FC<MechanicsStripProps> = ({ turn, budget }) => {
  const engagement = turn?.engagement;
  const budgetLabel = imageBudgetLabel(budget);
  if ((!engagement || engagement === 'off') && !budgetLabel) return null;
  const checks = turn?.checks ?? [];
  const summary = checks.length === 0 ? 'checks: none' : `checks: ${checks.length}`;
  return (
    <div className="flex flex-wrap items-center gap-2 px-4 pb-1 text-xs font-sans text-stone-400">
      {engagement && engagement !== 'off' && (
        <>
          <span className="uppercase tracking-wider text-stone-500">Mechanics: {engagement}</span>
          <span className="opacity-40">&middot;</span>
          <span>{summary}</span>
        </>
      )}
      {budgetLabel && (
        <>
          {engagement && engagement !== 'off' && <span className="opacity-40">&middot;</span>}
          <span data-section="image-budget" className="uppercase tracking-wider text-amber-300/80">
            {budgetLabel}
          </span>
        </>
      )}
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
