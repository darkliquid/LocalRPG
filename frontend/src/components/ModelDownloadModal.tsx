import React, { useEffect, useRef, useState } from 'react';
import { Volume2, Download, AlertCircle, CheckCircle2, X } from 'lucide-react';
import { APIClient } from '../api/client';
import { ModelStatus } from '../types';
import { useMountTransition } from '../hooks/useMountTransition';

interface ModelDownloadModalProps {
  modelId: string;
  modelName: string;
  sizeBytes: number;
  onClose: () => void;
  isOpen?: boolean;
}

export const ModelDownloadModal: React.FC<ModelDownloadModalProps> = ({
  modelId,
  modelName,
  sizeBytes,
  onClose,
  isOpen = true,
}) => {
  const [downloading, setDownloading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [downloadedBytes, setDownloadedBytes] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [completed, setCompleted] = useState(false);

  const lastRef = useRef({ id: modelId, name: modelName, size: sizeBytes });
  if (isOpen) lastRef.current = { id: modelId, name: modelName, size: sizeBytes };
  const active = lastRef.current;
  const formattedSize = (active.size / (1024 * 1024)).toFixed(1);
  const { mounted, state } = useMountTransition(isOpen, 200);

  useEffect(() => {
    if (!isOpen) return;
    const unsubscribe = APIClient.subscribeModelEvents((status: ModelStatus) => {
      if (status.id === modelId) {
        setDownloading(status.downloading);
        setProgress(status.progress);
        setDownloadedBytes(status.bytes_downloaded);
        if (status.error) {
          setError(status.error);
        }
        if (status.installed) {
          setCompleted(true);
        }
      }
    });
    return () => unsubscribe();
  }, [isOpen, modelId]);

  const handleStartDownload = async () => {
    try {
      setError(null);
      setDownloading(true);
      await APIClient.downloadModel(modelId);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
      setDownloading(false);
    }
  };

  if (!mounted) return null;

  return (
    <div
      data-state={state}
      className={`fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 ${
        state === 'enter' ? 'anim-fade-in' : 'anim-fade-out pointer-events-none'
      }`}
      style={{ '--anim-dur': '200ms' } as React.CSSProperties}
    >
      <div
        className={`w-full max-w-md bg-stone-900 border border-stone-800 rounded-xl shadow-2xl p-6 text-stone-200 ${
          state === 'enter' ? 'anim-scale-in' : 'anim-scale-out'
        }`}
      >
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-purple-500/10 text-purple-400 rounded-lg">
              <Volume2 className="w-6 h-6" />
            </div>
            <h3 className="text-lg font-semibold text-stone-100 font-sans">Enable Voice Narration</h3>
          </div>
          {!downloading && (
            <button
              onClick={onClose}
              className="text-stone-400 hover:text-stone-200 p-1 transition cursor-pointer"
            >
              <X className="w-5 h-5" />
            </button>
          )}
        </div>

        {completed ? (
          <div className="space-y-4">
            <div className="flex items-center gap-3 text-emerald-400 bg-emerald-950/30 border border-emerald-800/40 p-3 rounded-lg">
              <CheckCircle2 className="w-5 h-5 shrink-0" />
              <p className="text-sm">Voice pack installed! Future turns will narrate automatically.</p>
            </div>
            <button
              onClick={onClose}
              className="w-full py-2.5 bg-stone-800 hover:bg-stone-700 text-stone-200 font-medium rounded-lg transition cursor-pointer"
            >
              Done
            </button>
          </div>
        ) : (
          <div className="space-y-4">
            <p className="text-sm text-stone-400 leading-relaxed font-serif">
              Natural voice narration for characters and scene descriptions requires the{' '}
              <span className="text-stone-200 font-medium">{active.name}</span> (~{formattedSize} MB).
              Would you like to download it now?
            </p>

            {downloading && (
              <div className="space-y-2">
                <div className="flex justify-between text-xs text-stone-400 font-mono">
                  <span>Downloading...</span>
                  <span>{Math.round(progress * 100)}%</span>
                </div>
                <div className="w-full h-2 bg-stone-800 rounded-full overflow-hidden">
                  <div
                    className="h-full bg-purple-500 transition-all duration-200"
                    style={{ width: `${Math.round(progress * 100)}%` }}
                  />
                </div>
                <p className="text-xs text-stone-500 font-mono">
                  {(downloadedBytes / (1024 * 1024)).toFixed(1)} / {formattedSize} MB
                </p>
              </div>
            )}

            {error && (
              <div className="flex items-center gap-2 text-rose-400 text-xs bg-rose-950/30 p-2.5 rounded-lg border border-rose-800/40">
                <AlertCircle className="w-4 h-4 shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <div className="flex items-center gap-3 pt-2">
              <button
                disabled={downloading}
                onClick={onClose}
                className="flex-1 py-2 text-sm text-stone-400 hover:text-stone-200 transition disabled:opacity-50 cursor-pointer"
              >
                Continue in Silence
              </button>
              <button
                disabled={downloading}
                onClick={handleStartDownload}
                className="flex-1 py-2 px-4 bg-purple-600 hover:bg-purple-500 text-white font-semibold text-sm rounded-lg flex items-center justify-center gap-2 transition shadow-md disabled:opacity-50 cursor-pointer"
              >
                <Download className="w-4 h-4" />
                {downloading ? 'Downloading...' : `Download (${formattedSize} MB)`}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
