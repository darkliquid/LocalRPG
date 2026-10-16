import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { CampaignGallery } from './CampaignGallery';
import { GameSummary } from '../../types';

const game: GameSummary = {
  id: 'ashen-reach',
  name: 'Ashen Reach',
  system_id: 'osric',
  world_id: 'the-reach',
  player_name: 'Andrew',
  turn_count: 3,
  last_played: '2026-10-15T10:00:00Z',
};

describe('CampaignGallery', () => {
  it('is a dialog named Campaigns, holds focus, and closes on Escape', () => {
    const onClose = vi.fn();

    render(
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
  });
});
