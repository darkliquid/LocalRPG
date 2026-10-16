import { useEffect } from 'react';
import { render, screen } from '@testing-library/react';
import { useCampaign } from '../context/CampaignContext';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { CampaignScreen } from './CampaignScreen';
import { CampaignProvider } from '../context/CampaignContext';
import { SettingsProvider } from '../context/SettingsContext';
import { APIClient } from '../api/client';
import { Turn } from '../types';

const turn: Turn = {
  turn_number: 1,
  input_text: 'I open the door.',
  mode: 'Action',
  prose: 'The door gives onto a cold stair.',
};

function mockCampaign() {
  vi.spyOn(APIClient, 'listGames').mockResolvedValue([
    {
      id: 'campaign',
      name: 'Campaign',
      system_id: 'osric',
      world_id: 'reach',
      player_name: 'Andrew',
      turn_count: 1,
      last_played: '',
      thumbnail_url: '',
    },
  ]);
  vi.spyOn(APIClient.prototype, 'getGameState').mockResolvedValue({ game_name: 'Campaign' } as never);
  vi.spyOn(APIClient.prototype, 'getChronicle').mockResolvedValue([turn]);
  vi.spyOn(APIClient.prototype, 'getFindings').mockResolvedValue({ addressed: [] });
  vi.spyOn(APIClient.prototype, 'getGraph').mockResolvedValue({ nodes: [], links: [] });
  vi.spyOn(APIClient.prototype, 'listEntities').mockResolvedValue([]);
  vi.spyOn(APIClient.prototype, 'getRecap').mockResolvedValue(null as never);
  vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
    config: { media: { tts: { master_volume: 1 } } } as never,
    config_file_path: '',
    is_local_override: false,
  });
  vi.spyOn(APIClient, 'audioStatus').mockResolvedValue({ available: false, playing: false, turn: 0, segment: 0 });
  vi.spyOn(APIClient, 'getLimits').mockResolvedValue({ blocks: [] });
}

// A campaign has to be opened before the screen has one to show, so the harness
// opens one the way the shell does: handleSelectGame is what the launcher calls.
function Harness() {
  const { activeGameID, handleSelectGame } = useCampaign();
  useEffect(() => {
    if (!activeGameID) handleSelectGame('campaign');
  }, [activeGameID, handleSelectGame]);

  return activeGameID ? <CampaignScreen onOpenDocs={() => undefined} docsOpen={false} /> : null;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('CampaignScreen', () => {
  it('renders an open campaign: its header, its chronicle and its console', async () => {
    mockCampaign();
    render(
      <SettingsProvider>
        <CampaignProvider>
          <Harness />
        </CampaignProvider>
      </SettingsProvider>,
    );

    // The header carries the way out and the way to the settings.
    expect(await screen.findByTitle('Switch Campaign / Return to Hub')).toBeInTheDocument();
    expect(screen.getByTitle('Global Settings')).toBeInTheDocument();

    // The chronicle shows the turn the provider loaded.
    expect(await screen.findByText(/the door gives onto a cold stair/i)).toBeInTheDocument();

    // And the console is there to type the next one into.
    expect(document.getElementById('action-console-input')).not.toBeNull();
  });
});
