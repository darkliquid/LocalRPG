import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { APIClient } from './api/client';
import { GameState, Turn, EntityNote, EntitySummary, Recap, GraphData, AppConfig } from './types';
import { ChronicleView } from './components/ChronicleView';
import { TurnSegments } from './components/TurnSegments';
import { ActionConsole } from './components/ActionConsole';
import { Drawers } from './components/Drawers';
import { CharacterSheetDrawer } from './components/CharacterSheetDrawer';
import { GraphDrawer } from './components/GraphDrawer';
import { CodexDrawer } from './components/CodexDrawer';
import { LivingWorldDrawer } from './components/LivingWorldDrawer';
import { StoryTheater } from './components/StoryTheater';
import { LauncherHub } from './components/LauncherHub';
import { SettingsStudio } from './components/SettingsStudio';
import { ProloguePanel } from './components/ProloguePanel';
import { User, Network, BookOpen, Clock, Film, Compass, Settings, X } from 'lucide-react';

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
  // The campaign's notes, as the codex lists them. It grows as turns create
  // entities, so it is refetched rather than held from first load.
  const [entities, setEntities] = useState<EntitySummary[]>([]);
  // A campaign's long memory, read with the rest of the corpus so the panel shows
  // it without a model call of its own.
  const [recap, setRecap] = useState<Recap | null>(null);
  const [selectedEntity, setSelectedEntity] = useState<EntityNote | null>(null);
  // TTS playback preferences live in the global settings, so the chronicle and
  // the story theater honour the same switches as the settings studio.
  const [config, setConfig] = useState<AppConfig | null>(null);

  // Active drawer tab: null, 'character', 'graph', 'codex', 'world'
  // The application plays audio itself when it can, which is the only way to
  // narrate a turn without a browser autoplay gesture.
  const [serverAudio, setServerAudio] = useState(false);

  const [activeDrawer, setActiveDrawer] = useState<string | null>(null);
  const [isTheaterOpen, setIsTheaterOpen] = useState(false);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [addressed, setAddressed] = useState<Set<number>>(new Set());

  useEffect(() => {
    APIClient.getSettings()
      .then((res) => setConfig(res.config))
      .catch(console.error);
    APIClient.audioStatus()
      .then((status) => setServerAudio(status.available))
      .catch(() => setServerAudio(false));
  }, []);

  // Everything the drawers read is refetched together, so no panel can show a
  // corpus that is older than another's.
  const refreshCorpus = useCallback(() => {
    if (!client) return;
    client.getGameState().then(setGameState).catch(console.error);
    client.getGraph().then(setGraph).catch(console.error);
    client.listEntities().then(setEntities).catch(console.error);
    client.getRecap().then(setRecap).catch(console.error);
  }, [client]);

  useEffect(() => {
    if (!client) {
      setGameState(null);
      setChronicle([]);
      setGraph(null);
      setEntities([]);
      setRecap(null);
      setAddressed(new Set());
      return;
    }
    client.getChronicle().then(setChronicle).catch(console.error);
    client.getFindings().then((res) => {
      setAddressed(new Set(res.addressed.map((a) => a.turn)));
    }).catch(console.error);
    refreshCorpus();
  }, [client, refreshCorpus]);

  const handleSelectGame = (gameId: string) => {
    localStorage.setItem('localrpg_active_game', gameId);
    setActiveGameID(gameId);
  };

  const handleReturnToLauncher = () => {
    setActiveGameID(null);
    setActiveDrawer(null);
    setIsSettingsOpen(false);
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

  const [turnInFlight, setTurnInFlight] = useState(false);
  const [streamedProse, setStreamedProse] = useState('');
  const abortRef = useRef<AbortController | null>(null);

  const handleActionSubmit = async (mode: string, text: string) => {
    if (!client || !activeGameID || turnInFlight) return;

    setTurnInFlight(true);
    setStreamedProse('');

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      await APIClient.streamTurn(
        activeGameID,
        { mode, input: text },
        (event) => {
          if (event.type === 'chunk') {
            setStreamedProse((prev) => prev + (event.text ?? ''));
          } else if (event.type === 'turn' && event.turn) {
            const turn = event.turn;
            setChronicle((prev) => [...prev, turn]);
            setStreamedProse('');
            // A turn can introduce characters, so the graph and the character
            // view are refreshed rather than left showing the state before it.
            refreshCorpus();
          } else if (event.type === 'error') {
            console.error('turn failed:', event.message);
          }
        },
        controller.signal
      );
    } catch (err) {
      console.error('turn failed:', err);
    } finally {
      abortRef.current = null;
      setTurnInFlight(false);
    }
  };

  const handleStopTurn = () => {
    abortRef.current?.abort();
    setStreamedProse('');
  };

  // Beginning the story saves the player's opening prompt and then runs the
  // campaign's first turn in the reserved Opening mode, so the GM establishes
  // the scene before the player is asked for anything.
  const handleBeginStory = async (prompt: string) => {
    if (!client || !activeGameID || turnInFlight) return;
    try {
      await APIClient.updateGameSettings(activeGameID, { opening_prompt: prompt });
      setGameState((prev) => (prev ? { ...prev, opening_prompt: prompt } : prev));
    } catch (err) {
      console.error('save opening prompt:', err);
    }
    await handleActionSubmit('Opening', '');
  };

  const handleBeginWithAction = () => {
    document.getElementById('action-console-input')?.focus();
  };

  const handlePlayTurnAudio = (turnNumber: number, segmentIndex?: number) => {
    if (!activeGameID) return;
    if (segmentIndex === undefined) {
      APIClient.playTurnAudio(activeGameID, turnNumber).catch(console.error);
    } else {
      APIClient.playSegmentAudio(activeGameID, turnNumber, segmentIndex).catch(console.error);
    }
  };

  const handleStopAudio = () => {
    APIClient.stopAudio().catch(console.error);
  };

  const handleSaveEntity = async (entityId: string, markdown: string) => {
    if (!client) return;
    await client.saveEntity(entityId, markdown);
    const updated = await client.getEntity(entityId);
    setSelectedEntity(updated);
    // A saved note can change its own type, links, or name, so the graph and the
    // codex listing follow it rather than going stale.
    refreshCorpus();
  };

  const handleMergeEntity = async (sourceID: string, intoID: string) => {
    if (!client || !activeGameID) return;

    const sourceName = entities.find((candidate) => candidate.id === sourceID)?.name ?? sourceID;
    const targetName = entities.find((candidate) => candidate.id === intoID)?.name ?? intoID;
    const confirmed = window.confirm(
      `Merge "${sourceName}" into "${targetName}"? Its prose, tags, aliases, and turn history move across, and the note is removed.`
    );
    if (!confirmed) return;

    try {
      const merged = await client.mergeEntity(sourceID, intoID);
      setSelectedEntity(merged);
      refreshCorpus();
    } catch (err) {
      console.error('merge failed:', err);
    }
  };

  const handleCorrect = (note: string) => {
    void handleActionSubmit('GM', `/gm ${note}`);
  };

  const handleAddress = (turnNumber: number) => {
    client?.addressFinding(turnNumber, 'continuity').catch(console.error);
    setAddressed((prev) => new Set(prev).add(turnNumber));
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
                onClick={() => setIsSettingsOpen(true)}
                className={`flex items-center gap-1.5 text-xs font-cinzel px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
                  isSettingsOpen ? 'bg-amber-600 text-stone-950 font-bold shadow-md' : 'text-stone-300 hover:text-white hover:bg-white/10'
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
              {chronicle.length === 0 ? (
                gameState ? (
                  streamedProse ? (
                    <div className="flex-1 overflow-y-auto px-8 py-6">
                      <TurnSegments
                        segments={[{ kind: 'narration', text: streamedProse }]}
                        fallback={streamedProse}
                        onEntityClick={handleOpenWikilink}
                      />
                    </div>
                  ) : (
                    <ProloguePanel
                      key={activeGameID ?? 'prologue'}
                      gameName={gameState.game_name || activeGameID || ''}
                      playerName={gameState.player?.name}
                      initialPrompt={gameState.opening_prompt || ''}
                      busy={turnInFlight}
                      onBeginStory={handleBeginStory}
                      onBeginWithAction={handleBeginWithAction}
                    />
                  )
                ) : (
                  <div className="flex-1 flex items-center justify-center text-stone-500 font-mono text-sm animate-pulse">
                    Opening the chronicle...
                  </div>
                )
              ) : (
                <>
                  <ChronicleView
                    turns={chronicle}
                    onWikilinkClick={handleOpenWikilink}
                    // With application playback the browser must stay silent, so
                    // it never competes with the narrator or hits autoplay limits.
                    autoPlay={serverAudio ? false : config?.media.tts.auto_play ?? false}
                    volume={config?.media.tts.master_volume ?? 1}
                    serverPlayback={serverAudio}
                    onPlayTurnAudio={handlePlayTurnAudio}
                    onStopAudio={handleStopAudio}
                    onCorrect={handleCorrect}
                    addressedTurns={addressed}
                    onAddress={handleAddress}
                  />
                  {streamedProse && (
                    <div className="p-4 border-t border-white/5 bg-black/20">
                      <TurnSegments
                        segments={[{ kind: 'narration', text: streamedProse }]}
                        fallback={streamedProse}
                        onEntityClick={handleOpenWikilink}
                      />
                    </div>
                  )}
                </>
              )}
              <ActionConsole
                onSubmit={handleActionSubmit}
                streaming={turnInFlight}
                onStop={handleStopTurn}
                sttType={config?.media.stt?.type}
              />
            </div>
          </main>

          {/* Flyout Drawer Modal */}
          <Drawers
            isOpen={activeDrawer !== null}
            onClose={() => setActiveDrawer(null)}
            size={activeDrawer === 'codex' || activeDrawer === 'graph' ? 'xl' : 'md'}
            title={
              activeDrawer === 'character' ? 'Character Sheet' :
              activeDrawer === 'graph' ? 'Lore Graph' :
              activeDrawer === 'codex' ? 'Codex Markdown Editor' : 'Living World Arcs & Clocks'
            }
          >
            {activeDrawer === 'character' && <CharacterSheetDrawer player={gameState?.player} />}
            {activeDrawer === 'graph' && <GraphDrawer data={graph || undefined} onSelectNode={handleOpenWikilink} />}
            {activeDrawer === 'codex' && (
              <CodexDrawer
                entity={selectedEntity || undefined}
                entities={entities}
                onSelect={handleOpenWikilink}
                onSave={handleSaveEntity}
                onMerge={handleMergeEntity}
              />
            )}
            {activeDrawer === 'world' && (
              <LivingWorldDrawer
                state={gameState || undefined}
                recap={recap || undefined}
                idleTurns={config?.agents.thread_idle_turns ?? 10}
                onRefreshRecap={() => {
                  client?.getRecap().then(setRecap).catch(console.error);
                }}
              />
            )}
          </Drawers>

          {/* Global Settings Modal Dialog */}
          {isSettingsOpen && (
            <div className="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-6 bg-black/80 backdrop-blur-sm animate-fade-in">
              <div className="relative w-full max-w-4xl max-h-[88vh] bg-stone-900/95 border border-amber-500/30 rounded-2xl shadow-2xl flex flex-col overflow-hidden">
                <div className="flex items-center justify-between px-6 py-4 border-b border-stone-800 shrink-0">
                  <div className="flex items-center gap-2">
                    <Settings className="w-5 h-5 text-amber-400" />
                    <h2 className="font-cinzel text-lg font-bold text-amber-400">Global Configuration</h2>
                  </div>
                  <button
                    onClick={() => setIsSettingsOpen(false)}
                    className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors cursor-pointer"
                  >
                    <X className="w-5 h-5" />
                  </button>
                </div>
                <div className="flex-1 min-h-0 overflow-y-auto p-6">
                  <SettingsStudio
                    onSaved={() =>
                      APIClient.getSettings()
                        .then((res) => setConfig(res.config))
                        .catch(console.error)
                    }
                  />
                </div>
              </div>
            </div>
          )}

          {/* Full-Screen Visual Novel Story Theater */}
          <StoryTheater
            turns={chronicle}
            isOpen={isTheaterOpen}
            onClose={() => setIsTheaterOpen(false)}
            autoPlay={serverAudio ? false : config?.media.tts.auto_play ?? false}
            volume={config?.media.tts.master_volume ?? 1}
          />
        </>
      )}
    </div>
  );
};
