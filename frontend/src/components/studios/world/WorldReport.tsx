import { AlertCircle, Check } from 'lucide-react';

export interface WorldReportProps {
  toast: { type: 'success' | 'error'; message: string } | null;
}

// WorldReport is the studio's own feedback: the toast a save or a generation left
// behind, which the world studio's actions do not otherwise report.
export function WorldReport({ toast }: WorldReportProps) {
  return (
    <>
      {toast && (
        <div
          className={`p-3 rounded-xl text-xs flex items-center gap-2 ${
            toast.type === 'success'
              ? 'bg-emerald-950/60 border border-emerald-500/40 text-emerald-200'
              : 'bg-red-950/60 border border-red-500/40 text-red-200'
          }`}
        >
          {toast.type === 'success' ? (
            <Check className="w-4 h-4 text-emerald-400" />
          ) : (
            <AlertCircle className="w-4 h-4 text-red-400" />
          )}
          <span>{toast.message}</span>
        </div>
      )}
    </>
  );
}
