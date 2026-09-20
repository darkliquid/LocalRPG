import React, { useState, useEffect, useMemo } from 'react';
import { APIClient } from './api/client';
import { GameState, Turn, EntityNote, GraphData } from './types';
import { ChronicleView } from './components/ChronicleView';
import { ActionConsole } from './components/ActionConsole';
import { Drawers } from './components/Drawers';
import { CharacterSheetDrawer } from './components/CharacterSheetDrawer';
import { GraphDrawer } from './components/GraphDrawer';
import { CodexDrawer } from './components/CodexDrawer';
import { LivingWorldDrawer } from './components/LivingWorldDrawer';
import { StoryTheater } from './components/StoryTheater';
import { LauncherHub } from './components/LauncherHub';
import { SettingsStudio } from './components/SettingsStudio';
import { User, Network, BookOpen, Clock, Film, Compass, Settings } from 'lucide-react';

export const App: React.FC = () => {
  const [activeGameID, setActiveGameID] = useState<string | null>(() => {
    return localStorage.getItem('localrpg_active_game') || null;
  });

  const client = useMemo(() => {
    return activeGameID ? new APIClient(activeGameID) : null;
  }, [activeGameID]);

  const [gameState, setGameState] = useState<GameState | null>(null);
  const [chronicle, setChronicle] = useState<Turn[]>([]);
  const [graph, setGraph] = useState<GraphData | null>(null);
  const [selectedEntity, setSelectedEntity] = useState<EntityNote | null>(null);

  // Active drawer tab: null, 'character', 'graph', 'codex', 'world'
  const [activeDrawer, setActiveDrawer] = useState<string | null>(null);
  const [isTheaterOpen, setIsTheaterOpen] = useState(false);

  useEffect(() => {
    if (!client) {
      setGameState(null);
      setChronicle([]);
      setGraph(null);
      return;
    }
    client.getGameState().then(setGameState).catch(console.error);
    client.getChronicle().then(setChronicle).catch(console.error);
    client.getGraph().then(setGraph).catch(console.error);
  }, [client]);

  const handleSelectGame = (gameId: string) => {
    localStorage.setItem('localrpg_active_game', gameId);
    setActiveGameID(gameId);
  };

  const handleReturnToLauncher = () => {
    setActiveGameID(null);
    setActiveDrawer(null);
  };

  const handleOpenWikilink = async (entityId: string) => {
    if (!client) return;
    try {
      const ent = await client.getEntity(entityId);
      setSelectedEntity(ent);
      setActiveDrawer('codex');
    } catch (err) {
      console.error(err);
    }
  };

  const handleActionSubmit = async (mode: string, text: string) => {
    // Add optimistic turn
    const nextTurn: Turn = {
      turn_number: chronicle.length + 1,
      input_text: text,
      mode: mode,
      prose: 'The storyteller ponders your directive...',
    };
    setChronicle((prev) => [...prev, nextTurn]);
  };

  const handleSaveEntity = async (entityId: string, markdown: string) => {
    if (!client) return;
    await client.saveEntity(entityId, markdown);
    const updated = await client.getEntity(entityId);
    setSelectedEntity(updated);
  };

  // Find latest scene image for full-window atmospheric background
  const activeBgImage = chronicle.slice().reverse().find((t) => t.image_url)?.image_url;

  return (
    <div className="relative flex flex-col h-screen overflow-hidden text-stone-200">
      {/* Full-window atmospheric background layer (Twintail Launcher aesthetic) */}
      <div
        id="app-bg"
        key={activeBgImage || 'default'}
        className="fixed inset-0 bg-cover bg-center bg-no-repeat animate-bg-fade-in transition-all duration-700 pointer-events-none"
        style={{
          backgroundImage: activeBgImage ? `url(${activeBgImage})` : 'radial-gradient(ellipse at center, #261e1b 0%, #0c0a09 100%)',
          backgroundColor: '#0c0a09',
        }}
      />

      {/* Cinematic dark vignette and noise overlays */}
      <div className="fixed inset-0 bg-gradient-to-b from-black/70 via-black/45 to-black/85 pointer-events-none" />
      <div className="fixed inset-0 bg-radial-[circle_at_center] from-transparent via-black/30 to-black/90 pointer-events-none bg-noise" />

      {/* When no game is selected: Mount the Launcher Hub */}
      {!activeGameID ? (
        <LauncherHub onSelectGame={handleSelectGame} />
      ) : (
        <>
          {/* Floating Translucent Acrylic Header */}
          <header className="relative z-10 mx-6 mt-4 mb-2 h-14 bg-glass rounded-2xl px-6 flex items-center justify-between shadow-2xl">
            <div className="flex items-center gap-4">
              <button
                onClick={handleReturnToLauncher}
                className="flex items-center gap-1.5 text-xs font-cinzel font-bold px-3 py-1.5 rounded-xl transition-all cursor-pointer bg-stone-900/60 hover:bg-stone-800 text-amber-400 border border-amber-500/30"
                title="Switch Campaign / Return to Hub"
              >
                <Compass className="w-3.5 h-3.5" />
                <span>Campaigns</span>
              </button>

              <div className="flex items-center gap-2">
                <div className="w-2.5 h-2.5 rounded-full bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.8)]" />
                <h1 className="font-cinzel text-base md:text-lg font-bold text-amber-400 tracking-wider">
                  {gameState?.game_name || activeGameID}
                </h1>
              </div>
            </div>

            {/* Floating Drawer Trigger Pills */}
            <div className="flex items-center gap-2">
              <button
                onClick={() => setActiveDrawer('character')}
                className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  activeDrawer === 'character' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
              >
                <User className="w-3.5 h-3.5" />
                <span className="hidden sm:inline">Character</span>
              </button>
              <button
                onClick={() => setActiveDrawer('graph')}
                className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  activeDrawer === 'graph' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
              >
                <Network className="w-3.5 h-3.5" />
                <span className="hidden sm:inline">Graph</span>
              </button>
              <button
                onClick={() => setActiveDrawer('codex')}
                className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  activeDrawer === 'codex' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
              >
                <BookOpen className="w-3.5 h-3.5" />
                <span className="hidden sm:inline">Codex</span>
              </button>
              <button
                onClick={() => setActiveDrawer('world')}
                className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  activeDrawer === 'world' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
              >
                <Clock className="w-3.5 h-3.5" />
                <span className="hidden sm:inline">World Arcs</span>
              </button>
              <button
                onClick={() => setIsTheaterOpen(true)}
                className="flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer text-stone-300 hover:text-white hover:bg-white/10"
                title="Open Story Theater replay mode"
              >
                <Film className="w-3.5 h-3.5 text-amber-400" />
                <span className="hidden sm:inline">Theater</span>
              </button>
              <button
                onClick={() => setActiveDrawer('settings')}
                className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  activeDrawer === 'settings' ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
                }`}
                title="Global Settings"
              >
                <Settings className="w-3.5 h-3.5 text-amber-400" />
                <span className="hidden sm:inline">Settings</span>
              </button>
            </div>
          </header>

          {/* Main Floating Translucent Chronicle & Action Container */}
          <main className="relative z-10 flex-1 overflow-hidden mx-6 mb-4 flex flex-col">
            <div className="flex-1 bg-glass-card rounded-2xl flex flex-col overflow-hidden shadow-2xl">
              <ChronicleView turns={chronicle} onWikilinkClick={handleOpenWikilink} />
              <ActionConsole onSubmit={handleActionSubmit} />
            </div>
          </main>

          {/* Flyout Drawer Modal */}
          <Drawers
            isOpen={activeDrawer !== null}
            onClose={() => setActiveDrawer(null)}
            title={
              activeDrawer === 'character' ? 'Character Sheet' :
              activeDrawer === 'graph' ? 'Lore Graph' :
              activeDrawer === 'codex' ? 'Codex Markdown Editor' :
              activeDrawer === 'world' ? 'Living World Arcs & Clocks' : 'Global Settings'
            }
          >
            {activeDrawer === 'character' && <CharacterSheetDrawer player={gameState?.player} />}
            {activeDrawer === 'graph' && <GraphDrawer data={graph || undefined} onSelectNode={handleOpenWikilink} />}
            {activeDrawer === 'codex' && <CodexDrawer entity={selectedEntity || undefined} onSave={handleSaveEntity} />}
            {activeDrawer === 'world' && <LivingWorldDrawer state={gameState || undefined} />}
            {activeDrawer === 'settings' && <SettingsStudio isCompact={true} />}
          </Drawers>

          {/* Full-Screen Visual Novel Story Theater */}
          <StoryTheater
            turns={chronicle}
            isOpen={isTheaterOpen}
            onClose={() => setIsTheaterOpen(false)}
          />
        </>
      )}
    </div>
  );
};
