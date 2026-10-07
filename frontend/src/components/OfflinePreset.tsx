import React, { useState } from 'react';
import { ShieldCheck, AlertCircle, CheckCircle, WifiOff, Loader2, X } from 'lucide-react';
import { APIClient } from '../api/client';
import { AppConfig, OfflineIssue, OfflineReportResponse } from '../types';
import { TierBadge } from './providers/TierBadge';

export interface OfflinePresetProps {
  onChange: (config: AppConfig) => void;
}

export const OfflinePreset: React.FC<OfflinePresetProps> = ({ onChange }) => {
  const [showConfirm, setShowConfirm] = useState(false);
  const [ttsChoice, setTtsChoice] = useState<'native-os' | 'sherpa-onnx'>('native-os');
  const [isApplying, setIsApplying] = useState(false);
  const [applyResult, setApplyResult] = useState<string[] | null>(null);
  const [applyError, setApplyError] = useState<string | null>(null);

  const [isChecking, setIsChecking] = useState(false);
  const [report, setReport] = useState<OfflineReportResponse | null>(null);
  const [checkError, setCheckError] = useState<string | null>(null);

  const handleApply = async () => {
    setIsApplying(true);
    setApplyError(null);
    setApplyResult(null);
    try {
      const res = await APIClient.applyOfflinePreset(ttsChoice);
      setApplyResult(res.changes);
      setShowConfirm(false);
      const settings = await APIClient.getSettings();
      onChange(settings.config);
    } catch (err: unknown) {
      setApplyError(err instanceof Error ? err.message : 'Failed to apply preset');
    } finally {
      setIsApplying(false);
    }
  };

  const handleCheck = async () => {
    setIsChecking(true);
    setCheckError(null);
    setReport(null);
    try {
      const res = await APIClient.checkOffline();
      setReport(res);
    } catch (err: unknown) {
      setCheckError(err instanceof Error ? err.message : 'Failed to check offline configuration');
    } finally {
      setIsChecking(false);
    }
  };

  return (
    <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="font-sans text-sm font-bold text-stone-100 flex items-center gap-2">
            <WifiOff className="w-4 h-4 text-emerald-400" />
            <span>Offline Operations</span>
          </h3>
          <p className="text-xs text-stone-400 mt-0.5">
            Configure or verify a fully local, zero-network stack across language models, speech, and imagery.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-stone-800 hover:bg-stone-700 text-stone-200 transition-colors flex items-center gap-1.5"
            onClick={handleCheck}
            disabled={isChecking}
          >
            {isChecking ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />}
            <span>Check offline</span>
          </button>
          <button
            type="button"
            className="px-3 py-1.5 rounded-lg text-xs font-medium bg-emerald-600/20 hover:bg-emerald-600/30 text-emerald-300 border border-emerald-500/30 transition-colors flex items-center gap-1.5"
            onClick={() => {
              setShowConfirm(true);
              setApplyResult(null);
              setApplyError(null);
            }}
          >
            <WifiOff className="w-3.5 h-3.5" />
            <span>Offline preset</span>
          </button>
        </div>
      </div>

      {applyResult && (
        <div className="p-3 rounded-lg bg-emerald-950/40 border border-emerald-800/60 text-xs text-emerald-200 space-y-1">
          <div className="flex items-center justify-between font-semibold">
            <span className="flex items-center gap-1.5">
              <CheckCircle className="w-4 h-4 text-emerald-400" />
              Applied offline preset
            </span>
            <button
              type="button"
              className="text-stone-400 hover:text-stone-200"
              onClick={() => setApplyResult(null)}
              aria-label="Dismiss changes"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
          {applyResult.length === 0 ? (
            <p className="text-emerald-300/80">No changes needed; configuration was already offline.</p>
          ) : (
            <ul className="list-disc list-inside space-y-0.5 text-stone-300">
              {applyResult.map((change, idx) => (
                <li key={idx}>{change}</li>
              ))}
            </ul>
          )}
        </div>
      )}

      {applyError && (
        <div className="p-3 rounded-lg bg-red-950/40 border border-red-800/60 text-xs text-red-200 flex items-center justify-between">
          <span className="flex items-center gap-1.5">
            <AlertCircle className="w-4 h-4 text-red-400" />
            {applyError}
          </span>
          <button type="button" onClick={() => setApplyError(null)}>
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}

      {report && (
        <div
          className={`p-3 rounded-lg border text-xs space-y-2 ${
            report.offline
              ? 'bg-emerald-950/40 border-emerald-800/60 text-emerald-200'
              : 'bg-amber-950/40 border-amber-800/60 text-amber-200'
          }`}
        >
          <div className="flex items-center justify-between font-semibold">
            <span className="flex items-center gap-1.5">
              {report.offline ? (
                <>
                  <CheckCircle className="w-4 h-4 text-emerald-400" />
                  All configured providers are offline
                </>
              ) : (
                <>
                  <AlertCircle className="w-4 h-4 text-amber-400" />
                  Provider configuration is not fully offline
                </>
              )}
            </span>
            <button
              type="button"
              className="text-stone-400 hover:text-stone-200"
              onClick={() => setReport(null)}
              aria-label="Dismiss report"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>

          {!report.offline && report.issues && report.issues.length > 0 && (
            <div className="space-y-1.5 pt-1">
              <p className="text-xs text-amber-300/90">
                The following providers reach external services or run as local servers:
              </p>
              <div className="space-y-1">
                {report.issues.map((issue: OfflineIssue, idx: number) => (
                  <div
                    key={idx}
                    className="flex flex-wrap items-center justify-between gap-2 p-2 rounded bg-stone-900/60 border border-amber-900/40 text-stone-200"
                  >
                    <div className="flex items-center gap-2">
                      <span className="font-mono text-stone-400 uppercase text-[10px]">[{issue.role}]</span>
                      <span className="font-mono text-stone-100">{issue.provider_key}</span>
                      <TierBadge tier={issue.tier} />
                    </div>
                    <span className="text-stone-400 text-[11px]">{issue.reason}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {checkError && (
        <div className="p-3 rounded-lg bg-red-950/40 border border-red-800/60 text-xs text-red-200 flex items-center justify-between">
          <span className="flex items-center gap-1.5">
            <AlertCircle className="w-4 h-4 text-red-400" />
            {checkError}
          </span>
          <button type="button" onClick={() => setCheckError(null)}>
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}

      {showConfirm && (
        <div className="p-4 rounded-xl bg-stone-900 border border-stone-700 space-y-4">
          <div>
            <h4 className="font-sans text-sm font-bold text-stone-100 flex items-center gap-2">
              <WifiOff className="w-4 h-4 text-emerald-400" />
              <span>Confirm Offline Preset</span>
            </h4>
            <p className="text-xs text-stone-400 mt-1">
              Applying the offline preset will configure standard built-in providers that operate completely offline.
              Your file paths, UI preferences, and named provider entries will not be modified.
            </p>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs">
            <div className="p-2.5 rounded-lg bg-stone-950/60 border border-stone-800">
              <span className="text-stone-400 block text-[11px] uppercase tracking-wider font-semibold">Game Master</span>
              <span className="text-stone-100 font-medium">narrative-oracle</span>
              <span className="text-stone-500 block text-[11px]">Builtin schema-agnostic dice & prompt oracle</span>
            </div>
            <div className="p-2.5 rounded-lg bg-stone-950/60 border border-stone-800">
              <span className="text-stone-400 block text-[11px] uppercase tracking-wider font-semibold">Auxiliary Roles</span>
              <span className="text-stone-100 font-medium">Inherit from GM / Disabled</span>
              <span className="text-stone-500 block text-[11px]">Narrator disabled, extractor/completion inherit</span>
            </div>
            <div className="p-2.5 rounded-lg bg-stone-950/60 border border-stone-800">
              <span className="text-stone-400 block text-[11px] uppercase tracking-wider font-semibold">Image Generation</span>
              <span className="text-stone-100 font-medium">procedural-art</span>
              <span className="text-stone-500 block text-[11px]">Deterministic procedural svg canvas generation</span>
            </div>
            <div className="p-2.5 rounded-lg bg-stone-950/60 border border-stone-800">
              <span className="text-stone-400 block text-[11px] uppercase tracking-wider font-semibold">Embeddings</span>
              <span className="text-stone-100 font-medium">builtin-local (hash-projection)</span>
              <span className="text-stone-500 block text-[11px]">Zero-download vector projections</span>
            </div>
          </div>

          <div className="space-y-2 pt-1 border-t border-stone-800">
            <label className="block text-xs font-semibold text-stone-300">
              Text-to-Speech Provider Choice:
            </label>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
              <label
                className={`flex items-start gap-2.5 p-2.5 rounded-lg border cursor-pointer transition-colors ${
                  ttsChoice === 'native-os'
                    ? 'bg-emerald-950/30 border-emerald-500/50 text-stone-100'
                    : 'bg-stone-950/40 border-stone-800 text-stone-300 hover:border-stone-700'
                }`}
              >
                <input
                  type="radio"
                  name="offline-tts"
                  value="native-os"
                  checked={ttsChoice === 'native-os'}
                  onChange={() => setTtsChoice('native-os')}
                  className="mt-0.5 text-emerald-500 focus:ring-emerald-500"
                />
                <div>
                  <span className="block text-xs font-medium">native-os</span>
                  <span className="block text-[11px] text-stone-400">Uses OS built-in speech synthesis. Zero model downloads.</span>
                </div>
              </label>

              <label
                className={`flex items-start gap-2.5 p-2.5 rounded-lg border cursor-pointer transition-colors ${
                  ttsChoice === 'sherpa-onnx'
                    ? 'bg-emerald-950/30 border-emerald-500/50 text-stone-100'
                    : 'bg-stone-950/40 border-stone-800 text-stone-300 hover:border-stone-700'
                }`}
              >
                <input
                  type="radio"
                  name="offline-tts"
                  value="sherpa-onnx"
                  checked={ttsChoice === 'sherpa-onnx'}
                  onChange={() => setTtsChoice('sherpa-onnx')}
                  className="mt-0.5 text-emerald-500 focus:ring-emerald-500"
                />
                <div>
                  <span className="block text-xs font-medium">sherpa-onnx</span>
                  <span className="block text-[11px] text-stone-400">High quality local Kokoro TTS. Requires downloading weights once.</span>
                </div>
              </label>
            </div>
          </div>

          <div className="flex items-center justify-end gap-2 pt-2 border-t border-stone-800">
            <button
              type="button"
              className="px-3 py-1.5 rounded-lg text-xs text-stone-300 hover:text-stone-100 transition-colors"
              onClick={() => setShowConfirm(false)}
              disabled={isApplying}
            >
              Cancel
            </button>
            <button
              type="button"
              className="px-4 py-1.5 rounded-lg text-xs font-medium bg-emerald-600 hover:bg-emerald-500 text-white transition-colors flex items-center gap-1.5 shadow-sm"
              onClick={handleApply}
              disabled={isApplying}
            >
              {isApplying && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              <span>Apply preset</span>
            </button>
          </div>
        </div>
      )}
    </div>
  );
};
