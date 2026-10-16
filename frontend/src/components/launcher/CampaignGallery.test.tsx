import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { CampaignGallery } from './CampaignGallery';
import { GameSummary } from '../../types';
import { expectNoViolations } from '../../test/a11y';

const game: GameSummary = {
  id: 'ashen-reach',
  name: 'Ashen Reach',
  system_id: 'osric',
  world_id: 'the-reach',
  player_name: 'Andrew',
  turn_count: 3,
  last_played: '2026-10-15T10:00:00Z',
  thumbnail_url: '',
};

describe('CampaignGallery', () => {
  it('is a dialog named Campaigns, holds focus, and closes on Escape', async () => {
    const onClose = vi.fn();

    const { container } = render(
      <CampaignGallery
        isOpen
        games={[game]}
        worlds={[]}
        systems={[]}
        onPlayGame={vi.fn()}
        onOpenSettings={vi.fn()}
        onCreateCampaign={vi.fn()}
        onClose={onClose}
      />,
    );

    const dialog = screen.getByRole('dialog', { name: 'Campaigns' });
    expect(dialog.contains(document.activeElement)).toBe(true);

    fireEvent.keyDown(dialog, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledTimes(1);
    await expectNoViolations(container);
  });
});
