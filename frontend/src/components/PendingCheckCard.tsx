import React from 'react';
import { Dices, PenLine, MessageSquareWarning } from 'lucide-react';
import { PendingCheck } from '../types';

interface PendingCheckCardProps {
  pending?: PendingCheck;
  busy: boolean;
  onRoll: () => void;
  onManual: (total: number) => void;
  onArgue: () => void;
}

// PendingCheckCard is the prominent panel a GM-proposed check raises: the stakes,
// the notation and bonuses, the possible outcomes, and the Roll, Enter-a-roll,
// and Argue affordances. It renders nothing when there is no pending check.
export const PendingCheckCard: React.FC<PendingCheckCardProps> = ({ pending, busy, onRoll, onManual, onArgue }) => {
  const [manual, setManual] = React.useState('');

  if (!pending) return null;

  const request = pending.request ?? {};
  const stakes = (request.stakes ?? '').trim() || request.check_kind || 'The GM has called for a check.';
  const notation = pending.notation || request.notation || 'check';
  const bonuses = pending.bonuses ?? [];
  const outcomes = request.outcomes ?? {};
  const manualValue = manual.trim() === '' ? NaN : Number(manual);
  const manualValid = Number.isFinite(manualValue);

  return (
    <div className="mx-4 mb-2 rounded-xl border border-purple-500/40 bg-purple-950/30 px-4 py-3 space-y-2">
      <div className="text-xs font-sans font-bold uppercase tracking-wider text-purple-300">Roll required</div>
      <div className="text-sm font-sans text-stone-200">{stakes}</div>
      <div className="flex flex-wrap items-center gap-2 text-[11px] font-mono text-stone-400">
        <span className="text-stone-300">{notation}</span>
        {bonuses.map((bonus, index) => (
          <span key={index} className="rounded bg-white/5 px-1.5 py-0.5">
            {bonus.source} {bonus.value >= 0 ? `+${bonus.value}` : bonus.value}
          </span>
        ))}
      </div>
      {Object.keys(outcomes).length > 0 && (
        <div className="text-[11px] font-sans text-stone-400 space-y-0.5">
          {Object.entries(outcomes).map(([key, text]) => (
            <div key={key}>
              <span className="font-mono text-stone-300">{key}</span>: {text}
            </div>
          ))}
        </div>
      )}
      <div className="flex flex-wrap items-center gap-2 pt-1">
        <button
          type="button"
          disabled={busy}
          onClick={onRoll}
          className="flex items-center gap-1.5 px-4 py-2 rounded-xl bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white text-xs font-sans font-bold cursor-pointer"
        >
          <Dices className="w-3.5 h-3.5" />
          <span>Roll</span>
        </button>
        <div className="flex items-center gap-1">
          <input
            value={manual}
            onChange={(e) => setManual(e.target.value)}
            inputMode="numeric"
            aria-label="Manual roll"
            placeholder="Enter a roll"
            className="w-28 bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-200 focus:border-purple-500 outline-none"
          />
          <button
            type="button"
            disabled={busy || !manualValid}
            onClick={() => onManual(manualValue)}
            className="flex items-center gap-1.5 px-3 py-2 rounded-xl border border-stone-700 hover:border-purple-500/50 text-stone-300 hover:text-purple-300 disabled:opacity-50 text-xs font-sans cursor-pointer"
          >
            <PenLine className="w-3.5 h-3.5" />
            <span>Enter</span>
          </button>
        </div>
        <button
          type="button"
          disabled={busy}
          onClick={onArgue}
          className="flex items-center gap-1.5 px-3 py-2 rounded-xl border border-stone-700 hover:border-amber-500/50 text-stone-300 hover:text-amber-300 disabled:opacity-50 text-xs font-sans cursor-pointer"
        >
          <MessageSquareWarning className="w-3.5 h-3.5" />
          <span>Argue</span>
        </button>
      </div>
    </div>
  );
};
