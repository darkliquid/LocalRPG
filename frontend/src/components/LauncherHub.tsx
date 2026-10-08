import React, { useState, useEffect, useCallback, Suspense, lazy, useRef } from 'react';
import { APIClient } from '../api/client';
import { GameSummary, SystemInfo, WorldInfo, CreateGameRequest, ContentManifestInfo } from '../types';
import { LauncherDock } from './launcher/LauncherDock';
import { WorldFlyout } from './launcher/WorldFlyout';
import { WorldGallery } from './launcher/WorldGallery';
import { CampaignGallery } from './launcher/CampaignGallery';
import { CampaignHeroStage } from './launcher/CampaignHeroStage';
import { NewCampaignModal } from './launcher/NewCampaignModal';
import { CampaignSettingsModal } from './launcher/CampaignSettingsModal';
import { ContentImportDialog } from './ContentImportDialog';
import { RegistryModal } from './RegistryModal';
import { inspectPackageFile } from '../lib/packageInspect';
import { ArrowLeft, X } from 'lucide-react';
import { useMountTransition } from '../hooks/useMountTransition';

// The launcher's overlays and the docs reader load on demand, so neither the
// studios, the settings editor, nor the Markdown pipeline sits in the first
// bundle.
const DocsModal = lazy(() => import('./DocsModal').then((m) => ({ default: m.DocsModal })));
const WorldsStudio = lazy(() => import('./WorldsStudio').then((m) => ({ default: m.WorldsStudio })));
const SystemsStudio = lazy(() => import('./SystemsStudio').then((m) => ({ default: m.SystemsStudio })));
const SettingsStudio = lazy(() => import('./SettingsStudio').then((m) => ({ default: m.SettingsStudio })));

export function isWorkingGame(
  game: GameSummary,
  worldList: WorldInfo[],
  systemList: SystemInfo[]
): boolean {
  if (!game || !game.id || !game.world_id || !game.system_id) {
    return false;
  }
  if (worldList.length > 0 && !worldList.some((w) => w.id === game.world_id)) {
    return false;
  }
  if (systemList.length > 0 && !systemList.some((s) => s.id === game.system_id)) {
    return false;
  }
  return true;
}

interface LauncherHubProps {
  onSelectGame: (gameId: string) => void;
  onOpenDocs?: (articleID?: string) => void;
}

