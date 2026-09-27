import React from 'react';

interface MechanicsStripProps {
  engagement?: 'off' | 'auto' | 'ask';
  checks: number;
  outcome?: string;
}

// MechanicsStrip is the one-line readout above the action console: which policy
// is in force and what the last turn's mechanics did, so a quiet turn still says
// the system is present.
export const MechanicsStrip: React.FC<MechanicsStripProps> = ({ engagement, checks, outcome }) => {
  if (!engagement) return null;
  const summary =
    checks === 0
      ? 'no checks this turn'
      : `${checks} ${checks === 1 ? 'check' : 'checks'}${outcome ? ` (${outcome})` : ''}`;
  return (
    <div className="flex items-center gap-2 px-4 pb-1 text-xs font-sans text-stone-400">
      <span className="uppercase tracking-wider text-stone-500">Mechanics: {engagement}</span>
      <span className="opacity-40">&middot;</span>
      <span>{summary}</span>
    </div>
  );
};
