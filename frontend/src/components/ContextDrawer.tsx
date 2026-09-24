import React, { useCallback, useEffect, useState } from 'react';
import { APIClient } from '../api/client';
import { GameSummary, Ref, TurnContext, WorkingEntry } from '../types';
import { Drawers } from './Drawers';
import {
  Layers,
  Cpu,
  Database,
  Hash,
  RefreshCw,
  CheckCircle2,
  XCircle,
  Link2,
  FileText,
  Zap,
  ChevronDown,
  ChevronRight,
} from 'lucide-react';

interface ContextDrawerProps {
  gameID?: string;
  isOpen?: boolean;
  onClose?: () => void;
  onSelectWikilink?: (target: string) => void;
}

export const ContextDrawer: React.FC<ContextDrawerProps> = ({
  gameID: initialGameID,
  isOpen,
  onClose,
  onSelectWikilink,
}) => {
  const [selectedGameID, setSelectedGameID] = useState<string>(initialGameID || '');
  const [games, setGames] = useState<GameSummary[]>([]);
  const [context, setContext] = useState<TurnContext | null>(null);
  const [workingSet, setWorkingSet] = useState<WorkingEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showPrompt, setShowPrompt] = useState(false);

  useEffect(() => {
    if (initialGameID) {
      setSelectedGameID(initialGameID);
    } else {
      APIClient.listGames()
        .then((list) => {
          setGames(list);
          if (list.length > 0 && !selectedGameID) {
            setSelectedGameID(list[0].id);
          }
        })
        .catch(() => {});
    }
  }, [initialGameID]);

  const loadData = useCallback(async () => {
    if (!selectedGameID) return;
    setLoading(true);
    setError(null);
    try {
      const [ctxData, wsData] = await Promise.all([
        APIClient.getTurnContext(selectedGameID).catch((e) => {
          console.warn('getTurnContext failed:', e);
          return null;
        }),
        APIClient.getWorkingSet(selectedGameID).catch((e) => {
          console.warn('getWorkingSet failed:', e);
          return [];
        }),
      ]);
      setContext(ctxData);
      setWorkingSet(wsData || []);
      if (!ctxData && (!wsData || wsData.length === 0)) {
        setError('No turn context or working set available yet for this campaign.');
      }
    } catch (err: any) {
      setError(err.message || 'Failed to load turn context');
    } finally {
      setLoading(false);
    }
  }, [selectedGameID]);

  useEffect(() => {
    if (selectedGameID) {
      void loadData();
    }
  }, [selectedGameID, loadData]);

  const handleRefClick = (ref: Ref) => {
    if (onSelectWikilink) {
      onSelectWikilink(ref.id);
    }
  };

  const strategyColor = (strat: string) => {
    switch (strat) {
      case 'server_session':
        return 'bg-emerald-950/80 text-emerald-300 border-emerald-700/50';
      case 'cached_prefix':
        return 'bg-cyan-950/80 text-cyan-300 border-cyan-700/50';
      case 'full_prompt':
      default:
        return 'bg-stone-900 text-stone-300 border-stone-700';
    }
  };

  const content = (
    <div className="space-y-6 text-stone-200">
      {/* Header controls & Game Selector if needed */}
      <div className="flex flex-wrap items-center justify-between gap-3 bg-glass-card p-3 rounded-xl border border-stone-800">
        <div className="flex items-center gap-2">
          {!initialGameID && games.length > 0 && (
            <select
              value={selectedGameID}
              onChange={(e) => setSelectedGameID(e.target.value)}
              className="bg-stone-950 border border-stone-700 rounded-lg px-2.5 py-1 text-xs text-stone-200 focus:outline-none focus:border-purple-500"
            >
              {games.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name}
                </option>
              ))}
            </select>
          )}
          <span className="text-xs font-mono text-stone-400">
            {selectedGameID ? `Campaign: ${selectedGameID}` : 'Select a campaign'}
          </span>
        </div>
        <button
          onClick={() => void loadData()}
          disabled={loading}
          className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-lg bg-stone-900 border border-stone-700 text-stone-300 hover:text-purple-300 transition-all cursor-pointer"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          <span>Refresh</span>
        </button>
      </div>

      {error && !context && (
        <div className="p-4 rounded-xl bg-stone-900/60 border border-stone-800 text-stone-400 text-xs italic">
          {error}
        </div>
      )}

      {context && (
        <>
          {/* Strategy & Telemetry Card */}
          <div className="p-4 rounded-xl bg-glass-card border border-stone-800 space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <span className="text-xs uppercase font-sans font-bold tracking-wider text-stone-400">
                  Turn {context.turn_number} ({context.mode})
                </span>
                <span className={`text-[11px] font-mono px-2 py-0.5 rounded-md border ${strategyColor(context.strategy)}`}>
                  {context.strategy}
                </span>
                {context.cached_tokens !== undefined && context.cached_tokens > 0 && (
                  <span className="flex items-center gap-1 text-[11px] font-mono px-2 py-0.5 rounded-md bg-purple-950/80 text-purple-300 border border-purple-700/50">
                    <Zap className="w-3 h-3" />
                    <span>{context.cached_tokens} cached tokens</span>
                  </span>
                )}
              </div>
              <div className="text-xs font-mono text-stone-400">
                Tokens: <span className="text-stone-200">{context.estimated_tokens}</span> / {context.budget}
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-[11px] font-mono pt-2 border-t border-stone-800/80">
              <div className="flex items-center gap-1 text-stone-400 truncate">
                <Hash className="w-3.5 h-3.5 shrink-0 text-stone-500" />
                <span className="text-stone-500">Prompt Hash:</span>
                <span className="text-stone-300 truncate" title={context.prompt_hash}>
                  {context.prompt_hash ? context.prompt_hash.slice(0, 16) + '...' : 'none'}
                </span>
              </div>
              <div className="flex items-center gap-1 text-stone-400 truncate">
                <Cpu className="w-3.5 h-3.5 shrink-0 text-stone-500" />
                <span className="text-stone-500">Prefix Hash:</span>
                <span className="text-stone-300 truncate" title={context.prefix_hash}>
                  {context.prefix_hash ? context.prefix_hash.slice(0, 16) + '...' : 'none'}
                </span>
              </div>
              {context.session && (
                <>
                  <div className="flex items-center gap-1 text-stone-400 truncate">
                    <Database className="w-3.5 h-3.5 shrink-0 text-stone-500" />
                    <span className="text-stone-500">Session ID:</span>
                    <span className="text-purple-300 truncate" title={context.session.id}>
                      {context.session.id}
                    </span>
                  </div>
                  <div className="flex items-center gap-1 text-stone-400 truncate">
                    <Layers className="w-3.5 h-3.5 shrink-0 text-stone-500" />
                    <span className="text-stone-500">Session Tip:</span>
                    <span className="text-stone-300">Turn {context.session.through_turn}</span>
                  </div>
                </>
              )}
            </div>
          </div>

          {/* Active Continuity (Working Set) */}
          <div className="space-y-3">
            <h4 className="text-xs font-sans uppercase font-bold tracking-wider text-purple-400 flex items-center justify-between">
              <span>Active Continuity Working Set</span>
              <span className="font-mono text-stone-400 text-[11px]">{workingSet.length} entries</span>
            </h4>
            {workingSet.length === 0 ? (
              <p className="text-xs text-stone-500 italic bg-black/20 p-3 rounded-xl border border-stone-800">
                No entries in working set.
              </p>
            ) : (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                {workingSet.map((item) => (
                  <div
                    key={item.id}
                    onClick={() => onSelectWikilink && onSelectWikilink(item.id)}
                    className="p-2.5 rounded-xl bg-glass-card border border-stone-800/80 hover:border-purple-500/40 transition-colors flex items-center justify-between cursor-pointer"
                  >
                    <div className="space-y-0.5 min-w-0 pr-2">
                      <div className="flex items-center gap-1.5 truncate">
                        <Link2 className="w-3 h-3 text-purple-400 shrink-0" />
                        <span className="text-xs font-sans font-medium text-stone-200 truncate">
                          {item.name || item.id}
                        </span>
                      </div>
                      <div className="text-[10px] font-mono text-stone-500 flex items-center gap-2">
                        <span>{item.kind}</span>
                        {item.role && <span>• {item.role}</span>}
                        <span>• Turn {item.last_turn}</span>
                      </div>
                    </div>
                    <span className="font-mono text-xs font-bold text-purple-400 shrink-0">
                      {item.weight.toFixed(1)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Prompt Sections Breakdown */}
          <div className="space-y-3">
            <h4 className="text-xs font-sans uppercase font-bold tracking-wider text-purple-400 flex items-center justify-between">
              <span>Prompt Sections Breakdown</span>
              <span className="font-mono text-stone-400 text-[11px]">
                {context.sections.filter((s) => s.included).length} / {context.sections.length} included
              </span>
            </h4>
            <div className="space-y-2">
              {context.sections.map((sec) => (
                <div
                  key={sec.name}
                  className={`p-3 rounded-xl border transition-colors ${
                    sec.included
                      ? 'bg-glass-card border-stone-800'
                      : 'bg-black/30 border-stone-900 opacity-60'
                  }`}
                >
                  <div className="flex items-center justify-between mb-1.5">
                    <div className="flex items-center gap-2">
                      {sec.included ? (
                        <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                      ) : (
                        <XCircle className="w-4 h-4 text-stone-500" />
                      )}
                      <span className="text-xs font-sans font-bold text-stone-200 uppercase tracking-wide">
                        {sec.name}
                      </span>
                      {sec.source && (
                        <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-stone-900 text-stone-400">
                          {sec.source}
                        </span>
                      )}
                    </div>
                    <span className="text-xs font-mono text-stone-400">
                      ~{sec.tokens} tokens
                    </span>
                  </div>

                  {sec.refs && sec.refs.length > 0 && (
                    <div className="flex flex-wrap gap-1.5 mt-2 pt-2 border-t border-stone-800/60">
                      {sec.refs.map((r, i) => (
                        <button
                          key={`${r.kind}-${r.id}-${i}`}
                          onClick={() => handleRefClick(r)}
                          className="flex items-center gap-1 text-[11px] font-mono px-2 py-0.5 rounded-md bg-stone-900/80 hover:bg-purple-950/60 border border-stone-800 hover:border-purple-600/50 text-stone-300 hover:text-purple-200 transition-colors cursor-pointer"
                        >
                          <span className="text-purple-400 font-bold">[[</span>
                          <span>{r.id}</span>
                          {r.relation && (
                            <span className="text-stone-500 text-[10px]">({r.relation})</span>
                          )}
                          <span className="text-purple-400 font-bold">]]</span>
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>

          {/* Full Prompt Toggle */}
          {context.prompt && (
            <div className="space-y-2 pt-2 border-t border-stone-800">
              <button
                onClick={() => setShowPrompt((p) => !p)}
                className="flex items-center gap-2 text-xs font-sans uppercase font-bold tracking-wider text-purple-400 hover:text-purple-300 cursor-pointer"
              >
                {showPrompt ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
                <FileText className="w-3.5 h-3.5" />
                <span>Assembled Prompt Snapshot</span>
              </button>
              {showPrompt && (
                <pre className="p-3 bg-stone-950 rounded-xl border border-stone-800 text-[11px] font-mono text-stone-300 overflow-x-auto whitespace-pre-wrap leading-relaxed max-h-96">
                  {context.prompt}
                </pre>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );

  // If used standalone with Drawers modal wrapper:
  if (isOpen !== undefined && onClose !== undefined) {
    return (
      <Drawers isOpen={isOpen} onClose={onClose} title="Turn Context & Continuity" size="xl">
        {content}
      </Drawers>
    );
  }

  // Otherwise rendered inline inside a parent drawer:
  return content;
};
