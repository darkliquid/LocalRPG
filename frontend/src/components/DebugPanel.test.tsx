import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { DebugPanel } from './DebugPanel';
import { APIClient } from '../api/client';
import type { AppConfig, TraceEvent } from '../types';

vi.mock('./ContextDrawer', () => ({
  ContextDrawer: ({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) =>
    isOpen ? (
      <div data-testid="context-drawer-modal">
        <button onClick={onClose}>Close Context Drawer</button>
      </div>
    ) : null,
}));

const mockConfig: AppConfig = {
  version: '1',
  paths: { systems: '', worlds: '', games: '', cache: '' },
  agents: {
    default_role: 'gm',
    roles: {},
    trace_payload_chars: 20000,
    trace_max_bytes: 268435456,
    trace_max_files: 3,
    trace_rotate_check: 200,
    trace_chunk_limit: 500,
  },
  media: {
    tts: { type: 'disabled', auto_play: false, master_volume: 1 },
    stt: { type: 'disabled' },
    image: { type: 'disabled', auto_generate: false },
  },
  preferences: {
    streaming: false,
    typing_speed_ms: 0,
    cinematic_effects: false,
    font_scale: 'medium',
    trace_level: 'off',
  },
};

const mockEvents: TraceEvent[] = [
  {
    ts: '2026-10-17T20:00:00Z',
    event: 'turn.dispatch',
    level: 'summary',
    fields: { turn: 1, mode: 'action' },
  },
  {
    ts: '2026-10-17T20:00:01Z',
    event: 'provider.wire',
    level: 'full',
    fields: { line: 'data: {"candidates": []}' },
  },
];

describe('DebugPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(APIClient, 'traceEvents').mockResolvedValue(mockEvents);
    vi.spyOn(APIClient, 'clearTrace').mockResolvedValue(undefined);
  });

  it('renders trace level options and calls setConfig on change', async () => {
    const setConfig = vi.fn();
    render(<DebugPanel config={mockConfig} setConfig={setConfig} />);

    await waitFor(() => {
      expect(screen.getByText('Trace & Debug')).toBeInTheDocument();
    });

    const select = screen.getByRole('combobox');
    fireEvent.change(select, { target: { value: 'full' } });

    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      preferences: {
        ...mockConfig.preferences,
        trace_level: 'full',
      },
    });
  });

  it('renders trace events and raw provider lines', async () => {
    render(<DebugPanel config={mockConfig} setConfig={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByText('turn.dispatch')).toBeInTheDocument();
    });

    expect(screen.getByText('provider.wire')).toBeInTheDocument();
    expect(screen.getByText(/data: \{"candidates": \[\]\}/)).toBeInTheDocument();
  });

  it('displays empty message when no events are recorded', async () => {
    vi.spyOn(APIClient, 'traceEvents').mockResolvedValue([]);
    render(<DebugPanel config={mockConfig} setConfig={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByText(/Nothing recorded\. Set a trace level/)).toBeInTheDocument();
    });
  });

  it('displays error message when traceEvents fails', async () => {
    vi.spyOn(APIClient, 'traceEvents').mockRejectedValue(new Error('Permission denied on trace file'));
    render(<DebugPanel config={mockConfig} setConfig={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByText('Permission denied on trace file')).toBeInTheDocument();
    });
  });

  it('clears trace when Clear button is clicked', async () => {
    render(<DebugPanel config={mockConfig} setConfig={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByText('Clear')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Clear'));

    await waitFor(() => {
      expect(APIClient.clearTrace).toHaveBeenCalled();
      expect(screen.getByText(/Nothing recorded\. Set a trace level/)).toBeInTheDocument();
    });
  });

  it('toggles follow state and opens ContextDrawer', async () => {
    render(<DebugPanel config={mockConfig} setConfig={vi.fn()} />);

    const followBtn = screen.getByRole('button', { name: /Follow/i });
    expect(followBtn).toHaveTextContent('Follow');

    fireEvent.click(followBtn);
    expect(screen.getByRole('button', { name: /Following/i })).toHaveTextContent('Following');

    // Open context drawer
    fireEvent.click(screen.getByRole('button', { name: /Context & Continuity/i }));
    expect(screen.getByTestId('context-drawer-modal')).toBeInTheDocument();

    fireEvent.click(screen.getByText('Close Context Drawer'));
    expect(screen.queryByTestId('context-drawer-modal')).not.toBeInTheDocument();
  });
});
