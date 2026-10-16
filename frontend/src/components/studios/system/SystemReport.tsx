import { AlertCircle, Check, Info } from 'lucide-react';
import { SystemTestFailure, SystemVerifyResult } from '../../../types';

export interface SystemReportProps {
  toast: { type: 'success' | 'error'; message: string } | null;
  verificationResult: SystemVerifyResult | null;
  testFailures: SystemTestFailure[];
  draftNotes: string[];
  setShowGenerateDialog: (open: boolean) => void;
}

// SystemReport is what the last action left behind: the toast, the verification
// result, the scenario failures, and the notes a generation wrote.
export function SystemReport({
  toast,
  verificationResult,
  testFailures,
  draftNotes,
  setShowGenerateDialog,
}: SystemReportProps) {
  return (
    <>
      {toast && (
        <div
          className={`p-3 rounded-xl text-xs flex items-center gap-2 shrink-0 ${
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

      {testFailures.length > 0 && (
        <div className="p-3 rounded-xl text-xs bg-red-950/40 border border-red-500/40 text-red-200 space-y-1 shrink-0">
          {testFailures.map((failure, index) => (
            <div key={index} className="font-mono">
              {failure.scenario} step {failure.step}: {failure.detail}
            </div>
          ))}
        </div>
      )}

      {verificationResult && (
        <div
          data-testid="verification-banner"
          className={`p-3 rounded-xl text-xs flex items-center justify-between gap-3 shrink-0 ${
            verificationResult.ok
              ? 'bg-emerald-950/40 border border-emerald-500/40 text-emerald-200'
              : 'bg-red-950/40 border border-red-500/40 text-red-200'
          }`}
        >
          <div className="flex items-start gap-2 min-w-0">
            {verificationResult.ok ? (
              <Check className="w-4 h-4 text-emerald-400 shrink-0 mt-0.5" />
            ) : (
              <AlertCircle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
            )}
            <div className="space-y-0.5 min-w-0">
              <div className="font-semibold">
                {verificationResult.ok ? 'System verification passed' : 'System verification reported failures'}
              </div>
              {verificationResult.failures && verificationResult.failures.length > 0 && (
                <ul className="text-[11px] text-red-300 font-mono list-disc list-inside space-y-0.5">
                  {verificationResult.failures.map((f, i) => (
                    <li key={i}>{f}</li>
                  ))}
                </ul>
              )}
            </div>
          </div>
          {!verificationResult.ok && (
            <button
              type="button"
              onClick={() => setShowGenerateDialog(true)}
              className="px-2.5 py-1 text-xs font-sans font-medium rounded-lg bg-red-500/20 hover:bg-red-500/30 text-red-200 border border-red-500/40 transition-colors cursor-pointer shrink-0"
            >
              Regenerate
            </button>
          )}
        </div>
      )}

      {draftNotes.length > 0 && (
        <div
          data-testid="draft-notes"
          className="p-3 rounded-xl text-xs flex items-start gap-2 bg-amber-950/40 border border-amber-500/40 text-amber-200 shrink-0"
        >
          <Info className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
          <div className="space-y-0.5 min-w-0">
            <div className="font-semibold">Generation notes</div>
            <ul className="text-[11px] list-disc list-inside space-y-0.5">
              {draftNotes.map((note, i) => (
                <li key={i}>{note}</li>
              ))}
            </ul>
          </div>
        </div>
      )}
    </>
  );
}
