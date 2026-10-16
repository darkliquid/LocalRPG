import { render, screen, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { CampaignProvider, useCampaign } from './CampaignContext';
import { SettingsProvider } from './SettingsContext';
import { APIClient } from '../api/client';
import { Turn } from '../types';

const turn: Turn = {
  turn_number: 1,
  input_text: 'I open the door.',
  mode: 'Action',
  prose: 'The door gives.',
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
  vi.spyOn(APIClient.prototype, 'getGameState').mockResolvedValue({
    game_name: 'Campaign',
    turnover: undefined,
  } as never);
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

function Probe() {
  const campaign = useCampaign();
  return (
    <div>
      <span data-testid="status">{campaign.campaignStatus}</span>
      <span data-testid="turns">{campaign.chronicle.length}</span>
      <button type="button" onClick={() => campaign.handleSelectGame('campaign')}>
        Open
      </button>
      <button type="button" onClick={() => campaign.leaveCampaign()}>
        Leave
      </button>
      <button type="button" onClick={() => void campaign.handleActionSubmit('Action', 'I open the door.')}>
        Play
      </button>
    </div>
  );
}

function renderProbe() {
  return render(
    <SettingsProvider>
      <CampaignProvider>
        <Probe />
      </CampaignProvider>
    </SettingsProvider>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('CampaignProvider', () => {
  it('loads the campaign a player opens', async () => {
    mockCampaign();
    renderProbe();

    screen.getByRole('button', { name: 'Open' }).click();

    await waitFor(() => expect(screen.getByTestId('turns')).toHaveTextContent('1'));
    expect(screen.getByTestId('status')).toHaveTextContent('ready');
  });

  it('leaves the campaign and forgets it', async () => {
    mockCampaign();
    renderProbe();

    screen.getByRole('button', { name: 'Open' }).click();
    await waitFor(() => expect(screen.getByTestId('turns')).toHaveTextContent('1'));

    screen.getByRole('button', { name: 'Leave' }).click();

    await waitFor(() => expect(screen.getByTestId('status')).toHaveTextContent('idle'));
    expect(screen.getByTestId('turns')).toHaveTextContent('0');
  });

  it('appends the turn the stream reports', async () => {
    mockCampaign();
    vi.spyOn(APIClient, 'streamTurn').mockImplementation(async (_gameID, _body, onEvent) => {
      onEvent({ type: 'turn', turn } as never);
    });
    vi.spyOn(APIClient, 'mergePlaybackLedger').mockResolvedValue({ entries: {} } as never);

    renderProbe();
    screen.getByRole('button', { name: 'Open' }).click();
    await waitFor(() => expect(screen.getByTestId('turns')).toHaveTextContent('1'));

    screen.getByRole('button', { name: 'Play' }).click();

    // The chronicle had one turn loaded and the stream appends its own, so the
    // provider worked the event rather than dropping it.
    await waitFor(() => expect(screen.getByTestId('turns')).toHaveTextContent('2'));
  });
});
