import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { NewEntityWizard } from './NewEntityWizard';
import { expectNoViolations } from '../test/a11y';
import { APIClient } from '../api/client';
import { EntityTypeCatalog, TTSConfig } from '../types';

const catalog: EntityTypeCatalog = {
  base_keys: [],
  types: [
    {
      id: 'character',
      label: 'Character',
      description: 'Someone with a voice.',
      keys: [{ name: 'voice', type: 'string', required: false, description: 'Voice profile id.' }],
    },
  ],
};

// The host passes both voice inputs so the wizard never reads settings: the
// fixture then needs one mock, not two.
const ttsConfig: TTSConfig = { type: 'disabled', auto_play: false, master_volume: 1 };

afterEach(() => {
  vi.restoreAllMocks();
});

describe('NewEntityWizard', () => {
  it('is a dialog named by its heading, holds focus, and cancels on Escape', async () => {
    vi.spyOn(APIClient.prototype, 'getEntityTypes').mockResolvedValue(catalog);
    const onClose = vi.fn();

    const { container } = render(
      <NewEntityWizard
        isOpen
        existingIds={[]}
        ttsConfig={ttsConfig}
        voiceProfiles={[]}
        onConfirm={vi.fn()}
        onClose={onClose}
      />,
    );

    const dialog = screen.getByRole('dialog', { name: 'New Note' });
    expect(dialog).toBeInTheDocument();
    await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));

    fireEvent.keyDown(dialog, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledTimes(1);
    await expectNoViolations(container);
  });
});
