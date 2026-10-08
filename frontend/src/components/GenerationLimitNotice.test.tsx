import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { GenerationLimitNotice } from './GenerationLimitNotice';
import { APIClient } from '../api/client';
import { callLimitCode, sourceLimitCode } from '../lib/generationLimit';
import { SettingsResponse } from '../types';

afterEach(() => {
  vi.restoreAllMocks();
});

const settings = (generation: Record<string, number>) =>
  ({ config: { generation } }) as unknown as SettingsResponse;

describe('GenerationLimitNotice', () => {
  it('suggests double the current limit and offers both choices', async () => {
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue(settings({ max_chunks: 200 }));
    const onRetry = vi.fn();
    const onSave = vi.fn();

    render(
      <GenerationLimitNotice
        code={sourceLimitCode}
        message="the source is larger than the chunk limit: it holds 300 chunks and the limit is 200"
        onRetry={onRetry}
        onSave={onSave}
      />
    );

    const field = screen.getByLabelText(/source chunk limit/i);
    await waitFor(() => expect(field).toHaveValue(400));
    expect(screen.getByText(/currently 200/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /use for this run/i }));
    expect(onRetry).toHaveBeenCalledWith({ max_chunks: 400 });

    fireEvent.click(screen.getByRole('button', { name: /save and use/i }));
    expect(onSave).toHaveBeenCalledWith({ max_chunks: 400 });
  });

  it('asks for the field that caused the failure', async () => {
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue(settings({ max_calls: 20 }));

    render(
      <GenerationLimitNotice code={callLimitCode} message="too many calls" onRetry={vi.fn()} onSave={vi.fn()} />
    );

    const field = screen.getByLabelText(/call limit/i);
    await waitFor(() => expect(field).toHaveValue(40));
    expect(screen.queryByLabelText(/source chunk limit/i)).not.toBeInTheDocument();
  });

  it('refuses a limit that would not help', async () => {
    vi.spyOn(APIClient, 'getSettings').mockResolvedValue(settings({ max_chunks: 200 }));
    const onRetry = vi.fn();

    render(
      <GenerationLimitNotice code={sourceLimitCode} message="too big" onRetry={onRetry} onSave={vi.fn()} />
    );

    const field = screen.getByLabelText(/source chunk limit/i);
    await waitFor(() => expect(field).toHaveValue(400));

    fireEvent.change(field, { target: { value: '0' } });
    fireEvent.click(screen.getByRole('button', { name: /use for this run/i }));

    expect(onRetry).not.toHaveBeenCalled();
    expect(screen.getByText(/greater than zero/i)).toBeInTheDocument();
  });

  it('stays usable when the current limit cannot be read', async () => {
    vi.spyOn(APIClient, 'getSettings').mockRejectedValue(new Error('offline'));
    const onRetry = vi.fn();

    render(
      <GenerationLimitNotice code={sourceLimitCode} message="too big" onRetry={onRetry} onSave={vi.fn()} />
    );

    const field = screen.getByLabelText(/source chunk limit/i);
    await waitFor(() => expect(field).toHaveValue(400));

    fireEvent.click(screen.getByRole('button', { name: /use for this run/i }));
    expect(onRetry).toHaveBeenCalledWith({ max_chunks: 400 });
  });
});