export const LauncherHub: React.FC<LauncherHubProps> = ({ onSelectGame, onOpenDocs }) => {
  const [games, setGames] = useState<GameSummary[]>([]);
  const [systems, setSystems] = useState<SystemInfo[]>([]);
  const [worlds, setWorlds] = useState<WorldInfo[]>([]);
  const [selectedGameID, setSelectedGameID] = useState<string | null>(() => {
    return localStorage.getItem('localrpg_last_played_game') || localStorage.getItem('localrpg_active_game') || null;
  });

  const [isFlyoutOpen, setIsFlyoutOpen] = useState(false);
  const [isGalleryOpen, setIsGalleryOpen] = useState(false);
  const [isCampaignGalleryOpen, setIsCampaignGalleryOpen] = useState(false);
  const [creatingWorld, setCreatingWorld] = useState<WorldInfo | null>(null);
  const [isCreatingGame, setIsCreatingGame] = useState(false);

  const [settingsGameID, setSettingsGameID] = useState<string | null>(null);
  const [activeStudio, setActiveStudio] = useState<
    { studio: 'worlds'; mode: 'new' | 'browse' } | { studio: 'systems' } | null
  >(null);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [isDocsOpen, setIsDocsOpen] = useState(false);
  const [docsArticleID, setDocsArticleID] = useState<string | undefined>(undefined);
  const [isRegistryOpen, setIsRegistryOpen] = useState(false);
  const { mounted: settingsMounted, state: settingsState } = useMountTransition(isSettingsOpen, 200);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importManifest, setImportManifest] = useState<ContentManifestInfo | null>(null);
  const [isImporting, setIsImporting] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);

  const handleTriggerImport = () => {
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
      fileInputRef.current.click();
    }
  };

  const handleFileSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setImportError(null);
    try {
      const manifest = await inspectPackageFile(file);
      setImportFile(file);
      setImportManifest(manifest);
    } catch (err) {
      console.error('Failed to read package manifest:', err);
      setImportFile(file);
      setImportManifest({
        id: file.name.replace(/\.lrpgpack$/, ''),
        name: file.name.replace(/\.lrpgpack$/, ''),
        version: '1.0.0',
        type: 'world',
      });
    }
  };

  const handleConfirmImport = async (conflictMode: 'refuse' | 'rename' | 'overwrite') => {
    if (!importFile) return;
    setIsImporting(true);
    setImportError(null);
    try {
      await APIClient.importContent(importFile, conflictMode);
      setImportFile(null);
      setImportManifest(null);
      await loadData();
    } catch (err: any) {
      setImportError(err.message || 'Failed to import package');
    } finally {
      setIsImporting(false);
    }
  };

  const handleOpenDocs = (articleID?: string) => {
    if (onOpenDocs) {
      onOpenDocs(articleID);
    } else {
      setDocsArticleID(articleID);
      setIsDocsOpen(true);
    }
  };

  const loadData = useCallback(async () => {
    try {
      const [gList, sList, wList] = await Promise.all([
        APIClient.listGames().catch(() => []),
        APIClient.listSystems().catch(() => []),
        APIClient.listWorlds().catch(() => []),
      ]);
      setGames(gList);
      setSystems(sList);
      setWorlds(wList);

      setSelectedGameID((prev) => {
        const candidateID = prev || localStorage.getItem('localrpg_last_played_game') || localStorage.getItem('localrpg_active_game') || null;
        const candidate = candidateID ? gList.find((g) => g.id === candidateID) : null;
        if (candidate && isWorkingGame(candidate, wList, sList)) {
          return candidate.id;
        }
        const nextWorking = gList.find((g) => isWorkingGame(g, wList, sList));
        return nextWorking?.id || gList[0]?.id || null;
      });
    } catch (err) {
      console.error('Failed to load launcher data:', err);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const activeGame = games.find((g) => g.id === selectedGameID) || null;
  const activeWorld = worlds.find((w) => w.id === activeGame?.world_id);
  const activeSystem = systems.find((s) => s.id === activeGame?.system_id);
  const settingsGame = games.find((g) => g.id === settingsGameID) || null;

  const handleSelectWorldFromFlyout = (worldId: string) => {
    const world = worlds.find((w) => w.id === worldId);
    if (world) {
      setCreatingWorld(world);
      setIsFlyoutOpen(false);
      setIsGalleryOpen(false);
    }
  };

  const handleCreateGame = async (
    data: CreateGameRequest,
    bannerFile?: File,
    iconFile?: File
  ) => {
    setIsCreatingGame(true);
    try {
      const newGame = await APIClient.createGame(data);
      if (bannerFile) {
        await APIClient.uploadGameAsset(newGame.id, 'banner', bannerFile).catch(console.error);
      }
      if (iconFile) {
        await APIClient.uploadGameAsset(newGame.id, 'icon', iconFile).catch(console.error);
      }
      await loadData();
      setSelectedGameID(newGame.id);
      setCreatingWorld(null);
    } catch (err) {
      console.error('Failed to create campaign:', err);
    } finally {
      setIsCreatingGame(false);
    }
  };

  const handleUploadAsset = async (gameId: string, kind: 'banner' | 'icon', file: File) => {
    await APIClient.uploadGameAsset(gameId, kind, file);
    await loadData();
  };

  const handleGenerateAsset = async (gameId: string, kind: 'banner' | 'icon') => {
    await APIClient.generateGameAsset(gameId, kind);
    await loadData();
  };

  const handleUseWorldArtwork = async (gameId: string, kind: 'banner' | 'icon') => {
    await APIClient.deleteGameAsset(gameId, kind);
    await loadData();
  };

  const handleRestartGame = async (gameId: string) => {
    await APIClient.restartGame(gameId);
    await loadData();
  };

  const handleDeleteGame = async (gameId: string) => {
    await APIClient.deleteGame(gameId);
    await loadData();
  };

  const handleDeleteWorld = async (worldId: string, force = false) => {
    await APIClient.deleteWorld(worldId, force);
    await loadData();
  };

  // Full-Window Overlay: Worlds Studio
  if (activeStudio?.studio === 'worlds') {
    return (
      <div className="relative w-full h-full flex flex-col bg-stone-950 text-stone-200 anim-fade-in">
        <header className="h-14 px-6 border-b border-white/10 flex items-center justify-between bg-stone-900/60 backdrop-blur-xl">
          <button
            onClick={() => {
              setActiveStudio(null);
              loadData();
            }}
            className="flex items-center gap-2 px-3.5 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-300 hover:text-white transition-all cursor-pointer"
          >
            <ArrowLeft className="w-4 h-4" />
            <span>Back to Launcher</span>
          </button>
          <div className="text-sm font-sans font-bold text-white tracking-wide">
            Worlds Studio
          </div>
          <div className="w-24" />
        </header>
        <div className="flex-1 overflow-hidden">
          <WorldsStudio onWorldSaved={loadData} startMode={activeStudio.mode} />
        </div>
      </div>
    );
  }

  // Full-Window Overlay: Systems Studio
  if (activeStudio?.studio === 'systems') {
    return (
      <div className="relative w-full h-full flex flex-col bg-stone-950 text-stone-200 anim-fade-in">
        <header className="h-14 px-6 border-b border-white/10 flex items-center justify-between bg-stone-900/60 backdrop-blur-xl">
          <button
            onClick={() => {
              setActiveStudio(null);
              loadData();
            }}
            className="flex items-center gap-2 px-3.5 py-1.5 rounded-xl bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 text-xs font-sans font-semibold text-stone-300 hover:text-white transition-all cursor-pointer"
          >
            <ArrowLeft className="w-4 h-4" />
            <span>Back to Launcher</span>
          </button>
          <div className="text-sm font-sans font-bold text-white tracking-wide">
            Systems Studio
          </div>
          <div className="w-24" />
        </header>
        <div className="flex-1 overflow-hidden">
          <SystemsStudio onSystemSaved={loadData} />
        </div>
      </div>
    );
  }

  return (
    <div className="relative w-full h-full flex overflow-hidden bg-stone-950 text-stone-200 font-sans anim-fade-in">
      {/* Left Navigation Dock */}
      <LauncherDock
        games={games}
        activeGameID={selectedGameID}
        onSelectGame={(id) => {
          setSelectedGameID(id);
          setIsFlyoutOpen(false);
          setIsCampaignGalleryOpen(false);
        }}
        isFlyoutOpen={isFlyoutOpen}
        onToggleFlyout={() => {
          setIsFlyoutOpen((prev) => !prev);
          setIsCampaignGalleryOpen(false);
        }}
        isCampaignGalleryOpen={isCampaignGalleryOpen}
        onToggleCampaignGallery={() => {
          setIsCampaignGalleryOpen((prev) => !prev);
          setIsFlyoutOpen(false);
        }}
        onOpenWorldsStudio={() => setActiveStudio({ studio: 'worlds', mode: 'browse' })}
        onOpenSystemsStudio={() => setActiveStudio({ studio: 'systems' })}
        onOpenSettings={() => setIsSettingsOpen(true)}
        onOpenDocs={() => handleOpenDocs()}
        onImportPackage={handleTriggerImport}
        onOpenRegistry={() => setIsRegistryOpen(true)}
      />

      {/* Horizontal World Flyout */}
      <WorldFlyout
        isOpen={isFlyoutOpen}
        worlds={worlds}
        onSelectWorld={handleSelectWorldFromFlyout}
        onCreateWorld={() => {
          setIsFlyoutOpen(false);
          setActiveStudio({ studio: 'worlds', mode: 'new' });
        }}
        onExpand={() => setIsGalleryOpen(true)}
      />

      {/* Full-Page World Gallery */}
      <WorldGallery
        isOpen={isGalleryOpen}
        worlds={worlds}
        onSelectWorld={handleSelectWorldFromFlyout}
        onCreateWorld={() => {
          setIsGalleryOpen(false);
          setActiveStudio({ studio: 'worlds', mode: 'new' });
        }}
        onDeleteWorld={handleDeleteWorld}
        onClose={() => setIsGalleryOpen(false)}
      />

      {/* Full-Page Campaign Gallery */}
      <CampaignGallery
        isOpen={isCampaignGalleryOpen}
        games={games}
        worlds={worlds}
        systems={systems}
        onPlayGame={(id) => {
          setIsCampaignGalleryOpen(false);
          onSelectGame(id);
        }}
        onOpenSettings={(id) => {
          setIsCampaignGalleryOpen(false);
          setSettingsGameID(id);
        }}
        onCreateCampaign={() => {
          setIsCampaignGalleryOpen(false);
          setIsFlyoutOpen(true);
        }}
        onClose={() => setIsCampaignGalleryOpen(false)}
      />

      {/* Main Campaign Hero Presentation */}
      <CampaignHeroStage
        game={activeGame}
        worldName={activeWorld?.name}
        systemName={activeSystem?.name}
        hasGames={games.length > 0}
        hasWorlds={worlds.length > 0}
        onPlay={onSelectGame}
        onOpenCampaignSettings={(id) => setSettingsGameID(id)}
        onCreateWorld={() => setActiveStudio({ studio: 'worlds', mode: 'new' })}
        onBrowseSystems={() => setActiveStudio({ studio: 'systems' })}
        onOpenDocs={handleOpenDocs}
      />

      {/* New Campaign Modal */}
      <NewCampaignModal
        isOpen={Boolean(creatingWorld)}
        world={creatingWorld}
        systems={systems}
        onClose={() => setCreatingWorld(null)}
        onCreateGame={handleCreateGame}
        isSubmitting={isCreatingGame}
      />

      {/* Campaign Settings Modal */}
      <CampaignSettingsModal
        isOpen={Boolean(settingsGameID)}
        game={settingsGame}
        onClose={() => setSettingsGameID(null)}
        onUploadAsset={handleUploadAsset}
        onGenerateAsset={handleGenerateAsset}
        onUseWorldArtwork={handleUseWorldArtwork}
        onRestartGame={handleRestartGame}
        onDeleteGame={handleDeleteGame}
      />

      {/* Global Settings Modal */}
      {settingsMounted && (
        <div
          data-state={settingsState}
          className={`fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-md ${
            settingsState === 'enter' ? 'anim-fade-in' : 'anim-fade-out pointer-events-none'
          }`}
          style={{ '--anim-dur': '200ms' } as React.CSSProperties}
        >
          <div
            className={`relative w-full max-w-4xl bg-stone-900 border border-white/15 rounded-3xl overflow-hidden shadow-2xl flex flex-col max-h-[90vh] ${
              settingsState === 'enter' ? 'anim-scale-in' : 'anim-scale-out'
            }`}
          >
            <div className="p-4 px-6 border-b border-white/10 flex items-center justify-between bg-stone-950/60">
              <h2 className="text-base font-sans font-bold text-white">Global Settings</h2>
              <button
                onClick={() => setIsSettingsOpen(false)}
                className="w-8 h-8 rounded-full bg-white/[0.05] hover:bg-white/[0.1] border border-white/10 flex items-center justify-center text-stone-400 hover:text-white transition-all cursor-pointer"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto p-6">
              <Suspense fallback={null}>
                <SettingsStudio isCompact={false} />
              </Suspense>
            </div>
          </div>
        </div>
      )}

      {/* Built-in Help and Documentation Modal */}
      <Suspense fallback={null}>
        <DocsModal
          isOpen={isDocsOpen}
          initialArticleID={docsArticleID}
          onClose={() => {
            setIsDocsOpen(false);
            setDocsArticleID(undefined);
          }}
        />
      </Suspense>

      {/* Hidden file input for content import */}
      <input
        ref={fileInputRef}
        type="file"
        accept=".lrpgworld,.lrpgsystem,.lrpgpack"
        className="hidden"
        onChange={handleFileSelect}
      />

      {/* Content Import Dialog */}
      <ContentImportDialog
        manifest={importManifest || undefined}
        onConfirm={handleConfirmImport}
        onCancel={() => {
          setImportFile(null);
          setImportManifest(null);
          setImportError(null);
        }}
        loading={isImporting}
        error={importError}
      />

      {/* Content Package Registry Modal */}
      <RegistryModal
        isOpen={isRegistryOpen}
        onClose={() => setIsRegistryOpen(false)}
        onInstalled={loadData}
      />
    </div>
  );
};
