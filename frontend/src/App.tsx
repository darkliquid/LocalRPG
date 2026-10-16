import React, { Suspense, lazy, useState } from 'react';
import { FailuresRegion } from './context/FailuresContext';
import { CampaignProvider, useCampaign } from './context/CampaignContext';
import { useSettings } from './context/SettingsContext';
import { CampaignScreen } from './components/CampaignScreen';
import { LauncherHub } from './components/LauncherHub';
import { MenuBar } from './components/MenuBar';
import { AboutModal } from './components/AboutModal';
import { genreGradient } from './lib/genre';

// Everything the shell may need once a dialog is opened, so it stays out of the
// first bundle. The chronicle and the launcher are the first paint.
const DocsModal = lazy(() => import('./components/DocsModal').then((m) => ({ default: m.DocsModal })));
const ContentStudio = lazy(() => import('./components/ContentStudio'));

// App is the shell: it chooses between the launcher and a campaign and owns the
// chrome that sits above both. A campaign's data, its turn stream and its playback
// belong to CampaignProvider, which is why it is mounted here, above the component
// that chooses one.
export const App: React.FC = () => (
  <CampaignProvider>
    <AppShell />
  </CampaignProvider>
);

function AppShell() {
  const { activeGameID, handleSelectGame, activeBgImage, gameState } = useCampaign();
  const { settings } = useSettings();
  // The help reader, the content studio and the About dialog sit above the launcher
  // and a campaign alike, so their flags are the shell's.
  const [isDocsOpen, setIsDocsOpen] = useState(false);
  const [docsArticleID, setDocsArticleID] = useState<string | undefined>(undefined);
  const [isContentStudioOpen, setIsContentStudioOpen] = useState(false);
  const [isAboutOpen, setIsAboutOpen] = useState(false);

  const openDocs = (articleID?: string) => {
    setDocsArticleID(articleID);
    setIsDocsOpen(true);
  };

  return (
    <div className="relative flex flex-col h-screen overflow-hidden text-stone-200">
      <FailuresRegion />
      {/* Full-window atmospheric background layer */}
      <div
        id="app-bg"
        key={activeBgImage || 'default'}
        className="fixed inset-0 bg-cover bg-center bg-no-repeat animate-bg-fade-in transition-all duration-700 pointer-events-none"
        style={{
          backgroundImage: activeBgImage ? `url(${activeBgImage})` : genreGradient(gameState?.genre),
          backgroundColor: '#0c0a09',
        }}
      />
      <div className="fixed inset-0 bg-gradient-to-b from-black/60 via-black/40 to-black/70 pointer-events-none" />

      <div key={activeGameID ? 'play' : 'launcher'} className="relative flex-1 flex flex-col min-h-0 anim-fade-in">
        {!activeGameID ? (
          <LauncherHub onSelectGame={handleSelectGame} onOpenDocs={openDocs} />
        ) : (
          <CampaignScreen onOpenDocs={openDocs} docsOpen={isDocsOpen} />
        )}
      </div>

      {/* The application menu stays out of the way until Alt reveals it. */}
      <MenuBar
        onOpenDocs={() => openDocs()}
        onOpenAbout={() => setIsAboutOpen(true)}
        onOpenContentStudio={() => setIsContentStudioOpen(true)}
      />

      <Suspense fallback={null}>
        <ContentStudio
          isOpen={isContentStudioOpen}
          onClose={() => setIsContentStudioOpen(false)}
          gameID={activeGameID ?? ''}
        />
      </Suspense>

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

      <AboutModal
        isOpen={isAboutOpen}
        onClose={() => setIsAboutOpen(false)}
        onOpenDocs={() => openDocs()}
        version={settings?.app_version}
      />
    </div>
  );
}
