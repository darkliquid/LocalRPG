import React, { useState } from 'react';
import { AlertTriangle, Package, X } from 'lucide-react';
import { ContentManifestInfo } from '../types';

export interface ContentImportDialogProps {
  manifest?: ContentManifestInfo;
  onConfirm: (conflictMode: 'refuse' | 'rename' | 'overwrite') => void;
  onCancel: () => void;
  loading?: boolean;
  error?: string | null;
}

export const ContentImportDialog: React.FC<ContentImportDialogProps> = ({
  manifest,
  onConfirm,
  onCancel,
  loading = false,
  error = null,
}) => {
  const [conflictMode, setConflictMode] = useState<'refuse' | 'rename' | 'overwrite'>('refuse');

  if (!manifest) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm animate-fade-in">
      <div className="relative w-full max-w-lg overflow-hidden border border-white/10 rounded-2xl bg-neutral-900/90 shadow-2xl backdrop-blur-xl">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-white/10">
          <div className="flex items-center gap-3">
            <div className="p-2.5 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-400">
              <Package className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-white">Import Content Package</h3>
              <p className="text-xs text-neutral-400">Review package contents and trust settings</p>
            </div>
          </div>
          <button
            onClick={onCancel}
            disabled={loading}
            className="p-1.5 text-neutral-400 hover:text-white rounded-lg hover:bg-white/5 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 space-y-5">
          {error && (
            <div className="p-3 text-sm text-red-300 border border-red-500/30 rounded-xl bg-red-950/40">
              {error}
            </div>
          )}

          {/* Manifest Card */}
          <div className="p-4 rounded-xl bg-white/[0.03] border border-white/5 space-y-2.5 text-sm">
            <div className="flex justify-between items-start">
              <div>
                <h4 className="font-semibold text-white text-base">{manifest.name || manifest.id}</h4>
                <div className="flex items-center gap-2 mt-0.5 text-xs text-neutral-400">
                  <span className="font-mono bg-neutral-800 px-1.5 py-0.5 rounded text-neutral-300">
                    {manifest.id}
                  </span>
                  <span>•</span>
                  <span>v{manifest.version}</span>
                  {manifest.type && (
                    <>
                      <span>•</span>
                      <span className="capitalize">{manifest.type}</span>
                    </>
                  )}
                </div>
              </div>
              {manifest.file_count !== undefined && (
                <span className="text-xs text-neutral-400 bg-neutral-800/80 px-2 py-1 rounded-md border border-white/5">
                  {manifest.file_count} {manifest.file_count === 1 ? 'file' : 'files'}
                </span>
              )}
            </div>

            {manifest.description && (
              <p className="text-xs text-neutral-300 line-clamp-3 pt-1 border-t border-white/5">
                {manifest.description}
              </p>
            )}

            {(manifest.author || manifest.license) && (
              <div className="flex gap-4 pt-1 text-xs text-neutral-400">
                {manifest.author && <div>Author: <span className="text-neutral-300">{manifest.author}</span></div>}
                {manifest.license && <div>License: <span className="text-neutral-300">{manifest.license}</span></div>}
              </div>
            )}
          </div>

          {/* Script Warning */}
          {manifest.has_script && (
            <div className="flex items-start gap-3 p-4 rounded-xl bg-amber-500/10 border border-amber-500/25 text-amber-200">
              <AlertTriangle className="w-5 h-5 text-amber-400 shrink-0 mt-0.5" />
              <div className="text-xs space-y-1">
                <p className="font-semibold text-amber-300">Contains executable script hooks</p>
                <p className="text-amber-200/80 leading-relaxed">
                  This package includes custom mechanics.js script hooks which will be executed by the game engine.
                  Ensure you trust the author before proceeding.
                </p>
              </div>
            </div>
          )}

          {/* Conflict Mode Selection */}
          <div className="space-y-2">
            <label className="text-xs font-medium text-neutral-300">If content already exists:</label>
            <div className="grid grid-cols-3 gap-2">
              <label
                className={`flex flex-col p-3 rounded-xl border text-xs cursor-pointer transition-all ${
                  conflictMode === 'refuse'
                    ? 'border-purple-500/50 bg-purple-500/10 text-white'
                    : 'border-white/5 bg-white/[0.02] text-neutral-400 hover:border-white/10'
                }`}
              >
                <input
                  type="radio"
                  name="conflictMode"
                  value="refuse"
                  checked={conflictMode === 'refuse'}
                  onChange={() => setConflictMode('refuse')}
                  className="sr-only"
                />
                <span className="font-semibold text-white">Refuse</span>
                <span className="text-[10px] text-neutral-400 mt-0.5">Cancel on conflict</span>
              </label>

              <label
                className={`flex flex-col p-3 rounded-xl border text-xs cursor-pointer transition-all ${
                  conflictMode === 'rename'
                    ? 'border-purple-500/50 bg-purple-500/10 text-white'
                    : 'border-white/5 bg-white/[0.02] text-neutral-400 hover:border-white/10'
                }`}
              >
                <input
                  type="radio"
                  name="conflictMode"
                  value="rename"
                  checked={conflictMode === 'rename'}
                  onChange={() => setConflictMode('rename')}
                  className="sr-only"
                />
                <span className="font-semibold text-white">Rename</span>
                <span className="text-[10px] text-neutral-400 mt-0.5">Install as id-2</span>
              </label>

              <label
                className={`flex flex-col p-3 rounded-xl border text-xs cursor-pointer transition-all ${
                  conflictMode === 'overwrite'
                    ? 'border-purple-500/50 bg-purple-500/10 text-white'
                    : 'border-white/5 bg-white/[0.02] text-neutral-400 hover:border-white/10'
                }`}
              >
                <input
                  type="radio"
                  name="conflictMode"
                  value="overwrite"
                  checked={conflictMode === 'overwrite'}
                  onChange={() => setConflictMode('overwrite')}
                  className="sr-only"
                />
                <span className="font-semibold text-white">Overwrite</span>
                <span className="text-[10px] text-neutral-400 mt-0.5">Replace existing</span>
              </label>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 p-5 border-t border-white/10 bg-black/20">
          <button
            type="button"
            onClick={onCancel}
            disabled={loading}
            className="px-4 py-2 text-xs font-medium text-neutral-400 hover:text-white transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => onConfirm(conflictMode)}
            disabled={loading}
            className="px-5 py-2 text-xs font-semibold text-white rounded-xl bg-purple-600 hover:bg-purple-500 active:scale-[0.98] transition-all shadow-lg shadow-purple-600/20 disabled:opacity-50"
          >
            {loading ? 'Importing...' : 'Install Package'}
          </button>
        </div>
      </div>
    </div>
  );
};
