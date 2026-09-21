import React, { useState, useEffect } from 'react';
import { APIClient } from '../api/client';
import { GameSummary, SystemInfo, WorldInfo, CreateGameRequest } from '../types';
import { Play, Plus, User, Clock, Shield, Globe, Compass, X, Sparkles, BookOpen, AlertCircle, Settings } from 'lucide-react';
import { SystemsStudio } from './SystemsStudio';
import { WorldsStudio } from './WorldsStudio';
import { SettingsStudio } from './SettingsStudio';

interface LauncherHubProps {
  onSelectGame: (gameId: string) => void;
}

export const LauncherHub: React.FC<LauncherHubProps> = ({ onSelectGame }) => {
  const [games, setGames] = useState<GameSummary[]>([]);
  const [systems, setSystems] = useState<SystemInfo[]>([]);
  const [worlds, setWorlds] = useState<WorldInfo[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isWizardOpen, setIsWizardOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'campaigns' | 'systems' | 'worlds' | 'settings'>('campaigns');

  // Wizard form state
  const [newGameName, setNewGameName] = useState('');
  const [newSystemID, setNewSystemID] = useState('');
  const [newWorldID, setNewWorldID] = useState('');
  const [newPlayerName, setNewPlayerName] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    loadData();
  }, []);

  const loadData = async () => {
    setIsLoading(true);
    setError(null);
    try {
      const [gList, sList, wList] = await Promise.all([
        APIClient.listGames().catch(() => []),
        APIClient.listSystems().catch(() => []),
        APIClient.listWorlds().catch(() => []),
      ]);
      setGames(gList);
      setSystems(sList);
      setWorlds(wList);

      if (sList.length > 0 && !newSystemID) setNewSystemID(sList[0].id);
      if (wList.length > 0 && !newWorldID) setNewWorldID(wList[0].id);
    } catch (err: any) {
      setError(err.message || 'Failed to load campaigns');
    } finally {
      setIsLoading(false);
    }
  };

  const handleCreateGame = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newGameName.trim()) return;

    const effectiveSystemID = newSystemID || systems[0]?.id;
    const effectiveWorldID = newWorldID || worlds[0]?.id;

    if (!effectiveSystemID || !effectiveWorldID) {
      setError('Please create both a Rule System and a World Setting before launching a campaign.');
      return;
    }

    setIsSubmitting(true);
    try {
      const payload: CreateGameRequest = {
        name: newGameName.trim(),
        system_id: effectiveSystemID,
        world_id: effectiveWorldID,
        player_name: newPlayerName.trim() || 'Adventurer',
      };

      const created = await APIClient.createGame(payload);
      setIsWizardOpen(false);
      onSelectGame(created.id);
    } catch (err: any) {
      setError(err.message || 'Failed to create campaign');
      setIsSubmitting(false);
    }
  };

  const selectedWorld = worlds.find((w) => w.id === newWorldID);
  const latestGame = games.length > 0 ? games[0] : null;

  return (
    <div className="relative flex flex-col h-screen overflow-hidden text-stone-200 p-6 md:p-8 select-none">
      {/* Brand Header */}
      <header className="relative z-10 mx-auto w-full max-w-6xl shrink-0 h-auto sm:h-16 bg-glass rounded-2xl px-6 py-3 sm:py-0 flex flex-wrap items-center justify-between gap-4 shadow-2xl mb-6">
        <div className="flex items-center gap-3">
          <div className="w-3 h-3 rounded-full bg-amber-500 shadow-[0_0_10px_rgba(245,158,11,0.9)] animate-pulse" />
          <h1 className="font-cinzel text-xl font-extrabold text-amber-400 tracking-wider">
            LocalRPG
          </h1>
          <span className="text-[10px] uppercase font-mono tracking-widest text-stone-400 bg-stone-900/60 px-2 py-0.5 rounded-full border border-stone-800 hidden sm:inline">
            Chronicle Hub
          </span>
        </div>

        {/* Top-Level Studio Navigation Tabs */}
        <nav className="flex items-center gap-1 bg-stone-950/70 p-1 rounded-xl border border-stone-800">
          <button
            onClick={() => {
              setActiveTab('campaigns');
              loadData();
            }}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeTab === 'campaigns'
                ? 'bg-amber-600 text-stone-950 font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <BookOpen className="w-3.5 h-3.5" />
            <span>Campaigns</span>
          </button>
          <button
            onClick={() => setActiveTab('systems')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeTab === 'systems'
                ? 'bg-amber-600 text-stone-950 font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Shield className="w-3.5 h-3.5" />
            <span>Rule Systems</span>
          </button>
          <button
            onClick={() => setActiveTab('worlds')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeTab === 'worlds'
                ? 'bg-amber-600 text-stone-950 font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Globe className="w-3.5 h-3.5" />
            <span>Worlds Studio</span>
          </button>
          <button
            onClick={() => setActiveTab('settings')}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-cinzel transition-all cursor-pointer ${
              activeTab === 'settings'
                ? 'bg-amber-600 text-stone-950 font-bold shadow'
                : 'text-stone-400 hover:text-stone-200'
            }`}
          >
            <Settings className="w-3.5 h-3.5" />
            <span>Settings</span>
          </button>
        </nav>

        <button
          onClick={() => setIsWizardOpen(true)}
          className="flex items-center gap-2 text-xs font-cinzel font-bold px-4 py-2 rounded-xl transition-all cursor-pointer bg-amber-600 hover:bg-amber-500 text-stone-950 shadow-[0_0_15px_rgba(217,119,6,0.5)] active:scale-95"
        >
          <Plus className="w-4 h-4" />
          <span>New Campaign</span>
        </button>
      </header>

      {/* Main Content Area */}
      <main className="relative z-10 mx-auto w-full max-w-6xl flex-1 min-h-0 flex flex-col">
        {error && (
          <div className="mb-4 p-4 rounded-xl bg-red-950/60 border border-red-500/40 text-red-200 text-sm flex items-center justify-between shrink-0">
            <span>{error}</span>
            <button onClick={() => setError(null)} className="text-red-400 hover:text-red-200">
              <X className="w-4 h-4" />
            </button>
          </div>
        )}

        {activeTab === 'campaigns' && (
          <div className="flex-1 overflow-y-auto space-y-8 pr-1">
            {/* Hero Resume Banner if a campaign exists */}
            {latestGame && (
              <div className="relative overflow-hidden rounded-2xl bg-glass-card border border-amber-500/20 shadow-2xl p-6 md:p-8 flex flex-col md:flex-row items-start md:items-center justify-between gap-6 backdrop-blur-md">
                <div className="flex-1 space-y-2">
                  <div className="flex items-center gap-2 text-xs font-mono uppercase tracking-wider text-amber-400">
                    <Sparkles className="w-3.5 h-3.5" />
                    <span>Last Played Adventure</span>
                  </div>
                  <h2 className="font-cinzel text-2xl md:text-3xl font-bold text-white tracking-wide">
                    {latestGame.name}
                  </h2>
                  <div className="flex flex-wrap items-center gap-4 text-xs text-stone-300 pt-1">
                    <div className="flex items-center gap-1.5 bg-stone-900/60 px-2.5 py-1 rounded-lg border border-stone-800">
                      <User className="w-3.5 h-3.5 text-amber-400" />
                      <span>{latestGame.player_name}</span>
                    </div>
                    <div className="flex items-center gap-1.5 bg-stone-900/60 px-2.5 py-1 rounded-lg border border-stone-800">
                      <Globe className="w-3.5 h-3.5 text-amber-400" />
                      <span>{latestGame.world_id}</span>
                    </div>
                    <div className="flex items-center gap-1.5 bg-stone-900/60 px-2.5 py-1 rounded-lg border border-stone-800">
                      <Clock className="w-3.5 h-3.5 text-amber-400" />
                      <span>Turn {latestGame.turn_count}</span>
                    </div>
                  </div>
                </div>

                <button
                  onClick={() => onSelectGame(latestGame.id)}
                  className="flex items-center gap-2.5 text-sm font-cinzel font-bold px-6 py-3.5 rounded-xl transition-all cursor-pointer bg-gradient-to-r from-amber-600 to-amber-500 hover:from-amber-500 hover:to-amber-400 text-stone-950 shadow-[0_0_20px_rgba(217,119,6,0.6)] active:scale-95"
                >
                  <Play className="w-4 h-4 fill-stone-950" />
                  <span>Resume Adventure</span>
                </button>
              </div>
            )}

        {/* Campaigns Grid */}
        <section className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="font-cinzel text-lg font-semibold text-stone-200 tracking-wider flex items-center gap-2">
              <BookOpen className="w-4 h-4 text-amber-400" />
              <span>Saved Chronicles</span>
            </h3>
            <span className="text-xs text-stone-400 font-mono">
              {games.length} {games.length === 1 ? 'campaign' : 'campaigns'}
            </span>
          </div>

          {isLoading ? (
            <div className="p-12 text-center text-stone-400 font-mono text-sm animate-pulse">
              Scanning local chronicles...
            </div>
          ) : games.length === 0 ? (
            <div className="p-12 rounded-2xl bg-glass-card border border-stone-800/80 text-center space-y-4 shadow-xl">
              <Compass className="w-12 h-12 text-amber-500/60 mx-auto stroke-1" />
              <div className="space-y-1">
                <h4 className="font-cinzel text-lg font-bold text-stone-200">
                  {systems.length === 0 || worlds.length === 0 ? 'Welcome to LocalRPG' : 'No Chronicles Found'}
                </h4>
                <p className="text-xs text-stone-400 max-w-md mx-auto">
                  {systems.length === 0 || worlds.length === 0
                    ? 'LocalRPG starts completely blank with no pre-installed defaults. Visit the Rule Systems and Worlds Studio tabs to create or load the reference templates, then return here to begin!'
                    : 'Your journey has yet to begin. Create a new campaign to awaken the world and weave your first turn.'}
                </p>
              </div>
              <div className="flex flex-wrap items-center justify-center gap-3 pt-2">
                {systems.length === 0 && (
                  <button
                    onClick={() => setActiveTab('systems')}
                    className="inline-flex items-center gap-1.5 text-xs font-cinzel font-bold px-4 py-2 rounded-xl bg-stone-900 border border-amber-500/40 hover:bg-stone-800 text-amber-300 transition-all cursor-pointer shadow"
                  >
                    <Shield className="w-3.5 h-3.5" />
                    <span>Rule Systems</span>
                  </button>
                )}
                {worlds.length === 0 && (
                  <button
                    onClick={() => setActiveTab('worlds')}
                    className="inline-flex items-center gap-1.5 text-xs font-cinzel font-bold px-4 py-2 rounded-xl bg-stone-900 border border-amber-500/40 hover:bg-stone-800 text-amber-300 transition-all cursor-pointer shadow"
                  >
                    <Globe className="w-3.5 h-3.5" />
                    <span>Worlds Studio</span>
                  </button>
                )}
                <button
                  onClick={() => setIsWizardOpen(true)}
                  className="inline-flex items-center gap-2 text-xs font-cinzel font-bold px-5 py-2.5 rounded-xl bg-amber-600 hover:bg-amber-500 text-stone-950 transition-all cursor-pointer shadow-lg"
                >
                  <Plus className="w-4 h-4" />
                  <span>Begin Your Tale</span>
                </button>
              </div>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {games.map((g) => (
                <div
                  key={g.id}
                  onClick={() => onSelectGame(g.id)}
                  className="group relative rounded-xl bg-glass-card hover:bg-stone-900/80 border border-stone-800/80 hover:border-amber-500/50 p-5 transition-all duration-300 cursor-pointer shadow-lg hover:shadow-[0_0_20px_rgba(217,119,6,0.2)] flex flex-col justify-between"
                >
                  <div className="space-y-2">
                    <div className="flex items-center justify-between text-[11px] font-mono text-stone-400">
                      <span className="bg-stone-950/70 px-2 py-0.5 rounded border border-stone-800/60 text-amber-400">
                        {g.system_id}
                      </span>
                      <span>Turn {g.turn_count}</span>
                    </div>
                    <h4 className="font-cinzel text-base font-bold text-stone-100 group-hover:text-amber-400 transition-colors">
                      {g.name}
                    </h4>
                  </div>

                  <div className="pt-4 mt-4 border-t border-stone-800/60 flex items-center justify-between text-xs text-stone-400">
                    <div className="flex items-center gap-1.5">
                      <User className="w-3.5 h-3.5 text-stone-500 group-hover:text-amber-400 transition-colors" />
                      <span className="font-sans truncate max-w-[120px]">{g.player_name}</span>
                    </div>
                    <div className="flex items-center gap-1 text-[11px] text-amber-500/80 group-hover:text-amber-400">
                      <span>Launch</span>
                      <Play className="w-3 h-3 fill-current" />
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>
    )}

    {activeTab === 'systems' && (
      <SystemsStudio onSystemSaved={loadData} />
    )}

    {activeTab === 'worlds' && (
      <WorldsStudio onWorldSaved={loadData} />
    )}

    {activeTab === 'settings' && (
      <div className="flex-1 overflow-hidden">
        <SettingsStudio onSaved={loadData} />
      </div>
    )}
  </main>

      {/* New Campaign Creation Wizard Modal */}
      {isWizardOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-fade-in">
          <div className="relative w-full max-w-lg max-h-[85vh] flex flex-col rounded-2xl bg-stone-900/95 border border-amber-500/30 shadow-2xl overflow-hidden">
            {/* Fixed Header */}
            <div className="flex items-center justify-between border-b border-stone-800 px-6 py-4 shrink-0">
              <div className="flex items-center gap-2">
                <Sparkles className="w-5 h-5 text-amber-400" />
                <h3 className="font-cinzel text-lg font-bold text-amber-400">
                  New Campaign Wizard
                </h3>
              </div>
              <button
                onClick={() => setIsWizardOpen(false)}
                className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors cursor-pointer"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* Form with scrollable body and pinned footer */}
            <form onSubmit={handleCreateGame} className="flex-1 min-h-0 flex flex-col overflow-hidden">
              <div className="flex-1 overflow-y-auto p-6 space-y-4">
                {(!systems.length || !worlds.length) && (
                  <div className="p-4 rounded-xl bg-amber-950/40 border border-amber-500/40 space-y-3 text-xs text-amber-200">
                    <div className="flex items-center gap-2 font-cinzel font-bold text-amber-400">
                      <AlertCircle className="w-4 h-4" />
                      <span>Setup Required</span>
                    </div>
                    <p className="text-stone-300">
                      LocalRPG starts with no pre-installed defaults. Before creating a campaign, please author or load a reference template in the Studios.
                    </p>
                    <div className="flex flex-wrap gap-2 pt-1">
                      {systems.length === 0 && (
                        <button
                          type="button"
                          onClick={() => {
                            setIsWizardOpen(false);
                            setActiveTab('systems');
                          }}
                          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-amber-600 hover:bg-amber-500 text-stone-950 font-cinzel font-bold cursor-pointer transition-all shadow"
                        >
                          <Shield className="w-3.5 h-3.5" />
                          <span>Create Rule System</span>
                        </button>
                      )}
                      {worlds.length === 0 && (
                        <button
                          type="button"
                          onClick={() => {
                            setIsWizardOpen(false);
                            setActiveTab('worlds');
                          }}
                          className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-amber-600 hover:bg-amber-500 text-stone-950 font-cinzel font-bold cursor-pointer transition-all shadow"
                        >
                          <Globe className="w-3.5 h-3.5" />
                          <span>Create World Setting</span>
                        </button>
                      )}
                    </div>
                  </div>
                )}

                <div className="space-y-1.5">
                  <label className="text-xs font-cinzel uppercase tracking-wider text-stone-300">
                    Campaign Title
                  </label>
                  <input
                    type="text"
                    required
                    placeholder="e.g. Whispers of the High Hollow"
                    value={newGameName}
                    onChange={(e) => setNewGameName(e.target.value)}
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-amber-500/60 transition-colors"
                  />
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div className="space-y-1.5">
                    <label className="text-xs font-cinzel uppercase tracking-wider text-stone-300 flex items-center gap-1.5">
                      <Shield className="w-3.5 h-3.5 text-amber-400" />
                      <span>Rule System</span>
                    </label>
                    <select
                      value={newSystemID}
                      onChange={(e) => setNewSystemID(e.target.value)}
                      disabled={systems.length === 0}
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2.5 text-sm text-stone-100 focus:outline-none focus:border-amber-500/60 transition-colors cursor-pointer disabled:opacity-50"
                    >
                      {systems.length === 0 && <option value="" disabled>No rule systems available</option>}
                      {systems.map((s) => (
                        <option key={s.id} value={s.id}>
                          {s.name}
                        </option>
                      ))}
                    </select>
                  </div>

                  <div className="space-y-1.5">
                    <label className="text-xs font-cinzel uppercase tracking-wider text-stone-300 flex items-center gap-1.5">
                      <Globe className="w-3.5 h-3.5 text-amber-400" />
                      <span>World Setting</span>
                    </label>
                    <select
                      value={newWorldID}
                      onChange={(e) => setNewWorldID(e.target.value)}
                      disabled={worlds.length === 0}
                      className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2.5 text-sm text-stone-100 focus:outline-none focus:border-amber-500/60 transition-colors cursor-pointer disabled:opacity-50"
                    >
                      {worlds.length === 0 && <option value="" disabled>No worlds available</option>}
                      {worlds.map((w) => (
                        <option key={w.id} value={w.id}>
                          {w.name}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>

                {selectedWorld?.description && (
                  <p className="text-[11px] text-stone-400 italic bg-stone-950/60 p-3 rounded-xl border border-stone-800/60">
                    "{selectedWorld.description}"
                  </p>
                )}

                <div className="space-y-1.5">
                  <label className="text-xs font-cinzel uppercase tracking-wider text-stone-300 flex items-center gap-1.5">
                    <User className="w-3.5 h-3.5 text-amber-400" />
                    <span>Protagonist Character Name</span>
                  </label>
                  <input
                    type="text"
                    required
                    placeholder="e.g. Elena Nightshade"
                    value={newPlayerName}
                    onChange={(e) => setNewPlayerName(e.target.value)}
                    className="w-full bg-stone-950 border border-stone-800 rounded-xl px-4 py-2.5 text-sm text-stone-100 placeholder-stone-600 focus:outline-none focus:border-amber-500/60 transition-colors"
                  />
                </div>
              </div>

              {/* Pinned Action Footer */}
              <div className="flex items-center justify-end gap-3 border-t border-stone-800 px-6 py-4 shrink-0 bg-stone-900/90">
                <button
                  type="button"
                  onClick={() => setIsWizardOpen(false)}
                  className="px-4 py-2 text-xs font-cinzel text-stone-400 hover:text-white transition-colors cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={
                    isSubmitting ||
                    !newGameName.trim() ||
                    !newPlayerName.trim() ||
                    systems.length === 0 ||
                    worlds.length === 0
                  }
                  className="flex items-center gap-2 text-xs font-cinzel font-bold px-5 py-2.5 rounded-xl bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-stone-950 shadow-lg transition-all cursor-pointer"
                >
                  <Sparkles className="w-3.5 h-3.5" />
                  <span>{isSubmitting ? 'Weaving World...' : 'Embark on Adventure'}</span>
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
