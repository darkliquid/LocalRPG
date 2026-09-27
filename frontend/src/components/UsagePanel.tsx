import React, { useState, useEffect, useCallback } from 'react';
import { Usage, UsageRow } from '../types';
import { APIClient } from '../api/client';
import { Coins, RefreshCw, Filter, ArrowLeft, AlertCircle, Cpu, ShieldAlert } from 'lucide-react';

interface UsagePanelProps {
  activeGameID?: string;
}

export const formatCost = (micros: number, currency: string = 'USD'): string => {
  if (!micros || micros === 0) return `0.00 ${currency}`;
  const amount = micros / 1_000_000;
  if (amount < 0.0001) {
    return `${amount.toFixed(6).replace(/0+$/, '')} ${currency}`;
  }
  if (amount < 0.01) {
    return `${amount.toFixed(4)} ${currency}`;
  }
  return `${amount.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 })} ${currency}`;
};

export const formatConsumption = (row: UsageRow): string[] => {
  const parts: string[] = [];
  if (row.input_tokens || row.output_tokens) {
    const inTokens = (row.input_tokens ?? 0).toLocaleString();
    const outTokens = (row.output_tokens ?? 0).toLocaleString();
    parts.push(`${inTokens} in / ${outTokens} out tokens`);
  }
  if (row.characters) {
    parts.push(`${row.characters.toLocaleString()} chars`);
  }
  if (row.requests) {
    parts.push(`${row.requests} req${row.requests === 1 ? '' : 's'}`);
  }
  return parts;
};

