import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Layers, Loader2, RefreshCw, XCircle } from 'lucide-react';
import { APIClient } from '../api/client';
import type { GameSummary, TTSBatchJob } from '../types';

// A job that is still running can be cancelled.
const ACTIVE_STATUSES = new Set(['submitted', 'pending', 'running']);

// messageOf turns a thrown value into the reason the server gave, trimmed, so a
// provider error or a network fault is shown rather than a bare status.
const messageOf = (err: unknown): string =>
  (err instanceof Error ? err.message : String(err)).trim() || 'the request failed with no detail';

// statusColor maps a job status to a text colour.
const statusColor = (status: string): string => {
  switch (status) {
    case 'succeeded':
      return 'text-emerald-400';
    case 'failed':
      return 'text-rose-400';
    case 'cancelled':
    case 'expired':
      return 'text-stone-500';
    default:
      return 'text-purple-400';
  }
};

// TTSBatchPanel is the global manager for offline batch speech jobs. It lists
// every campaign's jobs, filters them by campaign, starts a backfill, and
// cancels a running one.
export const TTSBatchPanel: React.FC = () => {
  const [jobs, setJobs] = useState<TTSBatchJob[]>([]);
  const [games, setGames] = useState<GameSummary[]>([]);
  const [filter, setFilter] = useState('');
  const [startGame, setStartGame] = useState('');
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

  const start = async () => {
    if (!startGame) return;
    setError(null);
    try {
      const job = await APIClient.startTTSBatch(startGame);
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
          <button
            type="button"
            onClick={() => void start()}
            disabled={!startGame}
            className="px-2.5 py-1 rounded-lg text-xs font-bold bg-purple-600 text-white hover:bg-purple-500 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
          >
            Start
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
        </div>
      </div>

      <p className="text-xs text-stone-400">
        Offline jobs render a campaign's missing speech through the provider's batch API, at a discount and
        without spending interactive rate-limit quota.
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
          {visible.map((job) => (
            <div
              key={`${job.game_id}:${job.id}`}
              className="flex items-center justify-between gap-3 rounded-lg border border-stone-800 bg-stone-950/40 px-3 py-2 text-xs"
            >
              <div className="min-w-0">
                <div className="text-stone-200 truncate">{job.game_name || job.game_id}</div>
                <div className="text-stone-500 font-mono truncate">
                  {job.provider} · {job.id}
                </div>
              </div>
              <div className="flex items-center gap-3 shrink-0">
                <span className={`font-bold uppercase tracking-wide ${statusColor(job.status)}`}>{job.status}</span>
                <span className="text-stone-400">
                  {job.completed}/{job.request_count}
                </span>
                {job.failed_keys && job.failed_keys.length > 0 && (
                  <span className="text-amber-300" title={job.failed_keys.join(', ')}>
                    {job.failed_keys.length} failed
                  </span>
                )}
                {ACTIVE_STATUSES.has(job.status) && (
                  <button
                    type="button"
                    onClick={() => void cancel(job)}
                    className="p-1 rounded text-rose-400 hover:bg-rose-500/20 cursor-pointer"
                    title="Cancel this job"
                    aria-label="Cancel this job"
                  >
                    <XCircle className="w-3.5 h-3.5" />
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
