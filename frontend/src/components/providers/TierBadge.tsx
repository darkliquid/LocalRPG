import React from 'react';

// TierBadge labels a provider with its capability tier, so a user can tell an
// offline template from a cloud model at a glance. The caveat is shown as a
// tooltip and inline, because the honest "what it is not" is the point.
const LABELS: Record<string, string> = {
  'offline-basic': 'Offline · basic',
  'offline-neural': 'Offline · small model',
  'local-server': 'Local server',
  'cloud': 'Cloud',
};

// tierLabel is the user-facing name of a tier, or the raw value when unknown.
export function tierLabel(tier?: string): string {
  if (!tier) return '';
  return LABELS[tier] ?? tier;
}

// tierTone picks a colour family that matches how local the provider is.
function tierTone(tier: string): string {
  if (tier === 'cloud') return 'border-sky-400/40 bg-sky-400/10 text-sky-200';
  if (tier === 'local-server') return 'border-violet-400/40 bg-violet-400/10 text-violet-200';
  return 'border-emerald-400/40 bg-emerald-400/10 text-emerald-200';
}

export const TierBadge: React.FC<{ tier?: string; caveat?: string }> = ({ tier, caveat }) => {
  if (!tier) return null;
  return (
    <span
      title={caveat}
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-medium ${tierTone(tier)}`}
    >
      {tierLabel(tier)}
    </span>
  );
};

// TierLegend explains the four tiers, so the badge vocabulary is never a mystery.
export const TierLegend: React.FC<{ caveats: Record<string, string> }> = ({ caveats }) => (
  <ul className="space-y-1.5">
    {Object.entries(caveats).map(([tier, caveat]) => (
      <li key={tier} className="flex items-start gap-2">
        <TierBadge tier={tier} caveat={caveat} />
        <span className="text-[11px] text-stone-400">{caveat}</span>
      </li>
    ))}
  </ul>
);
