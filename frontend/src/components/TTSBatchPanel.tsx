import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, Download, Layers, Loader2, RefreshCw, Trash2, XCircle } from 'lucide-react';
import { APIClient } from '../api/client';
import type { GameSummary, TTSBatchJob } from '../types';

// A job in one of these phases is still in flight (or was left needing a
// download), so the manager keeps polling and the start button stays disabled.
const ACTIVE_STATUSES = new Set([
  'queued',
  'submitted',
  'pending',
  'processing',
  'running',
  'downloading',
  'storing',
  'succeeded',
]);

// PHASE_LABELS turns a job's stored status into what it actually means: a job the
// provider finished is still downloading, not done, until its clips are cached.
const PHASE_LABELS: Record<string, string> = {
  queued: 'Queued',
  submitted: 'Queued',
  pending: 'Queued',
  processing: 'Processing',
  running: 'Processing',
  downloading: 'Downloading',
  storing: 'Saving to cache',
  completed: 'Completed',
  succeeded: 'Awaiting download',
  failed: 'Failed',
  cancelled: 'Cancelled',
  expired: 'Expired',
};

const phaseLabel = (status: string): string => PHASE_LABELS[status] ?? status;

const statusColor = (status: string): string => {
  switch (status) {
    case 'completed':
      return 'text-emerald-400';
    case 'failed':
      return 'text-rose-400';
    case 'cancelled':
    case 'expired':
      return 'text-stone-500';
    case 'downloading':
    case 'storing':
      return 'text-sky-400';
    default:
      return 'text-purple-400';
  }
};

// messageOf turns a thrown value into the reason the server gave, trimmed, so a
// provider error or a network fault is shown rather than a bare status.
const messageOf = (err: unknown): string =>
  (err instanceof Error ? err.message : String(err)).trim() || 'the request failed with no detail';

// jobIncomplete reports whether a job is short of its request count, so its
// clips still need downloading and storing.
const jobIncomplete = (job: TTSBatchJob): boolean =>
  job.request_count > 0 &&
  job.completed + (job.failed_keys?.length ?? 0) < job.request_count &&
  job.status !== 'failed' &&
  job.status !== 'cancelled' &&
  job.status !== 'expired';

// canResume reports whether a job is worth finishing by hand: it is incomplete and
// the provider is not already working on it.
const canResume = (job: TTSBatchJob): boolean =>
  jobIncomplete(job) && job.status !== 'processing' && job.status !== 'running';

