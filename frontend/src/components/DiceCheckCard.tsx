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

const TONE_STYLES: Record<CheckTone, { border: string; chip: string }> = {
  success: { border: 'border-emerald-500/60', chip: 'text-emerald-300' },
  partial: { border: 'border-amber-500/60', chip: 'text-amber-300' },
  failure: { border: 'border-rose-500/60', chip: 'text-rose-400' },
  neutral: { border: 'border-stone-500/50', chip: 'text-stone-300' },
};

// MAX_SHOWN_DICE keeps a large pool readable: the rest is a count.
const MAX_SHOWN_DICE = 6;

export const DiceCheckCard: React.FC<{ check: TurnCheck }> = ({ check }) => {
  const tone = classifyCheckOutcome(check.outcome);
  const style = TONE_STYLES[tone];
  const roll = check.roll;
  const notation = roll?.notation ?? 'check';
  // Only the faces that landed can be drawn. A record written before the engine
  // carried them has a total and no dice, and a guessed face would be a lie.
  const faces = roll?.dice ?? [];
  const shown = Math.min(faces.length, MAX_SHOWN_DICE);
  const applied = check.applied ?? [];
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
        {shown > 0 && (
          <span className="flex items-center gap-1" aria-hidden="true">
            {faces.slice(0, shown).map((die, index) => {
              const face = die.symbol || String(die.value);
              return (
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
                  <text
                    x="12"
                    y="15.5"
                    textAnchor="middle"
                    fontSize={face.length > 2 ? 7 : 9}
                    fill="currentColor"
                    fontFamily="monospace"
                  >
                    {face}
                  </text>
                </svg>
              );
            })}
            {faces.length > shown && <span className="text-xs font-mono text-stone-400">+{faces.length - shown}</span>}
          </span>
        )}
        <span className="text-xs font-mono text-stone-300">{notation}</span>
        {roll && <span className="text-xs font-mono text-stone-400">&rarr; {roll.total}</span>}
        {roll && roll.successes !== undefined && roll.successes > 0 && (
          <span className="text-xs font-mono text-stone-400">{roll.successes} successes</span>
        )}
        <span className={`text-xs font-sans font-bold uppercase tracking-wider ${style.chip}`}>{check.outcome}</span>
      </div>
      {applied.length > 0 && (
        <div className="flex flex-wrap items-center gap-1 text-[11px] font-mono text-stone-400">
          <span className="text-stone-500">{notation}{roll ? ` ${roll.total}` : ''}</span>
          {applied.map((modifier, index) => (
            <span key={index} className="rounded bg-white/5 px-1.5 py-0.5">
              {modifier.source} {modifier.value >= 0 ? `+${modifier.value}` : modifier.value}
            </span>
          ))}
        </div>
      )}
      {(check.profile || check.position || check.effect || (check.successes ?? 0) > 0) && (
        <div className="flex flex-wrap items-center gap-1 text-[11px] font-mono text-stone-400">
          {check.profile && <span className="rounded bg-white/5 px-1.5 py-0.5">{check.profile}</span>}
          {check.position && <span className="rounded bg-white/5 px-1.5 py-0.5">{check.position}</span>}
          {check.effect && <span className="rounded bg-white/5 px-1.5 py-0.5">{check.effect}</span>}
          {(check.successes ?? 0) > 0 && roll?.successes === undefined && (
            <span>{check.successes} successes</span>
          )}
        </div>
      )}
      {stakes && <div className="text-xs font-sans text-stone-400">{stakes}</div>}
    </div>
  );
};
