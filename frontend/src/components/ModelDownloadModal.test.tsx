import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ModelDownloadModal } from './ModelDownloadModal';
import { expectNoViolations } from '../test/a11y';
import { APIClient } from '../api/client';
import type { ModelStatus } from '../types';

let modelEventSubscriber: ((status: ModelStatus) => void) | null = null;

vi.mock('../api/client', () => ({
  APIClient: {
    subscribeModelEvents: vi.fn((cb) => {
      modelEventSubscriber = cb;
      return () => {
        modelEventSubscriber = null;
      };
    }),
    downloadModel: vi.fn(),
  },
}));

describe('ModelDownloadModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    modelEventSubscriber = null;
    vi.mocked(APIClient.downloadModel).mockResolvedValue(undefined);
  });

  describe('accessibility and dialog behaviour', () => {
    it('is a dialog named by its heading, and takes focus', async () => {
      const { container } = render(
        <ModelDownloadModal modelId="tts" modelName="Kokoro" sizeBytes={1000} onClose={() => undefined} />,
      );
      expect(screen.getByRole('dialog')).toHaveAccessibleName('Enable Voice Narration');
      expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus();
      await expectNoViolations(container);
    });

    it('closes on Escape', async () => {
      const onClose = vi.fn();
      render(<ModelDownloadModal modelId="tts" modelName="Kokoro" sizeBytes={1000} onClose={onClose} />);
      await userEvent.keyboard('{Escape}');
      expect(onClose).toHaveBeenCalled();
    });
  });

  describe('downloading and progress states', () => {
    it('displays progress bar, byte details, and disables cancel actions while downloading', async () => {
      render(
        <ModelDownloadModal
          modelId="tts-kokoro"
          modelName="Kokoro"
          sizeBytes={100 * 1024 * 1024}
          onClose={() => undefined}
        />,
      );

      const downloadBtn = screen.getByRole('button', { name: /Download \(100\.0 MB\)/i });
      fireEvent.click(downloadBtn);

      await waitFor(() => {
        expect(APIClient.downloadModel).toHaveBeenCalledWith('tts-kokoro');
      });

      // Emit progress event
      expect(modelEventSubscriber).not.toBeNull();
      modelEventSubscriber!({
        id: 'tts-kokoro',
        name: 'Kokoro',
        installed: false,
        downloading: true,
        progress: 0.45,
        bytes_downloaded: 45 * 1024 * 1024,
        total_bytes: 100 * 1024 * 1024,
      });

      await waitFor(() => {
        expect(screen.getAllByText('Downloading...')).toHaveLength(2);
        expect(screen.getByText('45%')).toBeInTheDocument();
        expect(screen.getByText('45.0 / 100.0 MB')).toBeInTheDocument();
        // Close button is hidden during download
        expect(screen.queryByRole('button', { name: 'Close' })).not.toBeInTheDocument();
        // Continue in Silence is disabled
        expect(screen.getByRole('button', { name: 'Continue in Silence' })).toBeDisabled();
      });
    });
  });

  describe('error states', () => {
    it('displays error message when downloadModel API rejects', async () => {
      vi.mocked(APIClient.downloadModel).mockRejectedValue(new Error('Connection refused'));

      render(
        <ModelDownloadModal
          modelId="tts-kokoro"
          modelName="Kokoro"
          sizeBytes={50 * 1024 * 1024}
          onClose={() => undefined}
        />,
      );

      fireEvent.click(screen.getByRole('button', { name: /Download/i }));

      await waitFor(() => {
        expect(screen.getByText('Connection refused')).toBeInTheDocument();
      });
    });

    it('displays error message when model status event contains error', async () => {
      render(
        <ModelDownloadModal
          modelId="tts-kokoro"
          modelName="Kokoro"
          sizeBytes={50 * 1024 * 1024}
          onClose={() => undefined}
        />,
      );

      expect(modelEventSubscriber).not.toBeNull();
      modelEventSubscriber!({
        id: 'tts-kokoro',
        name: 'Kokoro',
        installed: false,
        downloading: false,
        progress: 0,
        bytes_downloaded: 0,
        total_bytes: 50 * 1024 * 1024,
        error: 'Checksum verification failed',
      });

      await waitFor(() => {
        expect(screen.getByText('Checksum verification failed')).toBeInTheDocument();
      });
    });
  });

  describe('completed state', () => {
    it('displays completed message and invokes onClose when Done is clicked', async () => {
      const onClose = vi.fn();
      render(
        <ModelDownloadModal modelId="tts-kokoro" modelName="Kokoro" sizeBytes={50 * 1024 * 1024} onClose={onClose} />,
      );

      expect(modelEventSubscriber).not.toBeNull();
      modelEventSubscriber!({
        id: 'tts-kokoro',
        name: 'Kokoro',
        installed: true,
        downloading: false,
        progress: 1,
        bytes_downloaded: 50 * 1024 * 1024,
        total_bytes: 50 * 1024 * 1024,
      });

      await waitFor(() => {
        expect(screen.getByText('Voice pack installed! Future turns will narrate automatically.')).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole('button', { name: 'Done' }));
      expect(onClose).toHaveBeenCalled();
    });
  });
});
