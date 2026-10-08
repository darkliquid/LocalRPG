import React, { useCallback, useEffect, useState } from 'react';
import { Download, X, AlertCircle, Loader2, Film, Globe, Ban, FolderOpen } from 'lucide-react';
import { APIClient } from '../api/client';
import type { ExportCapabilities, ExportEvent, ExportJob } from '../types';

interface ExportModalProps {
  isOpen: boolean;
  gameID: string | null;
  onClose: () => void;
}

/**
 * ExportModal starts a story export and shows its progress. It mirrors the
 * model-download flow: the request returns as soon as the job is accepted and
 * progress arrives on the export event stream.
 */
// maxReportedLines bounds the log: a long campaign reports a lot, and the last few lines
// are the ones that matter.
const maxReportedLines = 8;

// formatBytes reports a byte count the way a person reads it.
const formatBytes = (size: number): string => {
  if (size >= 1 << 20) return `${(size / (1 << 20)).toFixed(1)} MB`;
  if (size >= 1 << 10) return `${Math.round(size / (1 << 10))} KB`;
  return `${size} B`;
};

// formatDuration reports a render's elapsed time compactly.
const formatDuration = (ms: number): string => {
  const total = Math.round(ms / 1000);
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
};

// hasFrameDetail reports whether an event carries the render's frame and audio
// totals, which the compile phase does not.
const hasFrameDetail = (phase: string): boolean => phase === 'frames' || phase === 'encode' || phase === 'done';