export const UsagePanel: React.FC<UsagePanelProps> = ({ activeGameID }) => {
  const [isAllCampaigns, setIsAllCampaigns] = useState<boolean>(!activeGameID);
  const [selectedDrilldownGameID, setSelectedDrilldownGameID] = useState<string | null>(null);
  const [usage, setUsage] = useState<Usage | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [providerFilter, setProviderFilter] = useState<string>('all');
  const [roleFilter, setRoleFilter] = useState<string>('all');

  const fetchUsage = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      if (selectedDrilldownGameID) {
        const res = await APIClient.getGameUsage(selectedDrilldownGameID);
        setUsage(res);
      } else if (isAllCampaigns) {
        const res = await APIClient.getGlobalUsage();
        setUsage(res);
      } else if (activeGameID) {
        const res = await APIClient.getGameUsage(activeGameID);
        setUsage(res);
      } else {
        const res = await APIClient.getGlobalUsage();
        setUsage(res);
      }
    } catch (err) {
      console.error('Failed to load usage:', err);
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [activeGameID, isAllCampaigns, selectedDrilldownGameID]);

  useEffect(() => {
    fetchUsage();
  }, [fetchUsage]);

  const currency = usage?.currency || 'USD';

  // Compute filtered rows
  const rawRows = usage?.rows ?? [];
  const availableProviders = Array.from(new Set(rawRows.map((r) => r.provider).filter(Boolean)));
  const availableRoles = Array.from(new Set(rawRows.map((r) => r.role).filter(Boolean)));

  const filteredRows = rawRows.filter((r) => {
    if (providerFilter !== 'all' && r.provider !== providerFilter) return false;
    if (roleFilter !== 'all' && r.role !== roleFilter) return false;
    return true;
  });

  return (
    <div className="space-y-6">
      {/* Scope Selector & Actions Header */}
      <div className="flex flex-wrap items-center justify-between gap-3 bg-stone-900/60 p-4 rounded-2xl border border-white/10">
        <div className="flex items-center gap-2">
          {selectedDrilldownGameID ? (
            <button
              onClick={() => setSelectedDrilldownGameID(null)}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans bg-white/10 hover:bg-white/15 text-stone-200 transition-colors cursor-pointer"
            >
              <ArrowLeft className="w-3.5 h-3.5" />
              <span>Back to all campaigns</span>
            </button>
          ) : (
            <>
              {activeGameID && (
                <button
                  onClick={() => {
                    setIsAllCampaigns(false);
                    setSelectedDrilldownGameID(null);
                  }}
                  className={`px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
                    !isAllCampaigns
                      ? 'bg-purple-600 text-white font-bold shadow'
                      : 'text-stone-400 hover:text-stone-200 bg-stone-950/50'
                  }`}
                >
                  Current Campaign
                </button>
              )}
              <button
                onClick={() => {
                  setIsAllCampaigns(true);
                  setSelectedDrilldownGameID(null);
                }}
                className={`px-3 py-1.5 rounded-lg text-xs font-sans transition-all cursor-pointer ${
                  isAllCampaigns
                    ? 'bg-purple-600 text-white font-bold shadow'
                    : 'text-stone-400 hover:text-stone-200 bg-stone-950/50'
                }`}
              >
                All Campaigns
              </button>
            </>
          )}

          {selectedDrilldownGameID && (
            <span className="text-xs font-mono text-purple-300 bg-purple-950/40 px-2.5 py-1 rounded-md border border-purple-800/40">
              Campaign: {selectedDrilldownGameID}
            </span>
          )}
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={fetchUsage}
            disabled={loading}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-sans bg-stone-800 hover:bg-stone-700 text-stone-300 hover:text-white transition-colors cursor-pointer disabled:opacity-50"
            title="Refresh usage data"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      {error && (
        <div className="p-4 bg-red-950/30 border border-red-800/50 rounded-2xl flex items-center gap-3 text-red-300 text-sm">
          <AlertCircle className="w-5 h-5 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {/* Top Level Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* Total Spend */}
        <div className="bg-stone-900/60 p-5 rounded-2xl border border-white/10 flex flex-col justify-between">
          <div className="flex items-center justify-between text-stone-400 text-xs font-sans uppercase tracking-wider mb-2">
            <span>Total Spend</span>
            <Coins className="w-4 h-4 text-purple-400" />
          </div>
          <div className="text-2xl font-bold font-mono text-white tracking-tight">
            {formatCost(usage?.total_cost_micros ?? 0, currency)}
          </div>
          <div className="text-xs text-stone-400 mt-2">
            {isAllCampaigns && !selectedDrilldownGameID
              ? 'Aggregated across all campaigns & studio work'
              : `Total recorded for this campaign`}
          </div>
        </div>

        {/* Breakdown by Provider */}
        <div className="bg-stone-900/60 p-5 rounded-2xl border border-white/10">
          <div className="flex items-center justify-between text-stone-400 text-xs font-sans uppercase tracking-wider mb-2">
            <span>By Provider</span>
            <Cpu className="w-4 h-4 text-purple-400" />
          </div>
          <div className="space-y-1.5 max-h-24 overflow-y-auto pr-1">
            {usage?.by_provider && Object.keys(usage.by_provider).length > 0 ? (
              Object.entries(usage.by_provider).map(([provider, micros]) => (
                <div key={provider} className="flex items-center justify-between text-xs">
                  <span className="text-stone-300 font-mono truncate max-w-[140px]">{provider}</span>
                  <span className="text-stone-400 font-mono">{formatCost(micros, currency)}</span>
                </div>
              ))
            ) : (
              <span className="text-xs text-stone-500 italic">No provider breakdown</span>
            )}
          </div>
        </div>

        {/* Breakdown by Role */}
        <div className="bg-stone-900/60 p-5 rounded-2xl border border-white/10">
          <div className="flex items-center justify-between text-stone-400 text-xs font-sans uppercase tracking-wider mb-2">
            <span>By Role</span>
            <ShieldAlert className="w-4 h-4 text-purple-400" />
          </div>
          <div className="space-y-1.5 max-h-24 overflow-y-auto pr-1">
            {usage?.by_role && Object.keys(usage.by_role).length > 0 ? (
              Object.entries(usage.by_role).map(([role, micros]) => (
                <div key={role} className="flex items-center justify-between text-xs">
                  <span className="text-stone-300 font-mono uppercase">{role}</span>
                  <span className="text-stone-400 font-mono">{formatCost(micros, currency)}</span>
                </div>
              ))
            ) : (
              <span className="text-xs text-stone-500 italic">No role breakdown</span>
            )}
          </div>
        </div>
      </div>

      {/* If in All Campaigns global view and no drilldown is active, show the Campaigns Drilldown list */}
      {isAllCampaigns && !selectedDrilldownGameID && usage?.campaigns && usage.campaigns.length > 0 && (
        <div className="bg-stone-900/60 rounded-2xl border border-white/10 overflow-hidden">
          <div className="px-5 py-3 border-b border-white/10 bg-white/[0.02] flex items-center justify-between">
            <h3 className="text-sm font-sans font-bold text-white">Campaign Drilldown</h3>
            <span className="text-xs text-stone-400">{usage.campaigns.length} campaigns</span>
          </div>
          <div className="divide-y divide-white/5">
            {usage.campaigns.map((camp) => (
              <div
                key={camp.game_id}
                onClick={() => setSelectedDrilldownGameID(camp.game_id)}
                className="px-5 py-3.5 flex items-center justify-between hover:bg-white/[0.04] transition-colors cursor-pointer"
              >
                <div>
                  <div className="text-sm font-sans font-medium text-white">
                    {camp.name || (camp.game_id === 'global' ? 'Shared / Studio Spend' : camp.game_id)}
                  </div>
                  <div className="text-xs font-mono text-stone-400">{camp.game_id}</div>
                </div>
                <div className="text-right">
                  <div className="text-sm font-mono font-bold text-purple-300">
                    {formatCost(camp.total_cost_micros, currency)}
                  </div>
                  <div className="text-[11px] text-stone-500">Click to view turns →</div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Per-Turn Usage Ledger Table */}
      {(!isAllCampaigns || selectedDrilldownGameID) && (
        <div className="bg-stone-900/60 rounded-2xl border border-white/10 overflow-hidden space-y-3 p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h3 className="text-sm font-sans font-bold text-white">Usage Ledger</h3>

            {/* Filter controls */}
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <div className="flex items-center gap-1.5 text-stone-400">
                <Filter className="w-3.5 h-3.5" />
                <span>Filters:</span>
              </div>
              <select
                value={providerFilter}
                onChange={(e) => setProviderFilter(e.target.value)}
                className="bg-stone-950 border border-stone-800 rounded-lg px-2 py-1 text-stone-300 focus:outline-none focus:border-purple-500"
              >
                <option value="all">All Providers</option>
                {availableProviders.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </select>

              <select
                value={roleFilter}
                onChange={(e) => setRoleFilter(e.target.value)}
                className="bg-stone-950 border border-stone-800 rounded-lg px-2 py-1 text-stone-300 focus:outline-none focus:border-purple-500"
              >
                <option value="all">All Roles</option>
                {availableRoles.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-stone-800 text-stone-400 font-sans uppercase tracking-wider">
                  <th className="py-2.5 px-3">Turn</th>
                  <th className="py-2.5 px-3">Role</th>
                  <th className="py-2.5 px-3">Provider & Model</th>
                  <th className="py-2.5 px-3">Consumption</th>
                  <th className="py-2.5 px-3 text-right">Cost</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-stone-800/60 font-mono">
                {filteredRows.length > 0 ? (
                  filteredRows.map((row, idx) => {
                    const hasConsumption =
                      (row.input_tokens ?? 0) > 0 ||
                      (row.output_tokens ?? 0) > 0 ||
                      (row.characters ?? 0) > 0 ||
                      (row.requests ?? 0) > 0;
                    const isUnpriced = hasConsumption && (!row.cost_micros || row.cost_micros === 0);
                    const consumptionParts = formatConsumption(row);

                    return (
                      <tr key={idx} className="hover:bg-white/[0.02] transition-colors">
                        <td className="py-2.5 px-3 text-stone-300">
                          {row.turn_number === 0 ? (
                            <span className="text-stone-400">preview</span>
                          ) : (
                            `#${row.turn_number}`
                          )}
                        </td>
                        <td className="py-2.5 px-3">
                          <span className="px-2 py-0.5 rounded text-[11px] bg-purple-950/60 border border-purple-800/40 text-purple-300 uppercase">
                            {row.role}
                          </span>
                        </td>
                        <td className="py-2.5 px-3 text-stone-200">
                          <div>{row.provider}</div>
                          {row.model && <div className="text-[11px] text-stone-400">{row.model}</div>}
                        </td>
                        <td className="py-2.5 px-3 text-stone-300">
                          <div className="flex flex-wrap items-center gap-1.5">
                            {consumptionParts.map((part, pIdx) => (
                              <span key={pIdx} className="text-stone-300">
                                {part}
                              </span>
                            ))}
                            {row.estimated && (
                              <span className="px-1.5 py-0.5 rounded text-[10px] bg-stone-800 text-stone-400 border border-stone-700">
                                estimated
                              </span>
                            )}
                          </div>
                        </td>
                        <td className="py-2.5 px-3 text-right font-bold">
                          {isUnpriced ? (
                            <span className="px-2 py-0.5 rounded text-[11px] bg-amber-950/40 border border-amber-800/40 text-amber-300">
                              no price configured
                            </span>
                          ) : (
                            <span className="text-stone-200">
                              {formatCost(row.cost_micros ?? 0, currency)}
                            </span>
                          )}
                        </td>
                      </tr>
                    );
                  })
                ) : (
                  <tr>
                    <td colSpan={5} className="py-8 text-center text-stone-500 italic font-sans">
                      {rawRows.length === 0
                        ? 'No usage records logged for this scope.'
                        : 'No usage records match the current filter.'}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
};
