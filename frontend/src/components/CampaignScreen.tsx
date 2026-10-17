import { useState, useEffect, useCallback, Suspense, lazy } from 'react';
import { APIClient } from '../api/client';
import { useFailures } from '../context/FailuresContext';
import { useCampaign } from '../context/CampaignContext';
import { MechanicsStrip } from './MechanicsStrip';
import { ChronicleView } from './ChronicleView';
import { TurnSegments } from './TurnSegments';
import { ActionConsole } from './ActionConsole';
import { PendingCheckCard } from './PendingCheckCard';
import { Drawers } from './Drawers';
import { CharacterSheetDrawer } from './CharacterSheetDrawer';
import { GraphDrawer } from './GraphDrawer';
import { CodexDrawer } from './CodexDrawer';
import { LivingWorldDrawer } from './LivingWorldDrawer';
import { ContextDrawer } from './ContextDrawer';
import { ProloguePanel } from './ProloguePanel';
import { AddEntityModal } from './AddEntityModal';
import { Modal } from './Modal';
import { LiveStatus } from './ui/LiveStatus';
import { NewEntityWizard } from './NewEntityWizard';
import { ModelDownloadModal } from './ModelDownloadModal';
import { LimitChip } from './LimitChip';
import {
  User,
  Network,
  BookOpen,
  Clock,
  Film,
  Compass,
  Settings,
  X,
  Layers,
  AlertTriangle,
  HelpCircle,
  Download,
} from 'lucide-react';
import { CinematicOverlay } from './CinematicOverlay';

// Everything below the campaign shell is only needed once its overlay is
// opened, so it loads on demand instead of in the first bundle. The chronicle,
// its drawers, and the launcher stay eager because they are the first paint.
const SettingsStudio = lazy(() => import('./SettingsStudio').then((m) => ({ default: m.SettingsStudio })));
const StoryTheater = lazy(() => import('./StoryTheater').then((m) => ({ default: m.StoryTheater })));
const ExportModal = lazy(() => import('./ExportModal').then((m) => ({ default: m.ExportModal })));

// CampaignScreen is a campaign's shell: the chronicle, the action console, the
// drawers and the modals that belong to the campaign rather than to the app. The
// campaign's data, its turn stream and its playback come from the provider; what is
// left here is the chrome this screen opens and the flows that drive it.
export interface CampaignScreenProps {
  /** onOpenDocs is the shell's, because the help reader sits above a campaign. */
  onOpenDocs: (articleID?: string) => void;
  /** docsOpen highlights the header's help button while the reader is up. */
  docsOpen: boolean;
}

