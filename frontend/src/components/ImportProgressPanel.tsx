import React, { useEffect, useRef, useState } from 'react';
import { FileText, Loader2 } from 'lucide-react';
import { WorldImportProgress } from '../types';

export interface ImportProgressPanelProps {
  progress: WorldImportProgress;
}

// formatRemaining renders a duration as the roughest useful unit. A long import
// is minutes; seconds are noise beyond the first one.
export function formatRemaining(seconds: number): string {
  if (seconds < 5) return 'a few seconds';
  if (seconds < 90) return `about ${Math.round(seconds / 5) * 5} seconds`;
  return `about ${Math.round(seconds / 60)} min`;
}

// ImportProgressPanel reports an import batch by batch: what it is reading, what
// it has found, and how much is left. Reading a folder of two hundred pages is
// fifty model calls, and a dialog that says nothing for that looks hung.
export const ImportProgressPanel: React.FC<ImportProgressPanelProps> = ({ progress }) => {
  const { batch, batches, sources, found, total, names, cut_off: cutOff } = progress;

  // The estimate comes from the batches finished so far, so it sharpens as the
  // import runs. A batch of one is the first real measurement.
  const startedAt = useRef(Date.now());
  const [remaining, setRemaining] = useState<number | null>(null);

  useEffect(() => {
    if (batch < 1 || batches <= batch) {
      setRemaining(null);
      return;
    }
    const perBatch = (Date.now() - startedAt.current) / batch;
    setRemaining((perBatch * (batches - batch)) / 1000);
  }, [batch, batches]);

  const percent = batches > 0 ? Math.min(100, Math.round((batch / batches) * 100)) : 0;
  const reading = sources?.length ? sources.join(', ') : 'the source';

  return (
    <div className="p-4 rounded-xl bg-sky-500/5 border border-sky-500/20 space-y-2.5" aria-live="polite">
      <div className="flex items-center gap-2 text-xs text-sky-200">
        <Loader2 className="w-3.5 h-3.5 animate-spin" />
        <span data-testid="import-progress-summary">
          {batch === 0
            ? `Reading ${batches} ${batches === 1 ? 'batch' : 'batches'} of the source…`
            : `Read ${batch} of ${batches} batches`}
          {remaining !== null && ` · ${formatRemaining(remaining)} left`}
        </span>
      </div>

      <div className="h-1.5 rounded-full bg-white/5 overflow-hidden">
        <div
          role="progressbar"
          aria-valuenow={percent}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label="Import progress"
          className="h-full bg-sky-400/70 transition-all"
          style={{ width: `${percent}%` }}
        />
      </div>

      <p className="flex items-start gap-1.5 text-[11px] text-neutral-400">
        <FileText className="w-3.5 h-3.5 shrink-0 mt-0.5" />
        <span className="min-w-0 break-words">{reading}</span>
      </p>

      <p className="text-[11px] text-neutral-300">
        {total} {total === 1 ? 'entity' : 'entities'} so far
        {found > 0 && `, ${found} from this batch`}
      </p>

      {names && names.length > 0 && (
        <p className="text-[11px] text-neutral-500 break-words">{names.slice(0, 12).join(', ')}</p>
      )}

      {cutOff ? (
        <p className="text-[11px] text-amber-400/80">
          {cutOff} {cutOff === 1 ? 'batch' : 'batches'} ran out of room before finishing. What they
          wrote was kept; the rest of {cutOff === 1 ? 'that page' : 'those pages'} was not read.
        </p>
      ) : null}
    </div>
  );
};

export default ImportProgressPanel;
