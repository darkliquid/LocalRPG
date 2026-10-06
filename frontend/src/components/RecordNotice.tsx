import React from 'react';
import type { RecordReport } from '../types';

// summary names what happened, so the chip reads without opening it.
function summary(report: RecordReport): string {
  const parts: string[] = [];
  if (report.repaired > 0) parts.push(`${report.repaired} repaired`);
  if (report.failed > 0) parts.push(`${report.failed} dropped`);
  return `Record trouble: ${parts.join(', ')}`;
}

// RecordNotice surfaces a turn whose control records were repaired or dropped.
// It renders nothing for a clean turn, so the common case stays quiet.
export const RecordNotice: React.FC<{ report?: RecordReport }> = ({ report }) => {
  if (!report || (report.repaired === 0 && report.failed === 0)) return null;
  const warning = report.failed > 0;
  return (
    <details
      className={`my-2 rounded-lg border px-3 py-2 text-xs ${
        warning ? 'border-amber-500/40 bg-amber-500/10 text-amber-200' : 'border-white/10 bg-white/5 text-zinc-300'
      }`}
    >
      <summary className="cursor-pointer select-none">{summary(report)}</summary>
      <ul className="mt-2 space-y-1">
        {(report.issues ?? []).map((issue, index) => (
          <li key={index}>
            <span className="font-mono">{issue.type}</span>
            {issue.repair ? `: repaired (${issue.repair})` : `: dropped: ${issue.error}`}
          </li>
        ))}
      </ul>
    </details>
  );
};