// TTSBatchPanel is the global manager for offline batch speech jobs. It lists
// every campaign's jobs, filters them by campaign, starts a backfill, cancels a
// running one, and expands a job to inspect it.
export const TTSBatchPanel: React.FC = () => {
  const [jobs, setJobs] = useState<TTSBatchJob[]>([]);
  const [games, setGames] = useState<GameSummary[]>([]);
  const [filter, setFilter] = useState('');
  const [startGame, setStartGame] = useState('');
  const [force, setForce] = useState(false);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [nextJobs, nextGames] = await Promise.all([
        APIClient.listAllTTSBatchJobs(),
        APIClient.listGames(),
      ]);
      setJobs(nextJobs);
      setGames(nextGames);
    } catch (err) {
      setError(messageOf(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Poll while any job is in flight, so a completion (including one resumed at
  // launch) shows without a manual refresh.
  const hasActive = useMemo(() => jobs.some((job) => ACTIVE_STATUSES.has(job.status)), [jobs]);
  useEffect(() => {
    if (!hasActive) return;
    const timer = window.setInterval(() => {
      void refresh();
    }, 15000);
    return () => window.clearInterval(timer);
  }, [hasActive, refresh]);

  const visible = useMemo(
    () => (filter ? jobs.filter((job) => job.game_id === filter) : jobs),
    [jobs, filter]
  );

  // A backfill already in flight for the chosen campaign makes the start button
  // a no-op, so it is disabled rather than silently returning the same job.
  const startBlocked = useMemo(
    () => jobs.some((job) => job.game_id === startGame && ACTIVE_STATUSES.has(job.status)),
    [jobs, startGame]
  );

  const start = async () => {
    if (!startGame) return;
    setError(null);
    try {
      const job = await APIClient.startTTSBatch(startGame, force);
      if (!job) {
        setError('Every clip is already cached for that campaign.');
      }
      await refresh();
    } catch (err) {
      setError(messageOf(err));
    }
  };

  const cancel = async (job: TTSBatchJob) => {
    setError(null);
    try {
      await APIClient.cancelTTSBatchJob(job.game_id, job.id);
      await refresh();
    } catch (err) {
      setError(messageOf(err));
    }
  };

  const remove = async (job: TTSBatchJob) => {
    setError(null);
    try {
      await APIClient.deleteTTSBatchJob(job.game_id, job.id);
      await refresh();
    } catch (err) {
      setError(messageOf(err));
    }
  };

  const resume = async (job: TTSBatchJob) => {
    setError(null);
    try {
      await APIClient.resumeTTSBatchJob(job.game_id, job.id);
      await refresh();
    } catch (err) {
      setError(messageOf(err));
    }
  };

  const clearFinished = async () => {
    setError(null);
    try {
      const removed = await APIClient.clearTTSBatchJobs(filter || undefined);
      if (removed === 0) {
        setError('No finished jobs to clear.');
      }
      await refresh();
    } catch (err) {
      setError(messageOf(err));
    }
  };

  return (
    <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
          <Layers className="w-4 h-4" />
          <span>Batch Speech Backfill</span>
        </h3>
        <div className="flex flex-wrap items-center gap-2">
          <select
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="bg-stone-900 border border-stone-700 text-stone-200 rounded-lg pl-2.5 pr-7 py-1 text-xs focus:outline-none cursor-pointer"
          >
            <option value="">All campaigns</option>
            {games.map((game) => (
              <option key={game.id} value={game.id}>
                {game.name}
              </option>
            ))}
          </select>
          <select
            value={startGame}
            onChange={(e) => setStartGame(e.target.value)}
            className="bg-stone-900 border border-purple-500/30 text-purple-300 rounded-lg pl-2.5 pr-7 py-1 text-xs focus:outline-none cursor-pointer"
          >
            <option value="">Backfill a campaign...</option>
            {games.map((game) => (
              <option key={game.id} value={game.id}>
                {game.name}
              </option>
            ))}
          </select>
          <label
            className="flex items-center gap-1.5 text-xs text-stone-400 cursor-pointer"
            title="Re-render every clip, overwriting the cache, instead of only the missing ones"
          >
            <input
              type="checkbox"
              checked={force}
              onChange={(e) => setForce(e.target.checked)}
              className="accent-purple-500 cursor-pointer"
            />
            Full regenerate
          </label>
          <button
            type="button"
            onClick={() => void start()}
            disabled={!startGame || startBlocked}
            className="px-2.5 py-1 rounded-lg text-xs font-bold bg-purple-600 text-white hover:bg-purple-500 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
          >
            {startBlocked ? 'Running' : 'Start'}
          </button>
          <button
            type="button"
            onClick={() => void refresh()}
            className="p-1.5 rounded-lg text-stone-300 hover:text-purple-300 cursor-pointer"
            title="Refresh jobs"
            aria-label="Refresh jobs"
          >
            {loading ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <RefreshCw className="w-3.5 h-3.5" />}
          </button>
          <button
            type="button"
            onClick={() => void clearFinished()}
            className="px-2.5 py-1 rounded-lg text-xs font-semibold border border-stone-700 text-stone-300 hover:text-rose-300 hover:border-rose-500/40 cursor-pointer"
            title={filter ? 'Remove finished jobs for the filtered campaign' : 'Remove every finished job'}
          >
            Clear finished
          </button>
        </div>
      </div>

      <p className="text-xs text-stone-400">
        Offline jobs render a campaign's missing speech through the provider's batch API, at a discount and
        without spending interactive rate-limit quota. A job keeps running while the app is closed and is
        collected on the next launch.
      </p>

      {error && (
        <div className="rounded-lg border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-xs text-rose-200 whitespace-pre-wrap break-words">
          {error}
        </div>
      )}

      {visible.length === 0 ? (
        <div className="text-xs text-stone-500">No batch jobs.</div>
      ) : (
        <div className="space-y-1">
          {visible.map((job) => {
            const key = `${job.game_id}:${job.id}`;
            const isOpen = expanded === key;
            return (
              <div key={key} className="rounded-lg border border-stone-800 bg-stone-950/40 text-xs">
                <div className="flex items-center justify-between gap-3 px-3 py-2">
                  <button
                    type="button"
                    onClick={() => setExpanded(isOpen ? null : key)}
                    className="flex min-w-0 items-center gap-2 text-left cursor-pointer"
                    aria-expanded={isOpen}
                    title="Show job details"
                  >
                    {isOpen ? (
                      <ChevronDown className="w-3.5 h-3.5 shrink-0 text-stone-500" />
                    ) : (
                      <ChevronRight className="w-3.5 h-3.5 shrink-0 text-stone-500" />
                    )}
                    <span className="min-w-0">
                      <span className="block truncate text-stone-200">{job.game_name || job.game_id}</span>
                      <span className="block truncate font-mono text-stone-500">{job.provider}</span>
                    </span>
                  </button>
                  <div className="flex shrink-0 items-center gap-3">
                    <span className={`font-bold uppercase tracking-wide ${statusColor(job.status)}`}>
                      {phaseLabel(job.status)}
                    </span>
                    <span className="text-stone-400" title="clips stored / groups submitted">
                      {job.completed}/{job.request_count}
                    </span>
                    {job.failed_keys && job.failed_keys.length > 0 && (
                      <span className="text-amber-300" title={job.failed_keys.join(', ')}>
                        {job.failed_keys.length} failed
                      </span>
                    )}
                    {canResume(job) && (
                      <button
                        type="button"
                        onClick={() => void resume(job)}
                        className="p-1 rounded text-sky-400 hover:bg-sky-500/20 cursor-pointer"
                        title="Download and store this job's clips now"
                        aria-label="Download and store this job's clips now"
                      >
                        <Download className="w-3.5 h-3.5" />
                      </button>
                    )}
                    {ACTIVE_STATUSES.has(job.status) ? (
                      <button
                        type="button"
                        onClick={() => void cancel(job)}
                        className="p-1 rounded text-rose-400 hover:bg-rose-500/20 cursor-pointer"
                        title="Cancel this job"
                        aria-label="Cancel this job"
                      >
                        <XCircle className="w-3.5 h-3.5" />
                      </button>
                    ) : (
                      <button
                        type="button"
                        onClick={() => void remove(job)}
                        className="p-1 rounded text-stone-400 hover:text-rose-300 hover:bg-rose-500/20 cursor-pointer"
                        title="Remove this finished job"
                        aria-label="Remove this finished job"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    )}
                  </div>
                </div>

                {isOpen && (
                  <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 border-t border-stone-800 px-3 py-2 text-stone-400">
                    <dt>Campaign</dt>
                    <dd className="text-stone-200">{job.game_name || job.game_id}</dd>
                    <dt>Provider</dt>
                    <dd className="font-mono text-stone-200">{job.provider}</dd>
                    <dt>Model</dt>
                    <dd className="font-mono text-stone-200">{job.model || 'provider default'}</dd>
                    <dt>Stage</dt>
                    <dd className={statusColor(job.status)}>{phaseLabel(job.status)}</dd>
                    <dt>Groups</dt>
                    <dd className="text-stone-200">{job.request_count} submitted</dd>
                    <dt>Stored</dt>
                    <dd className="text-stone-200">{job.completed} clips cached</dd>
                    <dt>Failed</dt>
                    <dd className="text-stone-200">
                      {job.failed_keys && job.failed_keys.length > 0 ? (
                        <span className="font-mono break-all text-amber-300">{job.failed_keys.join(', ')}</span>
                      ) : (
                        'none'
                      )}
                    </dd>
                    <dt>Job id</dt>
                    <dd className="font-mono break-all text-stone-200">{job.id}</dd>
                  </dl>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
