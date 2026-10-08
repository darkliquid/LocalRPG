import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { EntityBatchDialog } from './EntityBatchDialog';
import { APIClient, HTTPError } from '../api/client';
import { SettingsResponse, WorldEntityBatch } from '../types';

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
    const preview = vi.spyOn(APIClient, 'previewWorldEntitiesStream').mockResolvedValue(batch);

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

  it('extracts a batch from a folder instead of an instruction', async () => {
    const preview = vi.spyOn(APIClient, 'previewWorldEntitiesStream').mockResolvedValue(batch);

    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);

    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    expect(screen.queryByLabelText('Count')).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(/folder path/i), { target: { value: '/tmp/notes' } });
    fireEvent.change(screen.getByLabelText(/instruction/i), {
      target: { value: 'keep the source names' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^extract$/i }));

    await waitFor(() => expect(preview).toHaveBeenCalledTimes(1));
    expect(preview.mock.calls[0][1]).toEqual({
      instruction: 'keep the source names',
      source: { kind: 'folder', path: '/tmp/notes' },
      kinds: undefined,
      count: undefined,
      focus: undefined,
    });
  });

  it('extracts a batch from URLs', async () => {
    const preview = vi.spyOn(APIClient, 'previewWorldEntitiesStream').mockResolvedValue(batch);

    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);

    fireEvent.click(screen.getByRole('button', { name: /urls/i }));
    fireEvent.change(screen.getByLabelText(/urls, one per line/i), {
      target: { value: 'https://example.org/a\nhttps://example.org/b\n' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^extract$/i }));

    await waitFor(() => expect(preview).toHaveBeenCalledTimes(1));
    expect(preview.mock.calls[0][1].source).toEqual({
      kind: 'url',
      urls: ['https://example.org/a', 'https://example.org/b'],
    });
  });

  it('fills the folder path from the native picker', async () => {
    vi.spyOn(APIClient, 'startDirectoryChoice').mockResolvedValue(undefined);
    vi.spyOn(APIClient, 'directoryChoice').mockResolvedValue({
      status: 'selected',
      path: '/home/you/notes',
    });

    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);

    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    fireEvent.click(screen.getByRole('button', { name: /browse/i }));

    await waitFor(() => expect(screen.getByLabelText(/folder path/i)).toHaveValue('/home/you/notes'));
  });

  it('offers to raise the source limit in place, then extracts', async () => {
    const preview = vi.spyOn(APIClient, 'previewWorldEntitiesStream').mockImplementation(async (_worldId, req) => {
      if (req.limits?.max_chunks) return batch;
      throw new HTTPError(
        400,
        'the source is larger than the chunk limit: it holds 300 chunks and the limit is 200',
        'generation_source_limit'
      );
    });
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
      config: { generation: { max_chunks: 200 } },
    } as unknown as SettingsResponse);

    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);

    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    fireEvent.change(screen.getByLabelText(/folder path/i), { target: { value: '/tmp/big' } });
    fireEvent.click(screen.getByRole('button', { name: /^extract$/i }));

    const field = await screen.findByLabelText(/source chunk limit/i);
    await waitFor(() => expect(field).toHaveValue(400));
    // The raw envelope is not shown to the user.
    expect(screen.queryByText(/^\{/)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /use for this run/i }));

    await waitFor(() => expect(screen.getByText('The Tidewatch')).toBeInTheDocument());
    expect(preview.mock.calls[preview.mock.calls.length - 1][1].limits).toEqual({ max_chunks: 400 });
  });

  it('saves a raised source limit when asked to make it stick', async () => {
    vi.spyOn(APIClient, 'previewWorldEntitiesStream').mockImplementation(async (_worldId, req) => {
      if (req.limits?.max_chunks) return batch;
      throw new HTTPError(400, 'the source is larger than the chunk limit', 'generation_source_limit');
    });
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
      config: { generation: { max_chunks: 200 } },
    } as unknown as SettingsResponse);
    const save = vi.spyOn(APIClient, 'raiseGenerationLimit').mockResolvedValue(undefined);

    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    fireEvent.change(screen.getByLabelText(/folder path/i), { target: { value: '/tmp/big' } });
    fireEvent.click(screen.getByRole('button', { name: /^extract$/i }));

    const field = await screen.findByLabelText(/source chunk limit/i);
    await waitFor(() => expect(field).toHaveValue(400));
    fireEvent.click(screen.getByRole('button', { name: /save and use/i }));

    await waitFor(() => expect(save).toHaveBeenCalledWith({ max_chunks: 400 }));
    await waitFor(() => expect(screen.getByText('The Tidewatch')).toBeInTheDocument());
  });

  it('uses an edited limit when the main action is pressed again', async () => {
    const preview = vi.spyOn(APIClient, 'previewWorldEntitiesStream').mockImplementation(async (_worldId, req) => {
      if (req.limits?.max_chunks === 500) return batch;
      throw new HTTPError(400, 'the source is larger than the chunk limit', 'generation_source_limit');
    });
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
      config: { generation: { max_chunks: 200 } },
    } as unknown as SettingsResponse);

    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);

    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    fireEvent.change(screen.getByLabelText(/folder path/i), { target: { value: '/tmp/big' } });
    fireEvent.click(screen.getByRole('button', { name: /^extract$/i }));

    const field = await screen.findByLabelText(/source chunk limit/i);
    await waitFor(() => expect(field).toHaveValue(400));

    // The user types a different number and presses the dialog's own action.
    fireEvent.change(field, { target: { value: '500' } });
    fireEvent.click(screen.getByRole('button', { name: /^extract/i }));

    await waitFor(() => expect(screen.getByText('The Tidewatch')).toBeInTheDocument());
    expect(preview.mock.calls[preview.mock.calls.length - 1][1].limits).toEqual({ max_chunks: 500 });
  });

  it('warns when a batch ran out of room', async () => {
    vi.spyOn(APIClient, 'previewWorldEntitiesStream').mockResolvedValue({
      entities: [{ id: 'saltmarch', name: 'Saltmarch', type: 'location', body: 'A port.' }],
      cut_off: 2,
    });

    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText(/instruction/i), { target: { value: 'add a place' } });
    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));

    await waitFor(() => expect(screen.getByText(/2 batches ran out of room/)).toBeInTheDocument());
    expect(screen.getByText(/the rest of those pages was not read/)).toBeInTheDocument();
  });

  it('says when the fallback answered instead of a model', async () => {
    vi.spyOn(APIClient, 'previewWorldEntitiesStream').mockResolvedValue({
      entities: [{ id: 'maren-vale', name: 'Maren Vale', type: 'character', body: 'A wanderer.' }],
      oracle: true,
    });

    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText(/instruction/i), { target: { value: 'add a faction' } });
    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));

    await waitFor(() =>
      expect(screen.getByText(/built-in template generator/i)).toBeInTheDocument()
    );
    expect(screen.getByText(/Settings .* AI Agents/i)).toBeInTheDocument();
  });

  it('surfaces a refusal to read a source with no provider', async () => {
    vi.spyOn(APIClient, 'previewWorldEntitiesStream').mockRejectedValue(
      new HTTPError(
        400,
        'reading a source needs a model provider: no agent role is set up to generate. The gm role is the shipped echo command, so assign a model provider to gm, or to a generator role, in Settings → AI Agents'
      )
    );

    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    fireEvent.change(screen.getByLabelText(/folder path/i), { target: { value: '/tmp/notes' } });
    fireEvent.click(screen.getByRole('button', { name: /^extract$/i }));

    await waitFor(() =>
      expect(screen.getByText(/needs a model provider/i)).toBeInTheDocument()
    );
    expect(screen.getByText(/Settings .* AI Agents/i)).toBeInTheDocument();
  });

  it('will not extract without a source', () => {
    render(<EntityBatchDialog worldId="w" onClose={() => {}} />);

    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    expect(screen.getByRole('button', { name: /^extract$/i })).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/folder path/i), { target: { value: '/tmp/notes' } });
    expect(screen.getByRole('button', { name: /^extract$/i })).toBeEnabled();
  });
});
