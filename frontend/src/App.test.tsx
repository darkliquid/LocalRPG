import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { App } from './App';
import { APIClient } from './api/client';
import { SettingsProvider } from './context/SettingsContext';

vi.mock('./components/LauncherHub', () => ({
  LauncherHub: ({
    onSelectGame,
    onOpenDocs,
  }: {
    onSelectGame: (id: string) => void;
    onOpenDocs: (id?: string) => void;
  }) => (
    <div data-testid="launcher-hub">
      <button onClick={() => onSelectGame('test-game')}>Select Game</button>
      <button onClick={() => onOpenDocs('test-article')}>Launcher Docs</button>
    </div>
  ),
}));

vi.mock('./components/CampaignScreen', () => ({
  CampaignScreen: ({ onOpenDocs, docsOpen }: { onOpenDocs: (id?: string) => void; docsOpen: boolean }) => (
    <div data-testid="campaign-screen">
      <button onClick={() => onOpenDocs('campaign-article')}>Campaign Docs</button>
      <span>DocsOpen: {String(docsOpen)}</span>
    </div>
  ),
}));

vi.mock('./components/MenuBar', () => ({
  MenuBar: ({
    onOpenDocs,
    onOpenAbout,
    onOpenContentStudio,
  }: {
    onOpenDocs: () => void;
    onOpenAbout: () => void;
    onOpenContentStudio: () => void;
  }) => (
    <div data-testid="menu-bar">
      <button onClick={onOpenDocs}>Menu Docs</button>
      <button onClick={onOpenAbout}>Menu About</button>
      <button onClick={onOpenContentStudio}>Menu Studio</button>
    </div>
  ),
}));

vi.mock('./components/AboutModal', () => ({
  AboutModal: ({
    isOpen,
    onClose,
    onOpenDocs,
    version,
  }: {
    isOpen: boolean;
    onClose: () => void;
    onOpenDocs: () => void;
    version?: string;
  }) =>
    isOpen ? (
      <div data-testid="about-modal">
        <span>Version: {version}</span>
        <button onClick={onClose}>Close About</button>
        <button onClick={onOpenDocs}>About Docs</button>
      </div>
    ) : null,
}));

vi.mock('./components/DocsModal', () => ({
  DocsModal: ({
    isOpen,
    onClose,
    initialArticleID,
  }: {
    isOpen: boolean;
    onClose: () => void;
    initialArticleID?: string;
  }) =>
    isOpen ? (
      <div data-testid="docs-modal">
        <span>Article: {initialArticleID}</span>
        <button onClick={onClose}>Close Docs</button>
      </div>
    ) : null,
}));

vi.mock('./components/ContentStudio', () => ({
  default: ({ isOpen, onClose, gameID }: { isOpen: boolean; onClose: () => void; gameID?: string }) =>
    isOpen ? (
      <div data-testid="content-studio">
        <span>Studio Game: {gameID}</span>
        <button onClick={onClose}>Close Studio</button>
      </div>
    ) : null,
}));

describe('App', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(APIClient, 'listGames').mockResolvedValue([
      {
        id: 'test-game',
        name: 'Test Game',
        system_id: 'dnd5e',
        world_id: 'world1',
        player_name: 'Player',
        turn_count: 1,
        last_played: '',
        thumbnail_url: '',
      },
    ]);
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
      config: {
        version: '1',
        paths: { systems: '', worlds: '', games: '', cache: '' },
        agents: { default_role: 'gm', roles: {} },
        media: {
          tts: { type: 'disabled', auto_play: false, master_volume: 1 },
          stt: { type: 'disabled' },
          image: { type: 'disabled', auto_generate: false },
        },
        preferences: { streaming: false, typing_speed_ms: 0, cinematic_effects: false, font_scale: 'medium' },
      },
      config_file_path: '/tmp/test.yaml',
      is_local_override: false,
      app_version: '0.5.0-test',
    });
    vi.spyOn(APIClient.prototype, 'getGameState').mockResolvedValue({ game_name: 'Test Game' } as never);
    vi.spyOn(APIClient.prototype, 'getChronicle').mockResolvedValue([]);
    vi.spyOn(APIClient.prototype, 'getFindings').mockResolvedValue({ addressed: [] });
    vi.spyOn(APIClient.prototype, 'getGraph').mockResolvedValue({ nodes: [], links: [] });
    vi.spyOn(APIClient.prototype, 'listEntities').mockResolvedValue([]);
    vi.spyOn(APIClient.prototype, 'getRecap').mockResolvedValue(null as never);
    vi.spyOn(APIClient, 'audioStatus').mockResolvedValue({ available: false, playing: false, turn: 0, segment: 0 });
    vi.spyOn(APIClient, 'getLimits').mockResolvedValue({ blocks: [] });
  });

  it('renders LauncherHub by default when no game is active', async () => {
    render(
      <SettingsProvider>
        <App />
      </SettingsProvider>,
    );

    expect(screen.getByTestId('launcher-hub')).toBeInTheDocument();
    expect(screen.queryByTestId('campaign-screen')).not.toBeInTheDocument();
    expect(document.getElementById('app-bg')).toBeInTheDocument();
  });

  it('opens and closes AboutModal from MenuBar', async () => {
    render(
      <SettingsProvider>
        <App />
      </SettingsProvider>,
    );

    expect(screen.queryByTestId('about-modal')).not.toBeInTheDocument();

    fireEvent.click(screen.getByText('Menu About'));
    expect(screen.getByTestId('about-modal')).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText(/Version: 0.5.0-test/)).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Close About'));
    expect(screen.queryByTestId('about-modal')).not.toBeInTheDocument();
  });

  it('opens DocsModal with article when triggered from LauncherHub and closes it', async () => {
    render(
      <SettingsProvider>
        <App />
      </SettingsProvider>,
    );

    expect(screen.queryByTestId('docs-modal')).not.toBeInTheDocument();

    fireEvent.click(screen.getByText('Launcher Docs'));
    await waitFor(() => {
      expect(screen.getByTestId('docs-modal')).toBeInTheDocument();
    });
    expect(screen.getByText('Article: test-article')).toBeInTheDocument();

    fireEvent.click(screen.getByText('Close Docs'));
    expect(screen.queryByTestId('docs-modal')).not.toBeInTheDocument();
  });

  it('opens and closes ContentStudio from MenuBar', async () => {
    render(
      <SettingsProvider>
        <App />
      </SettingsProvider>,
    );

    expect(screen.queryByTestId('content-studio')).not.toBeInTheDocument();

    fireEvent.click(screen.getByText('Menu Studio'));
    await waitFor(() => {
      expect(screen.getByTestId('content-studio')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Close Studio'));
    expect(screen.queryByTestId('content-studio')).not.toBeInTheDocument();
  });

  it('switches to CampaignScreen when game is selected', async () => {
    render(
      <SettingsProvider>
        <App />
      </SettingsProvider>,
    );

    expect(screen.getByTestId('launcher-hub')).toBeInTheDocument();

    fireEvent.click(screen.getByText('Select Game'));

    await waitFor(() => {
      expect(screen.getByTestId('campaign-screen')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('launcher-hub')).not.toBeInTheDocument();
  });
});
