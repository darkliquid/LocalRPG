import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { StoryTheater } from './StoryTheater';
import { CampaignProvider, useCampaign } from '../context/CampaignContext';
import { SettingsProvider } from '../context/SettingsContext';
import { useEffect } from 'react';
import { APIClient } from '../api/client';
import { expectNoViolations } from '../test/a11y';
import { Turn, TurnSegment } from '../types';

const seg = (text: string, clip_group: string, audio: string): TurnSegment => ({
  kind: 'speech',
  text,
  clip_group,
  audio_urls: [audio],
});

const turns: Turn[] = [
  {
    turn_number: 1,
    input_text: '',
    mode: 'do',
    prose: '',
    segments: [seg('one', 'g1', '/a'), seg('two', 'g1', '/a'), seg('three', 'g2', '/b')],
  },
];

// The playback comes from the campaign provider, so the theatre renders inside one
// and the clip it asks for is the client call the provider makes. The campaign's own
// loads are stubbed, because a theatre needs a campaign the way the app does.
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
  vi.spyOn(APIClient.prototype, 'getChronicle').mockResolvedValue([]);
  vi.spyOn(APIClient.prototype, 'getFindings').mockResolvedValue({ addressed: [] });
  vi.spyOn(APIClient.prototype, 'getGraph').mockResolvedValue({ nodes: [], links: [] });
  vi.spyOn(APIClient.prototype, 'listEntities').mockResolvedValue([]);
  vi.spyOn(APIClient.prototype, 'getRecap').mockResolvedValue(null as never);
  vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
    config: { media: { tts: { master_volume: 1 } } } as never,
    config_file_path: '',
    is_local_override: false,
  });
  vi.spyOn(APIClient, 'audioStatus').mockResolvedValue({
    available: true,
    playing: true,
    owner: 'device',
    turn: 0,
    segment: 0,
  });
  vi.spyOn(APIClient, 'getLimits').mockResolvedValue({ blocks: [] });
  vi.spyOn(APIClient, 'playTurnAudio').mockResolvedValue(undefined);
  vi.spyOn(APIClient, 'playSegmentAudio').mockResolvedValue(undefined);
}

// jsdom has no audio device and its play() returns undefined, which is the one
// window where the theatre's browser path can run before the provider's audio probe
// has answered. The test is about grouping and stepping, so the element is stubbed.
function stubAudioElement() {
  Object.defineProperty(HTMLMediaElement.prototype, 'play', {
    configurable: true,
    value: vi.fn().mockResolvedValue(undefined),
  });
  Object.defineProperty(HTMLMediaElement.prototype, 'pause', { configurable: true, value: vi.fn() });
}

// The theatre needs an open campaign, so the harness opens one the way the shell does.
function Theater({ children }: { children: React.ReactElement }) {
  const { activeGameID, handleSelectGame } = useCampaign();
  useEffect(() => {
    if (!activeGameID) handleSelectGame('campaign');
  }, [activeGameID, handleSelectGame]);
  return activeGameID ? children : null;
}

function renderTheater(element: React.ReactElement) {
  stubAudioElement();
  mockCampaign();
  return render(
    <SettingsProvider>
      <CampaignProvider>
        <Theater>{element}</Theater>
      </CampaignProvider>
    </SettingsProvider>,
  );
}

describe('StoryTheater grouping', () => {
  it('does not restart audio when stepping within a group', async () => {
    const { container } = renderTheater(<StoryTheater turns={turns} isOpen onClose={() => {}} />);
    // The beat the theatre is on is a group, so the clip it asks the server for is
    // that group's; the provider's audio probe has to have answered first.
    await waitFor(() => expect(APIClient.playSegmentAudio).toHaveBeenCalledWith('campaign', 1, 0, false));
    const afterOpen = vi.mocked(APIClient.playSegmentAudio).mock.calls.length;

    // Stepping to the second line of the same group shows it but leaves the audio
    // alone.
    fireEvent.click(screen.getByTitle('Next line'));
    expect(screen.getByText(/two/)).toBeInTheDocument();
    expect(vi.mocked(APIClient.playSegmentAudio).mock.calls.length).toBe(afterOpen);

    // Stepping into the next group requests its clip.
    fireEvent.click(screen.getByTitle('Next line'));
    expect(screen.getByText(/three/)).toBeInTheDocument();
    await waitFor(() => expect(APIClient.playSegmentAudio).toHaveBeenCalledWith('campaign', 1, 2, false));
    await expectNoViolations(container);
  });

  it('steps back to the previous line', async () => {
    renderTheater(<StoryTheater turns={turns} isOpen onClose={() => {}} />);
    await waitFor(() => expect(APIClient.playSegmentAudio).toHaveBeenCalled());
    fireEvent.click(screen.getByTitle('Next line'));
    fireEvent.click(screen.getByTitle('Previous line'));
    expect(screen.getByText(/one/)).toBeInTheDocument();
  });
});

describe('StoryTheater roll cards', () => {
  it('shows the roll card for the beat that narrates it', () => {
    const turn: Turn = {
      turn_number: 1,
      input_text: '',
      mode: 'do',
      prose: '',
      segments: [{ kind: 'narration', text: 'You slip past the guard.', check_ref: 'c1' }],
      checks: [
        {
          check_id: 'c1',
          actor: 'player',
          outcome: 'strong',
          outcome_text: 'You slip past the guard.',
          roll: { notation: '2d6', total: 9 },
        },
      ],
    };

    renderTheater(<StoryTheater turns={[turn]} isOpen onClose={() => {}} />);

    // The narration and the card both carry the outcome text; the notation is the
    // card's alone.
    expect(screen.getAllByText('You slip past the guard.').length).toBeGreaterThan(0);
    expect(screen.getByText('2d6')).toBeInTheDocument();
  });
});
