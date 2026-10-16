import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { LauncherHub } from './LauncherHub';
import { APIClient } from '../api/client';
import { expectNoViolations } from '../test/a11y';

// The studio the settings dialog contains reads the whole configuration on
// mount; the dialog's own behaviour does not depend on it, so it is stubbed.
vi.mock('./SettingsStudio', () => ({ SettingsStudio: () => <div data-testid="settings-studio" /> }));

afterEach(() => {
  vi.restoreAllMocks();
});

describe('LauncherHub', () => {
  it('opens Global Settings as a dialog named by its heading, and closes it on Escape', async () => {
    vi.spyOn(APIClient, 'listGames').mockResolvedValue([]);
    vi.spyOn(APIClient, 'listSystems').mockResolvedValue([]);
    vi.spyOn(APIClient, 'listWorlds').mockResolvedValue([]);

    const { container } = render(<LauncherHub onSelectGame={vi.fn()} onOpenDocs={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Settings' }));

    const dialog = await screen.findByRole('dialog', { name: 'Global Settings' });
    expect(dialog.contains(document.activeElement)).toBe(true);

    fireEvent.keyDown(dialog, { key: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Global Settings' })).not.toBeInTheDocument());
    await expectNoViolations(container);
  });
});
