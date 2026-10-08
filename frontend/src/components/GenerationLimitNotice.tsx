import React, { useEffect, useState } from 'react';
import { AlertTriangle, Gauge } from 'lucide-react';
import { APIClient } from '../api/client';
import { GenerationLimitsOverride } from '../types';
import { generationLimitHint, limitFieldFor } from '../lib/generationLimit';

export interface GenerationLimitNoticeProps {
  // code is the limit that stopped the run: the call limit or the source limit.
  code: string;
  // message is what the server said, which reports the size and the limit it hit.
  message: string;
  busy?: boolean;
  // onRetry runs again with a higher limit for this run only.
  onRetry: (limits: GenerationLimitsOverride) => void;
  // onSave raises the limit in the configuration as well, so later runs use it.
  onSave: (limits: GenerationLimitsOverride) => void;
}

// defaultsFor is the fallback when the config cannot be read, so the field is
// still usable.
const defaultsFor = { max_calls: 20, max_chunks: 200 } as const;

// GenerationLimitNotice lets a run that hit a limit carry on from where it is,
// instead of sending the user to Settings and back. Raising it for one run and
// raising it for good are different intentions, so both are offered.
export const GenerationLimitNotice: React.FC<GenerationLimitNoticeProps> = ({
  code,
  message,
  busy = false,
  onRetry,
  onSave,
}) => {
  const field = limitFieldFor(code) ?? 'max_chunks';
  const label = field === 'max_calls' ? 'Call limit' : 'Source chunk limit';

  const [value, setValue] = useState<number | null>(null);
  const [current, setCurrent] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  // The suggestion doubles the current limit, which is the smallest change that
  // is likely to get past the same source rather than hit the wall again.
  useEffect(() => {
    let cancelled = false;
    APIClient.getSettings()
      .then((res) => {
        if (cancelled) return;
        const configured = res.config.generation?.[field] ?? defaultsFor[field];
        setCurrent(configured);
        setValue(Math.max(configured * 2, configured + 10));
      })
      .catch(() => {
        if (!cancelled) setValue(defaultsFor[field] * 2);
      });
    return () => {
      cancelled = true;
    };
  }, [field]);

  const limits = (): GenerationLimitsOverride | null => {
    if (value === null || Number.isNaN(value) || value <= 0) {
      setError('Enter a limit greater than zero.');
      return null;
    }
    return field === 'max_calls' ? { max_calls: value } : { max_chunks: value };
  };

  return (
    <div className="p-4 rounded-xl bg-amber-500/10 border border-amber-500/25 space-y-3">
      <div className="flex items-start gap-2.5">
        <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
        <div className="space-y-1 min-w-0">
          <p className="text-xs text-amber-200">{message}</p>
          <p className="text-[11px] text-amber-200/70">{generationLimitHint}</p>
        </div>
      </div>

      {error && <p className="text-[11px] text-red-300">{error}</p>}

      <div className="flex flex-wrap items-end gap-2">
        <label className="space-y-1 text-[11px] font-medium text-amber-200/80">
          {label}
          <span className="block text-[10px] text-amber-200/50">
            {current === null ? 'reading the current value…' : `currently ${current}`}
          </span>
          <input
            type="number"
            min={1}
            aria-label={label}
            value={value ?? ''}
            onChange={(e) => {
              setError(null);
              setValue(e.target.value === '' ? null : Number(e.target.value));
            }}
            className="w-32 px-3 py-2 text-sm text-white bg-neutral-950/60 border border-amber-500/30 rounded-xl focus:border-amber-400 focus:outline-none font-mono"
          />
        </label>
        <button
          type="button"
          disabled={busy || value === null}
          onClick={() => {
            const raised = limits();
            if (raised) onRetry(raised);
          }}
          className="flex items-center gap-1.5 px-3 py-2 text-xs font-medium text-neutral-900 rounded-xl bg-amber-400 hover:bg-amber-300 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          <Gauge className="w-3.5 h-3.5" />
          Use for this run
        </button>
        <button
          type="button"
          disabled={busy || value === null}
          onClick={() => {
            const raised = limits();
            if (raised) onSave(raised);
          }}
          className="px-3 py-2 text-xs text-amber-100 rounded-xl border border-amber-500/40 hover:bg-amber-500/10 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >
          Save and use
        </button>
      </div>
    </div>
  );
};

export default GenerationLimitNotice;