export function CampaignScreen({ onOpenDocs: openDocs, docsOpen }: CampaignScreenProps) {
  const { report } = useFailures();

  // The screen owns the chrome it opens: the drawers, the theatre, the export and
  // the entity modals are this screen's, and nothing below it needs to know.
  const [activeDrawer, setActiveDrawer] = useState<string | null>(null);
  const [isTheaterOpen, setIsTheaterOpen] = useState(false);
  const [isExportOpen, setIsExportOpen] = useState(false);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [modalEntity, setModalEntity] = useState<{ name: string; turnNumber: number } | null>(null);
  const [wizardEntity, setWizardEntity] = useState<{ name: string; turnNumber: number } | null>(null);
  const [advancementSpending, setAdvancementSpending] = useState(false);
  const [advancementError, setAdvancementError] = useState<string | null>(null);
  const [dismissedContentWarnings, setDismissedContentWarnings] = useState(false);

  const {
    activeGameID,
    activeRateLimitSeconds,

    audioProgress,
    campaignError,
    campaignStatus,
    characterPortraits,
    chronicle,
    client,
    config,
    entities,
    fundsError,
    gameState,
    graph,
    handleActionSubmit,
    handleBeginStory,
    handleBeginWithAction,
    handleMergeEntity,
    handleRenegotiate,
    handleResolveCheck,
    handleSaveEntity,
    handleStopAudio,
    handleStopTurn,
    isRateLimited,
    lastTurnChecks,
    limits,
    missingModel,
    pendingAction,
    pendingCheck,
    pendingTurnNumber,
    recap,

    sceneImagePending,
    segmentAudioProgress,
    selectedEntity,
    streamedSegments,
    toolActivity,
    turnError,
    turnInFlight,
    turnStatus,
    addMissingEntity,
    addVoiceProfile,
    clearSelectedEntity,
    dismissFundsError,
    dismissModelPrompt,
    dismissTurnError,
    generateTurnImage,
    leaveCampaign,
    openEntity,
    reloadEntities,
    reloadRecap,
    saveWizardEntity,
    spendAdvancement,
  } = useCampaign();

  // The dismissals belong to the campaign, not to this screen, so a new campaign
  // starts with its warnings shown again.
  useEffect(() => {
    setDismissedContentWarnings(false);
  }, [activeGameID]);

  const handleSpendAdvancement = useCallback(
    async (unlockID: string) => {
      setAdvancementSpending(true);
      setAdvancementError(null);
      try {
        await spendAdvancement(unlockID);
      } catch (err) {
        setAdvancementError(err instanceof Error ? err.message : String(err));
      } finally {
        setAdvancementSpending(false);
      }
    },
    [spendAdvancement],
  );

  const handleReturnToLauncher = () => {
    leaveCampaign();
    setActiveDrawer(null);
    setIsSettingsOpen(false);
  };

  const handleOpenWikilink = async (entityId: string) => {
    try {
      await openEntity(entityId);
      setActiveDrawer('codex');
    } catch (err) {
      report({ text: 'Could not open the entity.', detail: err instanceof Error ? err.message : String(err) });
    }
  };

  const handleGenerateImage = async (turnNumber: number) => {
    try {
      await generateTurnImage(turnNumber);
    } catch (err) {
      report({
        text: 'Could not generate the illustration.',
        detail: err instanceof Error ? err.message : String(err),
      });
    }
  };

  const handleCorrect = (note: string, turnNumber?: number) => {
    const entityName = parseMissingEntityName(note);
    if (entityName && turnNumber !== undefined) {
      setModalEntity({ name: entityName, turnNumber });
    } else {
      const input = document.getElementById('action-console-input') as HTMLInputElement | null;
      if (input) {
        input.value = `/gm ${note}`;
        input.focus();
      }
    }
  };

  const handleQuickCreateEntity = async (name: string, type: string) => {
    if (!modalEntity) return;
    try {
      await addMissingEntity(name, type, modalEntity.turnNumber);
    } catch (err) {
      report({ text: 'Could not create the entity.', detail: err instanceof Error ? err.message : String(err) });
    } finally {
      setModalEntity(null);
    }
  };

  const handleEditInCodexEntity = (name: string) => {
    if (!modalEntity) return;
    setWizardEntity({ name, turnNumber: modalEntity.turnNumber });
    setModalEntity(null);
  };

  const handleWizardCreateEntity = async ({ id, markdown }: { id: string; name: string; markdown: string }) => {
    if (!wizardEntity) return;
    try {
      await saveWizardEntity({ id, markdown, turnNumber: wizardEntity.turnNumber });
      setActiveDrawer('codex');
    } catch (err) {
      report({ text: 'Could not create the entity.', detail: err instanceof Error ? err.message : String(err) });
    } finally {
      setWizardEntity(null);
      setModalEntity(null);
    }
  };

  const parseMissingEntityName = (note: string): string | null => {
    const match = note.match(
      /"([^"]+)" (?:speaks but has no note|is named but has no note|speaks but is not a known character)/i,
    );
    return match ? match[1] : null;
  };

  return (
    <>
      {/* Floating Translucent Acrylic Header */}
      <header className="relative z-10 mx-6 mt-4 mb-2 h-14 bg-glass rounded-2xl px-6 flex items-center justify-between shadow-2xl">
        <div className="flex items-center gap-4">
          <button
            onClick={handleReturnToLauncher}
            className="flex items-center gap-1.5 text-xs font-sans font-semibold px-3 py-1.5 rounded-xl transition-all cursor-pointer bg-white/[0.05] hover:bg-white/[0.1] text-stone-200 border border-white/10"
            title="Switch Campaign / Return to Hub"
          >
            <Compass className="w-3.5 h-3.5" />
            <span>Back</span>
          </button>

          <div className="flex items-center gap-2.5">
            <div className="w-2 h-2 rounded-full bg-purple-500 shadow-[0_0_8px_rgba(168,85,247,0.8)]" />
            <h1 className="font-sans text-base md:text-lg font-bold text-white tracking-tight">
              {gameState?.game_name || activeGameID}
            </h1>
            {gameState?.mechanics_engagement && (
              <span className="text-xs font-sans uppercase tracking-wider px-2 py-0.5 rounded-full border border-white/10 bg-white/[0.05] text-stone-400">
                mechanics: {gameState.mechanics_engagement}
              </span>
            )}
            {limits.map((block, idx) => (
              <LimitChip key={idx} block={block} />
            ))}
          </div>
        </div>

        {/* Floating Drawer Trigger Pills */}
        <div className="flex items-center gap-2">
          <button
            onClick={() => setActiveDrawer('character')}
            className={`relative flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              activeDrawer === 'character'
                ? 'bg-purple-600 text-white font-bold shadow-md'
                : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
          >
            <User className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Character</span>
            {(gameState?.advancement?.pending ||
              gameState?.advancement?.unlocks?.some((u) => u.affordable && u.requires_met && u.gate_open)) && (
              <span className="absolute -top-0.5 -right-0.5 w-2 h-2 rounded-full bg-purple-400 shadow-[0_0_6px_rgba(168,85,247,0.9)]" />
            )}
          </button>
          <button
            onClick={() => setActiveDrawer('graph')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              activeDrawer === 'graph'
                ? 'bg-purple-600 text-white font-bold shadow-md'
                : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
          >
            <Network className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Graph</span>
          </button>
          <button
            onClick={() => setActiveDrawer('codex')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              activeDrawer === 'codex'
                ? 'bg-purple-600 text-white font-bold shadow-md'
                : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
          >
            <BookOpen className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Codex</span>
          </button>
          <button
            onClick={() => setActiveDrawer('world')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              activeDrawer === 'world'
                ? 'bg-purple-600 text-white font-bold shadow-md'
                : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
          >
            <Clock className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">World Arcs</span>
          </button>
          <button
            onClick={() => setActiveDrawer('context')}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              activeDrawer === 'context'
                ? 'bg-purple-600 text-white font-bold shadow-md'
                : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
            title="Inspect prompt context sections and continuity working set"
          >
            <Layers className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Context</span>
          </button>
          <button
            onClick={() => setIsTheaterOpen(true)}
            className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer text-stone-300 hover:text-white hover:bg-white/10"
            title="Open Story Theater replay mode"
          >
            <Film className="w-3.5 h-3.5 text-purple-400" />
            <span className="hidden sm:inline">Theater</span>
          </button>
          <button
            onClick={() => setIsExportOpen(true)}
            className="flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer text-stone-300 hover:text-white hover:bg-white/10"
            title="Export this story as a web player or video"
          >
            <Download className="w-3.5 h-3.5 text-purple-400" />
            <span className="hidden sm:inline">Export</span>
          </button>
          <button
            onClick={() => setIsSettingsOpen(true)}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              isSettingsOpen
                ? 'bg-purple-600 text-white font-bold shadow-md'
                : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
            title="Global Settings"
          >
            <Settings className="w-3.5 h-3.5 text-purple-400" />
            <span className="hidden sm:inline">Settings</span>
          </button>
          <button
            onClick={() => openDocs()}
            className={`flex items-center gap-1.5 text-xs font-sans px-3 py-1.5 rounded-xl transition-all cursor-pointer ${
              docsOpen
                ? 'bg-purple-600 text-white font-bold shadow-md'
                : 'text-stone-300 hover:text-white hover:bg-white/10'
            }`}
            title="Help & Documentation"
          >
            <HelpCircle className="w-3.5 h-3.5 text-purple-400" />
            <span className="hidden sm:inline">Docs</span>
          </button>
        </div>
      </header>

      {/* Main Floating Translucent Chronicle & Action Container */}
      <main className="relative z-10 flex-1 overflow-hidden mx-6 mb-4 flex flex-col">
        <div
          aria-busy={turnInFlight}
          className="flex-1 bg-glass-card rounded-2xl flex flex-col overflow-hidden shadow-2xl"
        >
          <LiveStatus message={turnStatus} />
          {chronicle.length === 0 ? (
            gameState ? (
              streamedSegments.length > 0 ? (
                <div className="flex-1 overflow-y-auto px-8 py-6">
                  <TurnSegments
                    segments={streamedSegments}
                    onEntityClick={handleOpenWikilink}
                    displayMode={config?.media.tts.speech_cues?.display_mode}
                    characterPortraits={characterPortraits}
                    segmentProgress={segmentAudioProgress}
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
            ) : campaignStatus === 'unavailable' ? (
              <div className="flex-1 flex flex-col items-center justify-center gap-3 p-6 text-center">
                <Compass className="w-8 h-8 text-purple-500/50" />
                <div className="space-y-1">
                  <p className="font-sans text-sm text-purple-300 font-bold">This campaign could not be opened</p>
                  <p className="text-xs text-stone-400 font-mono max-w-md">
                    {campaignError ?? `"${activeGameID}" is unavailable.`}
                  </p>
                </div>
                <div className="flex items-center gap-2 pt-1">
                  <button
                    onClick={handleReturnToLauncher}
                    className="px-3.5 py-1.5 rounded-lg bg-purple-600 hover:bg-purple-500 text-white font-sans font-bold text-xs shadow-md transition-all cursor-pointer"
                  >
                    Return to Campaigns
                  </button>
                  <button
                    onClick={() => window.location.reload()}
                    className="px-3.5 py-1.5 rounded-lg bg-stone-900 border border-stone-700 text-stone-300 hover:text-purple-300 hover:border-purple-500/40 font-sans text-xs transition-colors cursor-pointer"
                  >
                    Retry
                  </button>
                </div>
              </div>
            ) : (
              <div className="flex-1 flex items-center justify-center text-stone-500 font-mono text-sm animate-pulse">
                Opening the chronicle...
              </div>
            )
          ) : (
            <>
              <ChronicleView
                turns={chronicle}
                genre={gameState?.genre}
                turnInFlight={turnInFlight}
                pendingAction={pendingAction}
                sceneImagePending={sceneImagePending}
                onWikilinkClick={handleOpenWikilink}
                onCorrect={handleCorrect}
                onGenerateImage={handleGenerateImage}
              />
            </>
          )}
          {toolActivity && <div className="text-xs font-mono text-purple-400/80 px-4 pb-1">{toolActivity}</div>}
          <MechanicsStrip
            turn={{ engagement: gameState?.mechanics_engagement, checks: lastTurnChecks }}
            budget={gameState?.image_budget}
          />
          <PendingCheckCard
            pending={pendingCheck}
            busy={turnInFlight || isRateLimited}
            onRoll={() => handleResolveCheck(pendingTurnNumber, pendingCheck!.ref)}
            onManual={(entry) => handleResolveCheck(pendingTurnNumber, pendingCheck!.ref, entry)}
            onArgue={(counter) => handleRenegotiate(pendingTurnNumber, pendingCheck!.ref, counter)}
          />

          {fundsError && (
            <div
              role="alert"
              className="mx-4 mb-2 p-3 bg-red-950/80 border border-red-500/50 text-red-200 text-xs rounded-xl flex items-center justify-between shadow-lg"
            >
              <div className="flex items-center gap-2">
                <AlertTriangle className="w-4 h-4 text-red-400 shrink-0" />
                <span>
                  Provider {fundsError.provider || 'AI'} rejected the request: insufficient funds/credits. Add funds and
                  retry — no other work is blocked.
                </span>
              </div>
              <button
                onClick={dismissFundsError}
                aria-label="Dismiss"
                className="text-stone-400 hover:text-white p-1 rounded hover:bg-white/10 ml-2 cursor-pointer"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
          )}

          {gameState?.content_warnings && gameState.content_warnings.length > 0 && !dismissedContentWarnings && (
            <div
              role="status"
              className="mx-4 mb-2 p-3 bg-amber-950/80 border border-amber-500/50 text-amber-200 text-xs rounded-xl flex items-center justify-between shadow-lg"
            >
              <div className="flex items-center gap-2">
                <AlertTriangle className="w-4 h-4 text-amber-400 shrink-0" />
                <span>Content warnings: {gameState.content_warnings.join('; ')}</span>
              </div>
              <button
                onClick={() => setDismissedContentWarnings(true)}
                aria-label="Dismiss"
                className="text-stone-400 hover:text-white p-1 rounded hover:bg-white/10 ml-2 cursor-pointer"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
          )}

          {isRateLimited && (
            <div
              role="status"
              className="mx-4 mb-2 p-3 bg-amber-950/80 border border-amber-500/50 text-amber-200 text-xs rounded-xl flex items-center gap-2 shadow-lg"
            >
              <Clock className="w-4 h-4 text-amber-400 shrink-0 animate-pulse" />
              <span>Rate limit backoff active. Submitting turns paused for {activeRateLimitSeconds}s.</span>
            </div>
          )}

          <ActionConsole
            onSubmit={handleActionSubmit}
            disabled={isRateLimited}
            streaming={turnInFlight}
            onStop={handleStopTurn}
            sttType={config?.media.stt?.type}
            audioProgress={audioProgress}
            engagement={gameState?.mechanics_engagement}
          />
        </div>
      </main>

      {/* Flyout Drawer Modal */}
      <Drawers
        isOpen={activeDrawer !== null}
        onClose={() => setActiveDrawer(null)}
        size={activeDrawer === 'codex' || activeDrawer === 'graph' || activeDrawer === 'context' ? 'xl' : 'md'}
        title={
          activeDrawer === 'character'
            ? 'Character Sheet'
            : activeDrawer === 'graph'
              ? 'Lore Graph'
              : activeDrawer === 'codex'
                ? 'Codex Markdown Editor'
                : activeDrawer === 'context'
                  ? 'Turn Context & Continuity'
                  : 'Living World Arcs & Clocks'
        }
      >
        {activeDrawer === 'character' && (
          <CharacterSheetDrawer
            player={gameState?.player}
            advancement={gameState?.advancement}
            onSpend={handleSpendAdvancement}
            spending={advancementSpending}
            spendError={advancementError}
          />
        )}
        {activeDrawer === 'graph' && <GraphDrawer data={graph || undefined} onSelectNode={handleOpenWikilink} />}
        {activeDrawer === 'context' && (
          <ContextDrawer gameID={activeGameID ?? undefined} onSelectWikilink={handleOpenWikilink} />
        )}
        {activeDrawer === 'codex' && (
          <CodexDrawer
            gameID={activeGameID || undefined}
            entity={selectedEntity || undefined}
            entities={entities}
            voiceProfiles={config?.media.tts.voice_profiles ?? []}
            ttsConfig={config?.media.tts}
            onAddProfile={(profile) => {
              if (!config) return;
              const existing = config.media.tts.voice_profiles ?? [];
              if (existing.some((p) => p.id === profile.id)) return;
              const next = {
                ...config,
                media: {
                  ...config.media,
                  tts: { ...config.media.tts, voice_profiles: [...existing, profile] },
                },
              };
              addVoiceProfile(next);
              APIClient.saveSettings(next).catch(() => undefined);
            }}
            onSelect={handleOpenWikilink}
            onSave={handleSaveEntity}
            onMerge={handleMergeEntity}
            onDeleteEntity={async (id) => {
              if (!client) return;
              await client.deleteEntity(id);
              await reloadEntities();
              if (selectedEntity?.id === id) clearSelectedEntity();
            }}
          />
        )}
        {activeDrawer === 'world' && (
          <LivingWorldDrawer
            state={gameState || undefined}
            recap={recap || undefined}
            idleTurns={config?.agents.thread_idle_turns ?? 10}
            onRefreshRecap={() => {
              reloadRecap().catch((err) =>
                report({
                  text: 'Could not load the recap.',
                  detail: err instanceof Error ? err.message : String(err),
                }),
              );
            }}
          />
        )}
      </Drawers>

      {/* Global Settings Modal Dialog */}
      <Modal
        open={isSettingsOpen}
        onClose={() => setIsSettingsOpen(false)}
        title="Global Configuration"
        className="max-w-4xl"
      >
        <Suspense fallback={null}>
          <SettingsStudio activeGameID={activeGameID ?? undefined} onOpenDocs={openDocs} />
        </Suspense>
      </Modal>

      {/* Full-Screen Visual Novel Story Theater */}
      <Suspense fallback={null}>
        <StoryTheater
          turns={chronicle}
          isOpen={isTheaterOpen}
          onClose={() => {
            handleStopAudio();
            setIsTheaterOpen(false);
          }}
          onEntityClick={handleOpenWikilink}
        />
      </Suspense>

      {/* Add Entity Modal */}
      <AddEntityModal
        isOpen={modalEntity !== null}
        entityName={modalEntity?.name ?? ''}
        onQuickCreate={handleQuickCreateEntity}
        onEditInCodex={handleEditInCodexEntity}
        onClose={() => setModalEntity(null)}
      />

      {wizardEntity && (
        <NewEntityWizard
          isOpen={wizardEntity !== null}
          initialName={wizardEntity.name}
          existingIds={entities.map((candidate) => candidate.id)}
          ttsConfig={config?.media.tts}
          voiceProfiles={config?.media.tts.voice_profiles ?? []}
          onClose={() => setWizardEntity(null)}
          onOpenExisting={async (id) => {
            setWizardEntity(null);
            if (client) await openEntity(id);
            setActiveDrawer('codex');
          }}
          onConfirm={handleWizardCreateEntity}
        />
      )}

      {/* Turn Failure Banner */}
      {turnError && (
        <div
          role="alert"
          className="fixed bottom-4 left-1/2 -translate-x-1/2 z-50 max-w-xl px-4 py-3 rounded-xl bg-red-950/90 border border-red-500/40 text-red-100 text-xs font-sans shadow-2xl flex items-center gap-3"
        >
          <span className="flex-1 select-text">{turnError}</span>
          <button onClick={dismissTurnError} className="text-red-300 hover:text-white cursor-pointer">
            Dismiss
          </button>
        </div>
      )}

      {/* Model Download Modal */}
      <ModelDownloadModal
        isOpen={missingModel !== null}
        modelId={missingModel?.id ?? ''}
        modelName={missingModel?.name ?? ''}
        sizeBytes={missingModel?.sizeBytes ?? 0}
        onClose={dismissModelPrompt}
      />

      {/* Story Export Modal */}
      <Suspense fallback={null}>
        <ExportModal isOpen={isExportOpen} gameID={activeGameID} onClose={() => setIsExportOpen(false)} />
      </Suspense>

      {/* Cinematic backdrop overlays honour the preference. The layer is
              inert and sits below the modals. */}
      <CinematicOverlay enabled={config?.preferences?.cinematic_effects ?? false} />
    </>
  );
}
