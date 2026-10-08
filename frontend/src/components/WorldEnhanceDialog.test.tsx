import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { WorldEnhanceDialog } from './WorldEnhanceDialog';
import { APIClient } from '../api/client';
import { WorldEnhancement } from '../types';

const proposals: WorldEnhancement[] = [
  { kind: 'lore', title: 'History', body: 'Long ago...', reason: 'fills a gap' },
  { kind: 'hook', title: 'The debt', body: 'A creditor arrives.' },
];

afterEach(() => {
  vi.restoreAllMocks();
});

describe('WorldEnhanceDialog', () => {
  it('accepts and rejects proposals', async () => {
    const apply = vi
      .spyOn(APIClient, 'applyWorldEnhancements')
      .mockResolvedValue({ written: ['prompts/lore.md'] });

    render(<WorldEnhanceDialog worldId="w" onClose={() => {}} proposals={proposals} />);

    expect(screen.getByText(/2 of 2 accepted/i)).toBeInTheDocument();
    expect(screen.getByText(/why: fills a gap/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /reject story hook the debt/i }));
    expect(screen.getByText(/1 of 2 accepted/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /apply accepted/i }));

    await waitFor(() => expect(apply).toHaveBeenCalledTimes(1));
    expect(apply.mock.calls[0][1].proposals).toEqual([proposals[0]]);
  });

  it('disables apply when everything is rejected', () => {
    render(<WorldEnhanceDialog worldId="w" onClose={() => {}} proposals={proposals} />);

    fireEvent.click(screen.getByRole('button', { name: /reject lore history/i }));
    fireEvent.click(screen.getByRole('button', { name: /reject story hook the debt/i }));

    expect(screen.getByRole('button', { name: /apply accepted/i })).toBeDisabled();
  });

  it('asks for proposals from an instruction', async () => {
    const enhance = vi.spyOn(APIClient, 'enhanceWorld').mockResolvedValue({ proposals });

    render(<WorldEnhanceDialog worldId="w" onClose={() => {}} />);

    fireEvent.change(screen.getByLabelText(/what should change/i), {
      target: { value: 'deepen it' },
    });
    fireEvent.click(screen.getByRole('button', { name: /propose changes/i }));

    await waitFor(() => expect(enhance).toHaveBeenCalledTimes(1));
    expect(enhance.mock.calls[0][1]).toEqual({ instruction: 'deepen it' });
    await waitFor(() => expect(screen.getByText('History')).toBeInTheDocument());
  });

  it('shows the entity a proposal would write', () => {
    render(
      <WorldEnhanceDialog
        worldId="w"
        onClose={() => {}}
        proposals={[
          {
            kind: 'entity',
            title: 'The Salt Circle',
            body: 'Rivals.',
            entity: { id: 'the-salt-circle', name: 'The Salt Circle', type: 'faction', body: 'Rivals.' },
          },
        ]}
      />
    );
    expect(screen.getByText('the-salt-circle')).toBeInTheDocument();
    expect(screen.getByText(/^new entity$/i)).toBeInTheDocument();
  });

  it('surfaces a refusal', async () => {
    vi.spyOn(APIClient, 'applyWorldEnhancements').mockRejectedValue(
      new Error('world entity already exists: the-debt')
    );

    render(<WorldEnhanceDialog worldId="w" onClose={() => {}} proposals={proposals} />);
    fireEvent.click(screen.getByRole('button', { name: /apply accepted/i }));

    await waitFor(() =>
      expect(screen.getByText(/world entity already exists/i)).toBeInTheDocument()
    );
  });
});
