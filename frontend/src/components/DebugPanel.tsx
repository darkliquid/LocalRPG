import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { APIClient } from '../api/client';
import { AppConfig, TraceEvent } from '../types';
import { Bug, RefreshCw, Trash2, Play, Pause } from 'lucide-react';

interface DebugPanelProps {
  config: AppConfig;
  setConfig: (config: AppConfig) => void;
}

const MIB = 1024 * 1024;

const levelLabel = (level: string): string => {
  if (level === 'summary') return 'Summary (decisions, sizes, timings)';
  if (level === 'full') return 'Full (prompts, replies, wire lines)';
  return 'Off';
};

// DebugPanel is a developer view, not a player view: it shows the prompts, raw
// provider bytes, and model metadata behind a turn. It lives under Settings so it
// sits next to the switch that turns it on.
export const DebugPanel: React.FC<DebugPanelProps> = ({ config, setConfig }) => {
  const [events, setEvents] = useState<TraceEvent[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [following, setFollowing] = useState(false);
  const [loading, setLoading] = useState(false);

  const traceLevel = config.preferences.trace_level ?? 'off';

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setEvents(await APIClient.traceEvents(300));
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Could not read the trace');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!following) return;
    const timer = setInterval(() => {
      void refresh();
    }, 1000);
    return () => clearInterval(timer);
  }, [following, refresh]);

  const clear = async () => {
    try {
      await APIClient.clearTrace();
      setEvents([]);
      setError(null);
    } catch (err: any) {
      setError(err.message || 'Could not clear the trace');
    }
  };

  const updateAgents = (patch: Partial<AppConfig['agents']>) => {
    setConfig({ ...config, agents: { ...config.agents, ...patch } });
  };

  // Newest first: the thing that just happened is what you opened this for.
  const ordered = useMemo(() => [...events].reverse(), [events]);
  const wireLines = useMemo(() => ordered.filter((event) => event.event === 'provider.wire').slice(0, 50), [ordered]);

  return (
    <div className="space-y-4 flex-1 overflow-y-auto pr-1">
      <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h3 className="font-sans text-sm font-bold text-purple-400 flex items-center gap-2">
            <Bug className="w-4 h-4" />
            <span>Trace &amp; Debug</span>
          </h3>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setFollowing((value) => !value)}
              className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg border transition-all cursor-pointer ${
                following
                  ? 'bg-purple-600/30 border-purple-500/50 text-purple-200'
                  : 'bg-stone-900 border-stone-700 text-stone-300 hover:text-purple-300'
              }`}
            >
              {following ? <Pause className="w-3.5 h-3.5" /> : <Play className="w-3.5 h-3.5" />}
              <span>{following ? 'Following' : 'Follow'}</span>
            </button>
            <button
              onClick={() => void refresh()}
              className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg bg-stone-900 border border-stone-700 text-stone-300 hover:text-purple-300 transition-all cursor-pointer"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
              <span>Refresh</span>
            </button>
            <button
              onClick={() => void clear()}
              className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg bg-stone-900 border border-stone-700 text-stone-300 hover:text-red-300 hover:border-red-500/40 transition-all cursor-pointer"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>Clear</span>
            </button>
          </div>
        </div>

        <p className="text-[11px] text-stone-500">
          Tracing is off by default. At full detail the file records prompts, replies, and raw provider lines, which
          are your own story and never leave this machine. The file is owner-only and rotates by size.
        </p>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300">Trace Level</label>
            <select
              value={traceLevel}
              onChange={(e) =>
                setConfig({
                  ...config,
                  preferences: { ...config.preferences, trace_level: e.target.value },
                })
              }
              className="w-full bg-stone-950 border border-stone-800 rounded-xl pl-3 pr-8 py-2 text-xs text-stone-100 focus:outline-none focus:border-purple-500/60 cursor-pointer"
            >
              <option value="off">{levelLabel('off')}</option>
              <option value="summary">{levelLabel('summary')}</option>
              <option value="full">{levelLabel('full')}</option>
            </select>
            <p className="text-[11px] text-stone-500">Save settings after changing this for it to take effect.</p>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
              <span>Payload Cap (characters)</span>
              <span className="font-mono text-purple-400">{config.agents.trace_payload_chars ?? 20000}</span>
            </label>
            <input
              type="number"
              min={500}
              step={500}
              value={config.agents.trace_payload_chars ?? 20000}
              onChange={(e) => {
                const parsed = parseInt(e.target.value, 10);
                updateAgents({ trace_payload_chars: Number.isNaN(parsed) ? 20000 : parsed });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <p className="text-[11px] text-stone-500">Any single recorded string is truncated past this.</p>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
              <span>Rotate At (MiB)</span>
              <span className="font-mono text-purple-400">
                {Math.round((config.agents.trace_max_bytes ?? 268435456) / MIB)}
              </span>
            </label>
            <input
              type="number"
              min={1}
              step={64}
              value={Math.round((config.agents.trace_max_bytes ?? 268435456) / MIB)}
              onChange={(e) => {
                const parsed = parseInt(e.target.value, 10);
                updateAgents({ trace_max_bytes: (Number.isNaN(parsed) ? 256 : parsed) * MIB });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <p className="text-[11px] text-stone-500">
              Generous by default: this guards a session left running, not normal play.
            </p>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
              <span>Rotated Files Kept</span>
              <span className="font-mono text-purple-400">{config.agents.trace_max_files ?? 3}</span>
            </label>
            <input
              type="number"
              min={1}
              max={20}
              value={config.agents.trace_max_files ?? 3}
              onChange={(e) => {
                const parsed = parseInt(e.target.value, 10);
                updateAgents({ trace_max_files: Number.isNaN(parsed) ? 3 : parsed });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <p className="text-[11px] text-stone-500">Older rotations are dropped once this many exist.</p>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
              <span>Rotate Check (events)</span>
              <span className="font-mono text-purple-400">{config.agents.trace_rotate_check ?? 200}</span>
            </label>
            <input
              type="number"
              min={1}
              step={50}
              value={config.agents.trace_rotate_check ?? 200}
              onChange={(e) => {
                const parsed = parseInt(e.target.value, 10);
                updateAgents({ trace_rotate_check: Number.isNaN(parsed) ? 200 : parsed });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <p className="text-[11px] text-stone-500">How often the file size is considered.</p>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-sans uppercase text-stone-300 flex items-center justify-between">
              <span>Wire Lines Per Call</span>
              <span className="font-mono text-purple-400">{config.agents.trace_chunk_limit ?? 500}</span>
            </label>
            <input
              type="number"
              min={0}
              step={50}
              value={config.agents.trace_chunk_limit ?? 500}
              onChange={(e) => {
                const parsed = parseInt(e.target.value, 10);
                updateAgents({ trace_chunk_limit: Number.isNaN(parsed) ? 500 : parsed });
              }}
              className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs font-mono text-stone-100 focus:outline-none focus:border-purple-500/60"
            />
            <p className="text-[11px] text-stone-500">Provider streams can be hundreds of lines; this bounds one call.</p>
          </div>
        </div>
      </div>

      <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-3">
        <div className="flex items-center justify-between">
          <h4 className="font-sans text-xs uppercase font-bold text-stone-200">Recorded Events</h4>
          <span className="text-[11px] font-mono text-stone-500">{events.length} events</span>
        </div>

        {error && <p className="text-xs text-red-400 font-mono">{error}</p>}

        {ordered.length === 0 && !error ? (
          <p className="text-stone-500 text-xs italic bg-black/30 p-3 rounded-xl border border-white/5">
            Nothing recorded. Set a trace level, save, and play a turn.
          </p>
        ) : (
          <div className="space-y-1 max-h-[28rem] overflow-y-auto pr-1">
            {ordered.map((event, index) => (
              <details key={index} className="bg-black/30 border border-white/5 rounded-lg px-2 py-1.5">
                <summary className="cursor-pointer text-xs font-mono text-stone-300 flex items-center gap-2">
                  <span className="text-stone-500">{event.ts}</span>
                  <span className="text-purple-300">{event.event}</span>
                  <span className="text-stone-500">{event.level}</span>
                </summary>
                <pre className="mt-1 text-[11px] text-stone-400 whitespace-pre-wrap break-all">
                  {JSON.stringify(event.fields ?? {}, null, 2)}
                </pre>
              </details>
            ))}
          </div>
        )}
      </div>

      {/* Raw wire lines bury everything else, so they live in their own panel. */}
      <details className="p-4 rounded-xl bg-glass-card border border-stone-800">
        <summary className="cursor-pointer font-sans text-xs uppercase font-bold text-stone-200 flex items-center gap-2">
          <span>Raw Provider Lines</span>
          <span className="text-[11px] font-mono text-stone-500">({wireLines.length} shown)</span>
        </summary>
        {wireLines.length === 0 ? (
          <p className="text-stone-500 text-xs italic mt-2">
            Only recorded at full detail. These are what the provider actually sent, before parsing.
          </p>
        ) : (
          <pre className="mt-2 text-[11px] text-stone-400 whitespace-pre-wrap break-all max-h-72 overflow-y-auto">
            {wireLines.map((event) => `${event.ts} ${String(event.fields?.line ?? '')}`).join('\n')}
          </pre>
        )}
      </details>
    </div>
  );
};
