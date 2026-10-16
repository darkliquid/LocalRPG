import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { NewCampaignModal } from './NewCampaignModal';
import { expectNoViolations } from '../../test/a11y';
import { APIClient } from '../../api/client';
import type { SystemDetail, WorldInfo } from '../../types';

const world: WorldInfo = { id: 'w', name: 'World', description: '', genre: '', compatible_systems: [] };

const systemDetail: SystemDetail = {
  id: 'sys',
  name: 'System',
  version: '1.0.0',
  description: '',
  script: '',
  rules_prompt: '',
  character_creation: {
    preamble: 'Answer in the chronicle tone.',
    fields: [
      {
        id: 'calling',
        label: 'Calling',
        kind: 'select',
        options: ['sellsword', 'scholar'],
        default: 'scholar',
        generatable: false,
      },
    ],
  },
};

function mockModal() {
  vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
    config: { media: { tts: { voice_profiles: [] } } },
  } as never);
  vi.spyOn(APIClient, 'getSystem').mockResolvedValue(systemDetail);
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('NewCampaignModal authored creation prompts', () => {
  it('renders a system-authored select and submits its answer as extra', async () => {
    mockModal();
    const onCreateGame = vi.fn().mockResolvedValue(undefined);

    const { container } = render(
      <NewCampaignModal
        isOpen
        world={world}
        systems={[{ id: 'sys', name: 'System', version: '1.0.0', description: '' }]}
        onClose={() => {}}
        onCreateGame={onCreateGame}
        isSubmitting={false}
      />,
    );

    await waitFor(() => expect(screen.getByText('Calling')).toBeInTheDocument());
    expect(screen.getByText('Answer in the chronicle tone.')).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'sellsword' })).toBeInTheDocument();

    fireEvent.change(screen.getByDisplayValue('scholar'), { target: { value: 'sellsword' } });
    fireEvent.change(screen.getByPlaceholderText(/Chronicles of Eldoria/i), {
      target: { value: 'The Salt Road' },
    });
    fireEvent.change(screen.getByPlaceholderText(/Valen Duskwarden/i), { target: { value: 'Elena' } });
    fireEvent.click(screen.getByRole('button', { name: /create campaign/i }));

    await waitFor(() => expect(onCreateGame).toHaveBeenCalledTimes(1));
    expect(onCreateGame.mock.calls[0][0].player.extra).toEqual({ calling: 'sellsword' });
    await expectNoViolations(container);
  });
});
