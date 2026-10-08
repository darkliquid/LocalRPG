import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { WorldGenerateDialog } from './WorldGenerateDialog';
import { APIClient, HTTPError } from '../api/client';
import { SettingsResponse, WorldDraftInfo } from '../types';

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

  it('offers to raise the limit in place when a generation hits one', async () => {
    const generate = vi.spyOn(APIClient, 'generateWorld').mockImplementation(async (req, onEvent) => {
      if (req.dry_run) {
        onEvent({ type: 'estimate', estimate: { calls: 4, priced: false } });
        return;
      }
      // The retry carries the raised limit, so it succeeds.
      if (req.limits?.max_calls) {
        onEvent({ type: 'draft', draft });
        return;
      }
      onEvent({
        type: 'error',
        code: 'generation_call_limit',
        message: 'generation call budget exceeded: 20 calls',
      });
    });
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
      config: { generation: { max_calls: 20 } },
    } as unknown as SettingsResponse);

    const onDraft = vi.fn();
    render(<WorldGenerateDialog onCancel={() => {}} onDraft={onDraft} estimateDelayMs={0} />);

    fireEvent.change(screen.getByLabelText(/premise/i), { target: { value: 'a drowned kingdom' } });
    await waitFor(() => expect(screen.getByTestId('generation-estimate')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));

    // The notice offers the field, with a raised suggestion, and both choices.
    const field = await screen.findByLabelText(/call limit/i);
    await waitFor(() => expect(field).toHaveValue(40));
    expect(screen.getByRole('button', { name: /use for this run/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /save and use/i })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /use for this run/i }));

    await waitFor(() => expect(onDraft).toHaveBeenCalledWith(draft));
    expect(generate.mock.calls[generate.mock.calls.length - 1][0].limits).toEqual({ max_calls: 40 });
  });

  it('uses an edited limit when the main action is pressed again', async () => {
    const generate = vi.spyOn(APIClient, 'generateWorld').mockImplementation(async (req, onEvent) => {
      if (req.dry_run) {
        onEvent({ type: 'estimate', estimate: { calls: 4, priced: false } });
        return;
      }
      if (req.limits?.max_calls === 55) {
        onEvent({ type: 'draft', draft });
        return;
      }
      onEvent({ type: 'error', code: 'generation_call_limit', message: 'generation call budget exceeded' });
    });
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
      config: { generation: { max_calls: 20 } },
    } as unknown as SettingsResponse);

    const onDraft = vi.fn();
    render(<WorldGenerateDialog onCancel={() => {}} onDraft={onDraft} estimateDelayMs={0} />);
    fireEvent.change(screen.getByLabelText(/premise/i), { target: { value: 'a drowned kingdom' } });
    await waitFor(() => expect(screen.getByTestId('generation-estimate')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));

    const field = await screen.findByLabelText(/call limit/i);
    await waitFor(() => expect(field).toHaveValue(40));

    // The user types a different number and presses the dialog's own action.
    fireEvent.change(field, { target: { value: '55' } });
    fireEvent.click(screen.getByRole('button', { name: /^generate \(limit 55\)$/i }));

    await waitFor(() => expect(onDraft).toHaveBeenCalledWith(draft));
    expect(generate.mock.calls[generate.mock.calls.length - 1][0].limits).toEqual({ max_calls: 55 });
  });

  it('saves a raised limit when asked to make it stick', async () => {
    vi.spyOn(APIClient, 'generateWorld').mockImplementation(async (req, onEvent) => {
      if (req.dry_run) {
        onEvent({ type: 'estimate', estimate: { calls: 4, priced: false } });
        return;
      }
      if (req.limits?.max_calls) {
        onEvent({ type: 'draft', draft });
        return;
      }
      onEvent({ type: 'error', code: 'generation_call_limit', message: 'generation call budget exceeded' });
    });
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
      config: { generation: { max_calls: 20 } },
    } as unknown as SettingsResponse);
    const save = vi.spyOn(APIClient, 'raiseGenerationLimit').mockResolvedValue(undefined);

    const onDraft = vi.fn();
    render(<WorldGenerateDialog onCancel={() => {}} onDraft={onDraft} estimateDelayMs={0} />);
    fireEvent.change(screen.getByLabelText(/premise/i), { target: { value: 'a drowned kingdom' } });
    await waitFor(() => expect(screen.getByTestId('generation-estimate')).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));

    await screen.findByLabelText(/call limit/i);
    fireEvent.click(screen.getByRole('button', { name: /save and use/i }));

    await waitFor(() => expect(save).toHaveBeenCalledWith({ max_calls: 40 }));
    await waitFor(() => expect(onDraft).toHaveBeenCalledWith(draft));
  });

  it('hides the counts once a source decides how many entities exist', async () => {
    vi.spyOn(APIClient, 'generateWorld').mockImplementation(async (_req, onEvent) => {
      onEvent({ type: 'estimate', estimate: { calls: 1, priced: false } });
    });

    render(<WorldGenerateDialog onCancel={() => {}} onDraft={() => {}} estimateDelayMs={0} />);

    expect(screen.getByLabelText('Locations')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    expect(screen.queryByLabelText('Locations')).not.toBeInTheDocument();
    expect(screen.getByText(/the source decides how many entities/i)).toBeInTheDocument();
  });

  it('fills the folder path from the native picker', async () => {
    vi.spyOn(APIClient, 'generateWorld').mockImplementation(async (_req, onEvent) => {
      onEvent({ type: 'estimate', estimate: { calls: 1, priced: false } });
    });
    const start = vi.spyOn(APIClient, 'startDirectoryChoice').mockResolvedValue(undefined);
    vi.spyOn(APIClient, 'directoryChoice').mockResolvedValue({
      status: 'selected',
      path: '/home/you/notes',
    });

    render(<WorldGenerateDialog onCancel={() => {}} onDraft={() => {}} estimateDelayMs={0} />);

    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    fireEvent.click(screen.getByRole('button', { name: /browse/i }));

    await waitFor(() => expect(screen.getByLabelText(/folder path/i)).toHaveValue('/home/you/notes'));
    expect(start).toHaveBeenCalledWith('Choose a source folder');
  });

  it('falls back to a typed path when there is no native dialog', async () => {
    vi.spyOn(APIClient, 'generateWorld').mockImplementation(async (_req, onEvent) => {
      onEvent({ type: 'estimate', estimate: { calls: 1, priced: false } });
    });
    vi.spyOn(APIClient, 'startDirectoryChoice').mockRejectedValue(new HTTPError(501, 'no dialog'));

    render(<WorldGenerateDialog onCancel={() => {}} onDraft={() => {}} estimateDelayMs={0} />);

    fireEvent.click(screen.getByRole('button', { name: /folder/i }));
    fireEvent.click(screen.getByRole('button', { name: /browse/i }));

    await waitFor(() =>
      expect(screen.getByText(/no native folder dialog is available/i)).toBeInTheDocument()
    );
    // The field is still usable by hand.
    fireEvent.change(screen.getByLabelText(/folder path/i), { target: { value: '/tmp/notes' } });
    expect(screen.getByLabelText(/folder path/i)).toHaveValue('/tmp/notes');
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
