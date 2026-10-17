import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ExportModal } from './ExportModal';
import { expectNoViolations } from '../test/a11y';
import { APIClient } from '../api/client';
import type { ExportEvent } from '../types';

let eventSubscriber: ((event: ExportEvent) => void) | null = null;

vi.mock('../api/client', () => ({
  APIClient: {
    exportCapabilities: vi.fn(),
    subscribeExportEvents: vi.fn((cb) => {
      eventSubscriber = cb;
      return () => {
        eventSubscriber = null;
      };
    }),
    startExport: vi.fn(),
    cancelExport: vi.fn(),
  },
}));

function renderModal(overrides: { gameID?: string | null; onClose?: () => void } = {}) {
  return render(
    <ExportModal
      isOpen
      gameID={overrides.gameID !== undefined ? overrides.gameID : 'campaign-01'}
      onClose={overrides.onClose ?? (() => undefined)}
    />,
  );
}
function makeExportEvent(partial: Partial<ExportEvent>): ExportEvent {
  return {
    game_id: 'campaign-01',
    format: 'web',
    phase: 'frames',
    done: 0,
    total: 10,
    frames: 0,
    image_frames: 0,
    repeat_frames: 0,
    audio_packets: 0,
    total_audio_packets: 0,
    audio_bytes: 0,
    total_audio_bytes: 0,
    elapsed_ms: 0,
    length_ms: 0,
    ...partial,
  };
}

describe('ExportModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    eventSubscriber = null;
    vi.mocked(APIClient.exportCapabilities).mockResolvedValue({
      default_dir: '/tmp/exports',
      native_dialog: false,
    });
    vi.mocked(APIClient.startExport).mockResolvedValue({
      game_id: 'campaign-01',
      format: 'web',
      output_path: '/tmp/exports',
      running: true,
    });
    vi.mocked(APIClient.cancelExport).mockResolvedValue(undefined);
  });

  describe('accessibility and dialog behaviour', () => {
    it('is a dialog named by its title', async () => {
      const { container } = renderModal();
      expect(screen.getByRole('dialog')).toHaveAccessibleName('Export Story Replay');
      await expectNoViolations(container);
    });

    it('takes focus into the dialog', () => {
      renderModal();
      expect(screen.getByRole('dialog')).toContainElement(document.activeElement as HTMLElement);
    });

    it('closes on Escape', async () => {
      const onClose = vi.fn();
      renderModal({ onClose });
      await userEvent.keyboard('{Escape}');
      expect(onClose).toHaveBeenCalled();
    });
  });

  describe('empty and missing campaign state', () => {
    it('prompts to open a campaign when gameID is null and disables export button', () => {
      renderModal({ gameID: null });
      expect(screen.getByText('Open a campaign before exporting.')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /Start export/i })).toBeDisabled();
    });
  });

  describe('error states', () => {
    it('displays error when exportCapabilities check fails on mount', async () => {
      vi.mocked(APIClient.exportCapabilities).mockRejectedValue(new Error('Export subsystem offline'));

      renderModal();

      await waitFor(() => {
        expect(screen.getByText('Export subsystem offline')).toBeInTheDocument();
      });
    });

    it('displays error when startExport fails', async () => {
      vi.mocked(APIClient.startExport).mockRejectedValue(new Error('Destination disk is full'));

      renderModal();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Start export/i })).not.toBeDisabled();
      });

      fireEvent.click(screen.getByRole('button', { name: /Start export/i }));

      await waitFor(() => {
        expect(screen.getByText('Destination disk is full')).toBeInTheDocument();
      });
    });

    it('displays error banner when an export error event arrives', async () => {
      renderModal();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Start export/i })).not.toBeDisabled();
      });

      fireEvent.click(screen.getByRole('button', { name: /Start export/i }));

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Cancel/i })).toBeInTheDocument();
      });

      expect(eventSubscriber).not.toBeNull();
      eventSubscriber!(
        makeExportEvent({
          game_id: 'campaign-01',
          phase: 'error',
          done: 0,
          total: 10,
          error: 'FFmpeg encoding crashed',
        }),
      );

      await waitFor(() => {
        expect(screen.getByText('FFmpeg encoding crashed')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /Start export/i })).toBeInTheDocument();
      });
    });
  });

  describe('running and completed states', () => {
    it('shows progress and logs when events stream, and can be cancelled', async () => {
      renderModal();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Start export/i })).not.toBeDisabled();
      });

      fireEvent.click(screen.getByRole('button', { name: /Start export/i }));

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Cancel/i })).toBeInTheDocument();
      });

      // Send progress event
      eventSubscriber!(
        makeExportEvent({
          game_id: 'campaign-01',
          phase: 'frames',
          message: 'Rendering video frames',
          done: 5,
          total: 10,
          frames: 5,
          image_frames: 3,
          repeat_frames: 2,
          elapsed_ms: 3000,
        }),
      );

      await waitFor(() => {
        expect(screen.getAllByText(/Rendering video frames/)).toHaveLength(2);
        expect(screen.getByText('50%')).toBeInTheDocument();
      });

      // Click cancel
      fireEvent.click(screen.getByRole('button', { name: /Cancel/i }));
      expect(APIClient.cancelExport).toHaveBeenCalledWith('campaign-01');
    });

    it('displays success message when export completes', async () => {
      renderModal();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Start export/i })).not.toBeDisabled();
      });

      fireEvent.click(screen.getByRole('button', { name: /Start export/i }));

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /Cancel/i })).toBeInTheDocument();
      });

      eventSubscriber!(
        makeExportEvent({
          game_id: 'campaign-01',
          phase: 'done',
          message: 'Finished',
          done: 10,
          total: 10,
          output_path: '/tmp/exports/campaign-01.html',
        }),
      );

      await waitFor(() => {
        expect(screen.getByText(/Exported to/)).toBeInTheDocument();
        expect(screen.getByText('/tmp/exports/campaign-01.html')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /Start export/i })).toBeInTheDocument();
      });
    });
  });
});
