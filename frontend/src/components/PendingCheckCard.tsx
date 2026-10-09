import React from 'react';
import { Dices, PenLine, MessageSquareWarning } from 'lucide-react';
import { Adjudication, CounterProposal, PendingCheck } from '../types';
import { DiceEntry, diceRange, isOutOfRange, parseDiceEntry } from '../lib/diceRange';

interface PendingCheckCardProps {
  pending?: PendingCheck;
  busy: boolean;
  onRoll: () => void;
  onManual: (entry: DiceEntry) => void;
  // onArgue submits a counter-proposal and resolves with the GM's ruling, so the
  // card can show it beside the terms now in force.
  onArgue?: (counter: CounterProposal) => Promise<Adjudication | undefined>;
}

// PendingCheckCard is the prominent panel a GM-proposed check raises: the stakes,
// the notation and bonuses, the possible outcomes, and the Roll, Enter-a-roll,
// and Argue affordances. It renders nothing when there is no pending check.
export const PendingCheckCard: React.FC<PendingCheckCardProps> = ({ pending, busy, onRoll, onManual, onArgue }) => {
  const [manual, setManual] = React.useState('');
  const [arguing, setArguing] = React.useState(false);
  const [approach, setApproach] = React.useState('');
  const [proposedStakes, setProposedStakes] = React.useState('');
  const [proposedDifficulty, setProposedDifficulty] = React.useState('');
  const [ruling, setRuling] = React.useState<Adjudication | null>(null);
  const [argueBusy, setArgueBusy] = React.useState(false);

  if (!pending) return null;

  const request = pending.request ?? {};
  const stakes = (request.stakes ?? '').trim() || request.check_kind || 'The GM has called for a check.';
  const notation = pending.notation || request.notation || 'check';
  const bonuses = pending.bonuses ?? [];
  const outcomes = request.outcomes ?? {};

  // The entry is the player's dice, not the final total: the bonuses are shown
  // beside it and the computed total is previewed, so a double-count is visible.
  const entry = parseDiceEntry(manual);
  const range = diceRange(notation);
  const bonusTotal = bonuses.reduce((sum, bonus) => sum + bonus.value, 0);
  const previewTotal = entry ? entry.total + bonusTotal : 0;
  const outOfRange = entry ? isOutOfRange(entry, range) : false;

  const counter: CounterProposal = {
    approach: approach.trim() || undefined,
    stakes: proposedStakes.trim() || undefined,
    difficulty: proposedDifficulty.trim() || undefined,
  };
  const counterEmpty = !counter.approach && !counter.stakes && !counter.difficulty;

  const submitArgue = async () => {
    if (!onArgue || counterEmpty) return;
    setArgueBusy(true);
    try {
      const result = await onArgue(counter);
      if (result) setRuling(result);
    } finally {
      setArgueBusy(false);
    }
  };

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
            placeholder="4 3 or 9"
            className="w-28 bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-200 focus:border-purple-500 outline-none"
          />
          <button
            type="button"
            disabled={busy || !entry}
            onClick={() => entry && onManual(entry)}
            className="flex items-center gap-1.5 px-3 py-2 rounded-xl border border-stone-700 hover:border-purple-500/50 text-stone-300 hover:text-purple-300 disabled:opacity-50 text-xs font-sans cursor-pointer"
          >
            <PenLine className="w-3.5 h-3.5" />
            <span>Enter</span>
          </button>
        </div>
        <button
          type="button"
          disabled={busy}
          aria-expanded={arguing}
          onClick={() => setArguing((open) => !open)}
          className="flex items-center gap-1.5 px-3 py-2 rounded-xl border border-stone-700 hover:border-amber-500/50 text-stone-300 hover:text-amber-300 disabled:opacity-50 text-xs font-sans cursor-pointer"
        >
          <MessageSquareWarning className="w-3.5 h-3.5" />
          <span>Argue</span>
        </button>
      </div>
      {range && (
        <div className="text-[11px] font-sans text-stone-500">
          {notation} can roll {range.min}-{range.max}. Enter your dice, for example 4 3.
        </div>
      )}
      {entry && (
        <div data-section="manual-preview" className="text-[11px] font-mono text-stone-400">
          {entry.dice.join(' + ')} = {entry.total}
          {bonusTotal !== 0 ? ` ${bonusTotal > 0 ? '+' : '-'} ${Math.abs(bonusTotal)} = ${previewTotal}` : ''}
          {outOfRange ? ' (outside the usual range; still accepted)' : ''}
        </div>
      )}
      {arguing && (
        <div data-section="argue" className="space-y-2 pt-2 border-t border-white/10">
          <div className="text-xs font-sans font-bold uppercase tracking-wider text-amber-300">Argue the check</div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
            <label className="space-y-1 md:col-span-2">
              <span className="text-[11px] font-sans uppercase text-stone-400">Approach</span>
              <textarea
                value={approach}
                onChange={(e) => setApproach(e.target.value)}
                aria-label="Approach"
                rows={2}
                placeholder="lift the bar instead of picking the lock"
                className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-sans text-stone-200 focus:border-amber-500 outline-none resize-none"
              />
            </label>
            <label className="space-y-1">
              <span className="text-[11px] font-sans uppercase text-stone-400">Stakes</span>
              <input
                value={proposedStakes}
                onChange={(e) => setProposedStakes(e.target.value)}
                aria-label="Proposed stakes"
                className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-sans text-stone-200 focus:border-amber-500 outline-none"
              />
            </label>
            <label className="space-y-1">
              <span className="text-[11px] font-sans uppercase text-stone-400">Difficulty</span>
              <input
                value={proposedDifficulty}
                onChange={(e) => setProposedDifficulty(e.target.value)}
                aria-label="Proposed difficulty"
                placeholder="risky"
                className="w-full bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-xs font-mono text-stone-200 focus:border-amber-500 outline-none"
              />
            </label>
          </div>
          <button
            type="button"
            disabled={busy || argueBusy || counterEmpty}
            onClick={submitArgue}
            className="px-3 py-2 rounded-xl border border-amber-500/50 text-amber-200 hover:bg-amber-500/10 disabled:opacity-50 text-xs font-sans cursor-pointer"
          >
            {argueBusy ? 'Arguing...' : 'Make the case'}
          </button>
          {ruling && (
            <div data-section="ruling" className="text-[11px] font-sans text-stone-300 space-y-0.5">
              <div>
                <span className="font-mono uppercase tracking-wider text-amber-300">{ruling.ruling}</span>
                {ruling.reason ? `: ${ruling.reason}` : ''}
              </div>
              {ruling.ruling !== 'hold' && (ruling.stakes || ruling.difficulty) && (
                <div className="text-stone-400">
                  Agreed: {[ruling.stakes, ruling.difficulty].filter(Boolean).join(' / ')}
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};