export const ExportModal: React.FC<ExportModalProps> = ({ isOpen, gameID, onClose }) => {
  const [format, setFormat] = useState<'web' | 'video'>('web');
  const [art, setArt] = useState(true);
  const [audio, setAudio] = useState(true);
  const [still, setStill] = useState(false);
  const [fps, setFps] = useState(15);
  const [size, setSize] = useState('1920x1080');
  const [outDir, setOutDir] = useState('');

  const [capabilities, setCapabilities] = useState<ExportCapabilities | null>(null);
  const [running, setRunning] = useState(false);
  const [job, setJob] = useState<ExportJob | null>(null);
  const [progress, setProgress] = useState<ExportEvent | null>(null);
  // What the export said as it went: coverage, beats it could not speak, clips it repaired.
  // The phase line only ever shows the latest event, so without this a diagnostic is
  // overwritten the moment the next phase starts.
  const [messages, setMessages] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    APIClient.exportCapabilities()
      .then((caps) => {
        setCapabilities(caps);
        setOutDir((current) => current || caps.default_dir || '');
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to check export support'));
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen || !gameID) return;
    const close = APIClient.subscribeExportEvents((event) => {
      if (event.game_id !== gameID) return;
      setProgress(event);
      if (event.message) {
        setMessages((prev) => (prev[prev.length - 1] === event.message ? prev : [...prev, event.message as string].slice(-maxReportedLines)));
      }
      if (event.phase === 'done') {
        setRunning(false);
        setJob((current) =>
          current
            ? { ...current, running: false, output_path: event.output_path ?? current.output_path }
            : null
        );
      } else if (event.phase === 'error' || event.phase === 'cancelled') {
        setRunning(false);
        if (event.phase === 'error') setError(event.error || 'Export failed');
      }
    });
    return close;
  }, [isOpen, gameID]);

  const handleStart = useCallback(async () => {
    if (!gameID || !outDir.trim()) return;
    setError(null);
    setProgress(null);
    setMessages([]);
    setJob(null);
    try {
      const started = await APIClient.startExport({
        game_id: gameID,
        format,
        out_dir: outDir.trim(),
        art,
        audio,
        still: format === 'video' ? still : undefined,
        fps: format === 'video' ? fps : undefined,
        size: format === 'video' ? size : undefined,
      });
      setJob(started);
      setRunning(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to start export');
    }
  }, [gameID, format, outDir, art, audio, still, fps, size]);

  const handleBrowse = useCallback(async () => {
    setError(null);
    try {
      const chosen = await APIClient.chooseDirectory('Choose an export destination');
      if (chosen) setOutDir(chosen);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to open the directory dialog');
    }
  }, []);

  const handleCancel = useCallback(async () => {
    if (!gameID) return;
    try {
      await APIClient.cancelExport(gameID);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to cancel export');
    }
  }, [gameID]);

  if (!isOpen) return null;

  const percent =
    progress && progress.total > 0 ? Math.min(100, Math.round((progress.done / progress.total) * 100)) : null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-6 bg-black/80 backdrop-blur-sm anim-fade-in">
      <div className="relative w-full max-w-xl bg-stone-900/95 border border-purple-500/30 rounded-2xl shadow-2xl flex flex-col overflow-hidden">
        <div className="flex items-center justify-between px-6 py-4 border-b border-stone-800">
          <div className="flex items-center gap-2">
            <Download className="w-5 h-5 text-purple-400" />
            <h2 className="font-sans text-lg font-bold text-purple-400">Export Story Replay</h2>
          </div>
          <button
            onClick={onClose}
            className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="p-6 space-y-4 text-xs font-sans text-stone-300">
          {!gameID && (
            <div className="p-3 rounded-xl bg-amber-950/50 border border-amber-500/40 text-amber-200">
              Open a campaign before exporting.
            </div>
          )}

          <div className="grid grid-cols-2 gap-2">
            <button
              onClick={() => setFormat('web')}
              disabled={running}
              className={`flex items-center justify-center gap-2 py-2 rounded-xl border transition-all cursor-pointer disabled:opacity-50 ${
                format === 'web'
                  ? 'bg-purple-600/80 border-purple-400 text-white font-bold'
                  : 'bg-glass-card border-stone-800 text-stone-300 hover:border-purple-500/40'
              }`}
            >
              <Globe className="w-4 h-4" />
              Web player
            </button>
            <button
              onClick={() => setFormat('video')}
              disabled={running}
              className={`flex items-center justify-center gap-2 py-2 rounded-xl border transition-all cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed ${
                format === 'video'
                  ? 'bg-purple-600/80 border-purple-400 text-white font-bold'
                  : 'bg-glass-card border-stone-800 text-stone-300 hover:border-purple-500/40'
              }`}
            >
              <Film className="w-4 h-4" />
              Video
            </button>
          </div>

          <div className="space-y-1">
            <label className="text-stone-400">Destination folder</label>
            <div className="flex gap-2">
              <input
                type="text"
                value={outDir}
                disabled={running}
                placeholder={capabilities?.default_dir || '/path/to/folder'}
                onChange={(e) => setOutDir(e.target.value)}
                className="flex-1 bg-stone-950 border border-stone-800 rounded-lg px-2 py-1.5 text-stone-200"
              />
              {capabilities?.native_dialog && (
                <button
                  onClick={handleBrowse}
                  disabled={running}
                  className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-stone-700 text-stone-200 hover:bg-stone-800 cursor-pointer disabled:opacity-50"
                >
                  <FolderOpen className="w-4 h-4" />
                  Browse…
                </button>
              )}
            </div>
          </div>

          <div className="flex flex-wrap gap-4">
            <label className="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" checked={art} disabled={running} onChange={(e) => setArt(e.target.checked)} />
              Include scene art
            </label>
            <label className="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" checked={audio} disabled={running} onChange={(e) => setAudio(e.target.checked)} />
              Include speech
            </label>
            {format === 'video' && (
              <label className="flex items-center gap-2 cursor-pointer">
                <input type="checkbox" checked={still} disabled={running} onChange={(e) => setStill(e.target.checked)} />
                Still frames (fast)
              </label>
            )}
          </div>

          {format === 'video' && (
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2">
                FPS
                <input
                  type="number"
                  min={1}
                  max={60}
                  value={fps}
                  disabled={running}
                  onChange={(e) => setFps(Number(e.target.value))}
                  className="w-16 bg-stone-950 border border-stone-800 rounded-lg px-2 py-1 text-stone-200"
                />
              </label>
              <label className="flex items-center gap-2">
                Size
                <input
                  type="text"
                  value={size}
                  disabled={running}
                  onChange={(e) => setSize(e.target.value)}
                  className="w-28 bg-stone-950 border border-stone-800 rounded-lg px-2 py-1 text-stone-200"
                />
              </label>
            </div>
          )}

          {messages.length > 0 && (
            <div className="max-h-28 overflow-y-auto rounded-lg bg-stone-950/70 border border-stone-800 px-3 py-2 space-y-0.5">
              {messages.map((message, index) => (
                <div key={index} className="text-xs font-mono text-stone-400">
                  {message}
                </div>
              ))}
            </div>
          )}

          {progress && running && (
            <div className="space-y-1">
              <div className="flex justify-between text-stone-400">
                <span>{progress.phase}{progress.message ? `: ${progress.message}` : ''}</span>
                {percent !== null && <span>{percent}%</span>}
              </div>
              <div className="h-2 rounded-full bg-stone-800 overflow-hidden">
                <div
                  className="h-full bg-purple-500 transition-all"
                  style={{ width: percent !== null ? `${percent}%` : '10%' }}
                />
              </div>
              {hasFrameDetail(progress.phase) && progress.total > 0 && (
                <div className="flex flex-wrap justify-between gap-x-3 text-xs font-mono text-stone-500">
                  <span>
                    {progress.frames}/{progress.total} frames ({progress.image_frames} drawn, {progress.repeat_frames} repeats)
                  </span>
                  {progress.total_audio_packets > 0 && (
                    <span>
                      {progress.audio_packets}/{progress.total_audio_packets} audio ({formatBytes(progress.audio_bytes)}/
                      {formatBytes(progress.total_audio_bytes)})
                    </span>
                  )}
                  {progress.elapsed_ms > 0 && <span>{formatDuration(progress.elapsed_ms)}</span>}
                </div>
              )}
            </div>
          )}

          {error && (
            <div className="p-3 rounded-xl bg-red-950/50 border border-red-500/40 text-red-200 flex items-center gap-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {job?.output_path && !running && (
            <div className="p-3 rounded-xl bg-emerald-950/40 border border-emerald-500/40 text-emerald-200">
              Exported to <code className="break-all">{job.output_path}</code>
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            {running ? (
              <button
                onClick={handleCancel}
                className="flex items-center gap-2 px-4 py-2 rounded-xl border border-red-500/40 text-red-200 hover:bg-red-950/40 transition-colors cursor-pointer"
              >
                <Ban className="w-4 h-4" />
                Cancel
              </button>
            ) : (
              <button
                onClick={handleStart}
                disabled={!gameID || !outDir.trim()}
                className="flex items-center gap-2 px-4 py-2 rounded-xl bg-purple-600 text-white font-bold hover:bg-purple-500 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {running ? <Loader2 className="w-4 h-4 animate-spin" /> : <Download className="w-4 h-4" />}
                Start export
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
