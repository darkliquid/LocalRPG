import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { EntityBatchDialog } from './EntityBatchDialog';
import { APIClient } from '../api/client';
import { WorldEntityBatch } from '../types';

const batch: WorldEntityBatch = {
  entities: [
    {
      id: 'the-tidewatch',
      name: 'The Tidewatch',
      type: 'faction',
      body: 'Rivals of [[Saltmarch]].',
      links: ['Saltmarch'],
      dropped_links: ['Nowhere'],
    },
  ],
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe('EntityBatchDialog', () => {
  it('previews a batch and accepts it', async () => {
    const accept = vi
      .spyOn(APIClient, 'acceptWorldEntities')
      .mockResolvedValue({ written: ['the-tidewatch'] });
    const onAccepted = vi.fn();

    render(<EntityBatchDialog worldId="w" onClose={() => {}} onAccepted={onAccepted} batch={batch} />);

    expect(screen.getByText('The Tidewatch')).toBeInTheDocument();
    expect(screen.getByText(/links: saltmarch/i)).toBeInTheDocument();
    expect(screen.getByText(/dropped unresolved links: nowhere/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /accept/i }));

    await waitFor(() => expect(accept).toHaveBeenCalledTimes(1));
    expect(accept.mock.calls[0][1]).toEqual({ entities: batch.entities, rename: false });
    await waitFor(() => expect(onAccepted).toHaveBeenCalledWith({ written: ['the-tidewatch'] }));
    expect(screen.getByText(/wrote 1 entity/i)).toBeInTheDocument();
  });

  it('generates a preview from an instruction', async () => {
    const preview = vi.spyOn(APIClient, 'previewWorldEntities').mockResolvedValue(batch);

    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);

    fireEvent.change(screen.getByLabelText(/instruction/i), {
      target: { value: 'three rival factions in the south' },
    });
    fireEvent.change(screen.getByLabelText(/count/i), { target: { value: '3' } });
    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));

    await waitFor(() => expect(preview).toHaveBeenCalledTimes(1));
    expect(preview.mock.calls[0][1]).toMatchObject({
      instruction: 'three rival factions in the south',
      kinds: ['faction'],
      count: 3,
    });
    await waitFor(() => expect(screen.getByText('The Tidewatch')).toBeInTheDocument());
  });

  it('asks to rename when the caller opts in', async () => {
    const accept = vi.spyOn(APIClient, 'acceptWorldEntities').mockResolvedValue({
      written: ['the-tidewatch-2'],
      renamed: ['the-tidewatch-2'],
    });

    render(<EntityBatchDialog worldId="w" onClose={() => {}} batch={batch} />);
    fireEvent.click(screen.getByLabelText(/rename on an id clash/i));
    fireEvent.click(screen.getByRole('button', { name: /accept/i }));

    await waitFor(() => expect(accept).toHaveBeenCalled());
    expect(accept.mock.calls[0][1].rename).toBe(true);
  });

  it('surfaces a refusal', async () => {
    vi.spyOn(APIClient, 'acceptWorldEntities').mockRejectedValue(
      new Error('world entity already exists: the-tidewatch')
    );

    render(<EntityBatchDialog worldId="w" onClose={() => {}} batch={batch} />);
    fireEvent.click(screen.getByRole('button', { name: /accept/i }));

    await waitFor(() =>
      expect(screen.getByText(/world entity already exists/i)).toBeInTheDocument()
    );
  });
});
