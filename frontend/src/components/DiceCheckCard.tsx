import React from 'react';
import { TurnCheck } from '../types';

export type CheckTone = 'success' | 'partial' | 'failure' | 'neutral';

// classifyCheckOutcome maps a resolver's arbitrary outcome word to a tone. The
// order matters: a costed success is partial, and a critical failure is a
// failure before it is anything else.
export function classifyCheckOutcome(outcome: string): CheckTone {
  const value = (outcome || '').trim().toLowerCase();
  if (!value) return 'neutral';
  if (/(partial|mixed|success_with_cost|complication)/.test(value)) return 'partial';
  if (/(fail|failure)/.test(value)) return 'failure';
  if (/(success|pass|critical|succeed)/.test(value)) return 'success';
  return 'neutral';
}

function diceCount(notation?: string): number {
  if (!notation) return 1;
  const match = notation.match(/(\d*)\s*d\s*(\d+)/i);
  if (!match) return 1;
  const count = match[1] ? parseInt(match[1], 10) : 1;
  return Number.isFinite(count) && count > 0 ? count : 1;
}

function dieSize(notation?: string): string {
  const match = notation?.match(/d\s*(\d+)/i);
  return match ? match[1] : '?';
}

const TONE_STYLES: Record<CheckTone, { border: string; chip: string }> = {
  success: { border: 'border-emerald-500/60', chip: 'text-emerald-300' },
  partial: { border: 'border-amber-500/60', chip: 'text-amber-300' },
  failure: { border: 'border-rose-500/60', chip: 'text-rose-400' },
  neutral: { border: 'border-stone-500/50', chip: 'text-stone-300' },
};

export const DiceCheckCard: React.FC<{ check: TurnCheck }> = ({ check }) => {
  const tone = classifyCheckOutcome(check.outcome);
  const style = TONE_STYLES[tone];
  const roll = check.roll;
  const notation = roll?.notation ?? 'check';
  const count = Math.max(1, roll?.roll_count && roll.roll_count > 0 ? roll.roll_count : diceCount(roll?.notation));
  const shown = Math.min(count, 6);
  const size = dieSize(roll?.notation);
  const stakes =
    (check.stakes ?? '').trim() ||
    [check.actor, check.target].filter(Boolean).join(' vs ') ||
    (check.check_kind ?? '').trim();
  const label = `${check.actor ? `${check.actor} ` : ''}${notation}${roll ? ` = ${roll.total}` : ''}, ${check.outcome}${
    stakes ? `: ${stakes}` : ''
  }`;

  return (
    <div className={`my-3 rounded-xl border-l-4 ${style.border} bg-black/30 px-3 py-2 space-y-1`} aria-label={label}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="flex items-center gap-1" aria-hidden="true">
          {Array.from({ length: shown }).map((_, index) => (
            <svg key={index} viewBox="0 0 24 24" className="w-5 h-5 text-stone-300">
              <rect
                x="2"
                y="2"
                width="20"
                height="20"
                rx="5"
                fill="currentColor"
                opacity="0.12"
                stroke="currentColor"
                strokeWidth="1.5"
              />
              <text x="12" y="15.5" textAnchor="middle" fontSize="9" fill="currentColor" fontFamily="monospace">
                {size}
              </text>
            </svg>
          ))}
          {count > shown && <span className="text-xs font-mono text-stone-400">+{count - shown}</span>}
        </span>
        <span className="text-xs font-mono text-stone-300">{notation}</span>
        {roll && <span className="text-xs font-mono text-stone-400">&rarr; {roll.total}</span>}
        {roll && roll.successes !== undefined && roll.successes > 0 && (
          <span className="text-xs font-mono text-stone-400">{roll.successes} successes</span>
        )}
        <span className={`text-xs font-sans font-bold uppercase tracking-wider ${style.chip}`}>{check.outcome}</span>
      </div>
      {stakes && <div className="text-xs font-sans text-stone-400">{stakes}</div>}
    </div>
  );
};
