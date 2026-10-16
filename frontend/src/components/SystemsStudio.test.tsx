import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { SystemsStudio } from './SystemsStudio';
import { APIClient } from '../api/client';
import { expectNoViolations } from '../test/a11y';

// The rules and script tabs mount CodeMirror, which needs a real layout; the
// studio's own rendering does not depend on it, so it is replaced with a stub.
vi.mock('./editor/MarkdownEditor', () => ({ default: () => <div data-testid="editor" /> }));

function mockStudio() {
  vi.spyOn(APIClient, 'listSystems').mockResolvedValue([
    { id: 'osric', name: 'OSRIC', description: '', version: '1.0' },
  ]);
  vi.spyOn(APIClient, 'listReferenceSystems').mockResolvedValue({
    systems: [{ id: 'osric-base', name: 'OSRIC Base', version: '1.0', description: '', rules_prompt: '', script: '' }],
  });
  vi.spyOn(APIClient, 'deleteSystem').mockResolvedValue(undefined);
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('SystemsStudio dialogs', () => {
  it('confirms a system delete in a dialog that Escape cancels', async () => {
    mockStudio();
    const { container } = render(<SystemsStudio />);

    fireEvent.click(await screen.findByTitle('Delete system "OSRIC"'));

    const dialog = await screen.findByRole('dialog', { name: 'Delete system "OSRIC"?' });
    fireEvent.keyDown(dialog, { key: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('dialog', { name: /delete system/i })).not.toBeInTheDocument());
    expect(APIClient.deleteSystem).not.toHaveBeenCalled();
    await expectNoViolations(container);
  });

  it('opens the base system catalogue as a dialog', async () => {
    mockStudio();
    render(<SystemsStudio />);

    fireEvent.click(await screen.findByTitle('Start from a reference base system'));

    const dialog = await screen.findByRole('dialog', { name: 'Start from a base system' });
    fireEvent.keyDown(dialog, { key: 'Escape' });

    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: 'Start from a base system' })).not.toBeInTheDocument(),
    );
  });
});

describe('SystemsStudio tabs', () => {
  it('renders every tab with its own content', async () => {
    mockStudio();
    render(<SystemsStudio />);

    // The manifest is the tab it opens on.
    expect(await screen.findByText('System Name')).toBeInTheDocument();

    for (const [tab, landmark] of [
      ['rules.md', /AI Storyteller Instructions/],
      ['Mechanics', /Mechanics/],
      ['mechanics.js', /JavaScript Runtime/],
      ['Tests', /0 scenarios/],
    ] as const) {
      fireEvent.click(screen.getByRole('button', { name: new RegExp(`^${tab}$`, 'i') }));
      expect(await screen.findByText(landmark)).toBeInTheDocument();
    }
  });
});
