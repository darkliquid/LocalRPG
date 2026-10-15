import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { AlertTriangle, X } from 'lucide-react';

// One thing that went wrong, in a sentence the player can read. The detail is
// what a developer wants and is shown behind a disclosure, so a stack trace
// never leads the message.
export interface Failure {
  id: number;
  text: string;
  detail?: string;
}

export interface FailureReport {
  text: string;
  detail?: string;
}

interface FailuresValue {
  failures: Failure[];
  report: (failure: FailureReport) => void;
  dismiss: (id: number) => void;
}

const FailuresContext = createContext<FailuresValue | null>(null);

let nextFailureId = 1;

// FailuresProvider owns the app's failures and renders them once, so a failure
// raised while a modal is closed still has somewhere to land. A component whose
// failure belongs to its own form keeps its own error state instead.
export function FailuresProvider({ children }: { children: ReactNode }) {
  const [failures, setFailures] = useState<Failure[]>([]);

  const report = useCallback((failure: FailureReport) => {
    setFailures((prev) => {
      // Reporting the same sentence again replaces it rather than stacking, so a
      // retry loop cannot fill the region with the same line.
      const last = prev[prev.length - 1];
      if (last && last.text === failure.text) {
        return [...prev.slice(0, -1), { ...last, detail: failure.detail }];
      }
      return [...prev, { id: nextFailureId++, text: failure.text, detail: failure.detail }];
    });
  }, []);

  const dismiss = useCallback((id: number) => {
    setFailures((prev) => prev.filter((failure) => failure.id !== id));
  }, []);

  const value = useMemo(() => ({ failures, report, dismiss }), [failures, report, dismiss]);
  return <FailuresContext.Provider value={value}>{children}</FailuresContext.Provider>;
}

// useFailures returns the app's failure reporter. Outside a provider it keeps the
// developer's console line and reports nothing, so a component rendered on its
// own in a test does not have to be wrapped and does not throw.
export function useFailures(): FailuresValue {
  const value = useContext(FailuresContext);
  if (value) return value;

  return noProviderFailures;
}

const noProviderFailures: FailuresValue = {
  failures: [],
  report: ({ text, detail }) => {
    console.error(`[failure] ${text}`, detail ?? '');
  },
  dismiss: () => undefined,
};

// FailuresRegion is the one place a failure is shown. The newest sentence is
// rendered in full and older ones are counted, because a region that lists every
// failure at once is unreadable by the time it matters.
export function FailuresRegion() {
  const { failures, dismiss } = useFailures();
  if (failures.length === 0) return null;

  const newest = failures[failures.length - 1];
  const earlier = failures.length - 1;

  return (
    <div
      role="status"
      aria-live="polite"
      className="mx-4 my-2 flex items-start gap-2 rounded-lg border border-amber-700/50 bg-amber-950/40 px-3 py-2 text-sm font-sans text-amber-200"
    >
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" aria-hidden="true" />
      <div className="flex-1">
        <p>{newest.text}</p>
        {newest.detail && (
          <details className="mt-1">
            <summary className="cursor-pointer text-xs text-amber-300/80">Details</summary>
            <pre className="mt-1 overflow-x-auto whitespace-pre-wrap text-xs text-amber-300/80">{newest.detail}</pre>
          </details>
        )}
        {earlier > 0 && (
          <p className="mt-1 text-xs text-amber-300/80">
            {earlier} earlier {earlier === 1 ? 'failure' : 'failures'}
          </p>
        )}
      </div>
      <button
        type="button"
        onClick={() => dismiss(newest.id)}
        className="cursor-pointer text-amber-300/80 hover:text-amber-100"
        aria-label="Dismiss failure"
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  );
}
