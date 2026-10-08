import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { WorldGenerateDialog } from './WorldGenerateDialog';
import { APIClient } from '../api/client';
import { WorldDraftInfo } from '../types';

const draft: WorldDraftInfo = {
  id: 'ashen-reach',
  name: 'Ashen Reach',
  description: 'A dying frontier.',
  genre: 'dark fantasy',
  lore: '# Lore\n\nA.\n',
  sections: [{ title: 'Lore', body: 'A.' }],
  entities: [{ id: 'saltmarch', name: 'Saltmarch', type: 'location', body: 'A port.' }],
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe('WorldGenerateDialog', () => {
  it('collects a brief and shows step progress', async () => {
    vi.spyOn(APIClient, 'generateWorld').mockImplementation(async (req, onEvent) => {
      if (req.dry_run) {
        onEvent({ type: 'estimate', estimate: { calls: 4, priced: false } });
        return;
      }
      onEvent({ type: 'step', step: { name: 'outline', status: 'done' } });
      onEvent({ type: 'step', step: { name: 'places', status: 'done' } });
      onEvent({ type: 'draft', draft });
    });

    const onDraft = vi.fn();
    render(<WorldGenerateDialog onCancel={() => {}} onDraft={onDraft} estimateDelayMs={0} />);

    fireEvent.change(screen.getByLabelText(/premise/i), {
      target: { value: 'a drowned kingdom' },
    });

    await waitFor(() =>
      expect(screen.getByTestId('generation-estimate')).toHaveTextContent('4 calls')
    );
    expect(screen.getByTestId('generation-estimate')).toHaveTextContent('unpriced');

    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));

    await waitFor(() => expect(screen.getByText('Outline')).toBeInTheDocument());
    expect(screen.getByText('Places and factions')).toBeInTheDocument();
    await waitFor(() => expect(onDraft).toHaveBeenCalledWith(draft));
  });

  it('requires confirmation for a large estimate', async () => {
    const generate = vi.spyOn(APIClient, 'generateWorld').mockImplementation(async (req, onEvent) => {
      if (req.dry_run) {
        onEvent({ type: 'estimate', estimate: { calls: 20, priced: true, cost_micros: 5_000_000 } });
        return;
      }
      onEvent({ type: 'draft', draft });
    });

    const onDraft = vi.fn();
    render(<WorldGenerateDialog onCancel={() => {}} onDraft={onDraft} estimateDelayMs={0} />);

    fireEvent.change(screen.getByLabelText(/premise/i), { target: { value: 'a drowned kingdom' } });
    await waitFor(() =>
      expect(screen.getByTestId('generation-estimate')).toHaveTextContent('$5.00')
    );

    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));
    await waitFor(() => expect(screen.getByTestId('generation-confirm')).toBeInTheDocument());
    expect(onDraft).not.toHaveBeenCalled();

    // The second click is the confirmation.
    fireEvent.click(screen.getByRole('button', { name: /generate anyway/i }));
    await waitFor(() => expect(onDraft).toHaveBeenCalledWith(draft));
    // One dry run plus one generation: the first click only asked for confirmation.
    expect(generate).toHaveBeenCalledTimes(2);
  });

  it('offers a folder source that needs no network, and warns about URLs', async () => {
    vi.spyOn(APIClient, 'generateWorld').mockImplementation(async (_req, onEvent) => {
      onEvent({ type: 'estimate', estimate: { calls: 1, priced: false } });
    });

    render(<WorldGenerateDialog onCancel={() => {}} onDraft={() => {}} estimateDelayMs={0} />);

    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    expect(screen.getByLabelText(/folder path/i)).toBeInTheDocument();
    expect(screen.getByText(/nothing is fetched/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /urls/i }));
    expect(screen.getByLabelText(/urls, one per line/i)).toBeInTheDocument();
    expect(screen.getByText(/this fetches the pages you name/i)).toBeInTheDocument();
  });

  it('streams a source ingestion through the ingest endpoint', async () => {
    vi.spyOn(APIClient, 'generateWorld').mockImplementation(async (_req, onEvent) => {
      onEvent({ type: 'estimate', estimate: { calls: 5, priced: false } });
    });
    const ingest = vi.spyOn(APIClient, 'ingestWorld').mockImplementation(async (_req, onEvent) => {
      onEvent({ type: 'step', step: { name: 'extract', status: 'done', detail: '2 chunk(s)' } });
      onEvent({ type: 'draft', draft });
    });

    const onDraft = vi.fn();
    render(<WorldGenerateDialog onCancel={() => {}} onDraft={onDraft} estimateDelayMs={0} />);

    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    fireEvent.change(screen.getByLabelText(/folder path/i), { target: { value: '/tmp/notes' } });
    await waitFor(() => expect(screen.getByTestId('generation-estimate')).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));
    await waitFor(() => expect(ingest).toHaveBeenCalled());
    expect(screen.getByText('Read the source')).toBeInTheDocument();
    expect(screen.getByText('2 chunk(s)')).toBeInTheDocument();
  });
});
