import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { SystemGenerateDialog } from './SystemGenerateDialog';
import { APIClient } from '../api/client';
import { SystemDraftInfo } from '../types';

const draft: SystemDraftInfo = {
  id: 'iron-steam',
  name: 'Iron & Steam',
  version: '1.0.0',
  description: 'Steampunk d20 system.',
  script: 'onAction("do", function(ctx) {});',
  rules_prompt: '# Rules Guide',
  verify: { ok: true, script: true },
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe('SystemGenerateDialog', () => {
  it('collects a brief and shows step progress', async () => {
    vi.spyOn(APIClient, 'generateSystem').mockImplementation(async (req, onEvent) => {
      if (req.dry_run) {
        onEvent({ type: 'estimate', estimate: { calls: 4, priced: false } });
        return;
      }
      onEvent({ type: 'step', step: { name: 'shape', status: 'done' } });
      onEvent({ type: 'step', step: { name: 'schema', status: 'done' } });
      onEvent({ type: 'step', step: { name: 'verify', status: 'done' } });
      onEvent({ type: 'draft', system_draft: draft });
    });

    const onDraft = vi.fn();
    render(<SystemGenerateDialog onCancel={() => {}} onDraft={onDraft} estimateDelayMs={0} />);

    fireEvent.change(screen.getByLabelText(/system brief/i), {
      target: { value: 'a steampunk d20 system' },
    });

    await waitFor(() =>
      expect(screen.getByTestId('generation-estimate')).toHaveTextContent('4 calls')
    );
    expect(screen.getByTestId('generation-estimate')).toHaveTextContent('unpriced');

    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));

    await waitFor(() => expect(screen.getByText('Mechanical shape')).toBeInTheDocument());
    expect(screen.getByText('Stats, skills and checks')).toBeInTheDocument();
    expect(screen.getByText('Smoke test verification')).toBeInTheDocument();
    await waitFor(() => expect(onDraft).toHaveBeenCalledWith(draft));
  });

  it('requires confirmation for a large estimate', async () => {
    const generate = vi.spyOn(APIClient, 'generateSystem').mockImplementation(async (req, onEvent) => {
      if (req.dry_run) {
        onEvent({ type: 'estimate', estimate: { calls: 20, priced: true, cost_micros: 5_000_000 } });
        return;
      }
      onEvent({ type: 'draft', system_draft: draft });
    });

    const onDraft = vi.fn();
    render(<SystemGenerateDialog onCancel={() => {}} onDraft={onDraft} estimateDelayMs={0} />);

    fireEvent.change(screen.getByLabelText(/system brief/i), { target: { value: 'a deep tactical system' } });
    await waitFor(() =>
      expect(screen.getByTestId('generation-estimate')).toHaveTextContent('$5.00')
    );

    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));
    await waitFor(() => expect(screen.getByTestId('generation-confirm')).toBeInTheDocument());
    expect(onDraft).not.toHaveBeenCalled();

    // The second click confirms
    fireEvent.click(screen.getByRole('button', { name: /generate anyway/i }));
    await waitFor(() => expect(onDraft).toHaveBeenCalledWith(draft));
    expect(generate).toHaveBeenCalledTimes(2);
  });

  it('offers to raise the limit in place when a generation hits one', async () => {
    const generate = vi.spyOn(APIClient, 'generateSystem').mockImplementation(async (req, onEvent) => {
      if (req.dry_run) {
        onEvent({ type: 'estimate', estimate: { calls: 4, priced: false } });
        return;
      }
      if (req.limits?.max_calls) {
        onEvent({ type: 'draft', system_draft: draft });
        return;
      }
      onEvent({
        type: 'error',
        message: 'generation call budget exceeded: 20 calls',
        code: 'generation_call_limit',
      });
    });
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
      config: { generation: { max_calls: 20 } },
    } as any);

    const onDraft = vi.fn();
    render(<SystemGenerateDialog onCancel={() => {}} onDraft={onDraft} estimateDelayMs={0} />);

    fireEvent.change(screen.getByLabelText(/system brief/i), { target: { value: 'a gritty system' } });
    await waitFor(() =>
      expect(screen.getByTestId('generation-estimate')).toHaveTextContent('4 calls')
    );

    fireEvent.click(screen.getByRole('button', { name: /^generate$/i }));

    const field = await screen.findByLabelText(/call limit/i);
    await waitFor(() => expect(field).toHaveValue(40));
    expect(screen.getByRole('button', { name: /use for this run/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /save and use/i })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /use for this run/i }));
    await waitFor(() => expect(onDraft).toHaveBeenCalledWith(draft));
    expect(generate.mock.calls[generate.mock.calls.length - 1][0].limits).toEqual({ max_calls: 40 });
  });
});
