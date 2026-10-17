import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { TTSBatchPanel } from './TTSBatchPanel';
import { APIClient } from '../api/client';
import type { GameSummary, TTSBatchJob } from '../types';

const mockGames: GameSummary[] = [
  {
    id: 'game-1',
    name: 'Curse of Strahd',
    system_id: 'dnd5e',
    world_id: 'barovia',
    player_name: 'Hero',
    turn_count: 5,
    last_played: '2026-01-01T00:00:00Z',
    thumbnail_url: '',
  },
  {
    id: 'game-2',
    name: 'Tomb of Annihilation',
    system_id: 'dnd5e',
    world_id: 'chult',
    player_name: 'Hero',
    turn_count: 3,
    last_played: '2026-01-01T00:00:00Z',
    thumbnail_url: '',
  },
];

const activeJob: TTSBatchJob = {
  id: 'job-1',
  game_id: 'game-1',
  game_name: 'Curse of Strahd',
  provider: 'gemini',
  model: 'gemini-2.0-flash',
  status: 'processing',
  request_count: 10,
  completed: 4,
  failed_keys: [],
};

const failedJob: TTSBatchJob = {
  id: 'job-2',
  game_id: 'game-1',
  game_name: 'Curse of Strahd',
  provider: 'gemini',
  status: 'failed',
  request_count: 5,
  completed: 2,
  failed_keys: ['key-1', 'key-2'],
  last_error: 'Provider quota exhausted',
};

const completedJob: TTSBatchJob = {
  id: 'job-3',
  game_id: 'game-2',
  game_name: 'Tomb of Annihilation',
  provider: 'kokoro',
  status: 'completed',
  request_count: 8,
  completed: 8,
  failed_keys: [],
};

describe('TTSBatchPanel', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockResolvedValue([]);
    vi.spyOn(APIClient, 'listGames').mockResolvedValue(mockGames);
  });

  describe('loading state', () => {
    it('shows a loading spinner while refreshing', async () => {
      let resolveJobs: (jobs: TTSBatchJob[]) => void = () => {};
      const pendingPromise = new Promise<TTSBatchJob[]>((resolve) => {
        resolveJobs = resolve;
      });
      vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockReturnValue(pendingPromise);

      render(<TTSBatchPanel />);

      const refreshBtn = screen.getByRole('button', { name: 'Refresh jobs' });
      expect(refreshBtn.querySelector('.animate-spin')).toBeInTheDocument();

      resolveJobs([]);
      await waitFor(() => {
        expect(refreshBtn.querySelector('.animate-spin')).not.toBeInTheDocument();
      });
    });
  });

  describe('error states', () => {
    it('displays error message when listAllTTSBatchJobs fails', async () => {
      vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockRejectedValue(new Error('Failed to reach batch service'));

      render(<TTSBatchPanel />);

      await waitFor(() => {
        expect(screen.getByText('Failed to reach batch service')).toBeInTheDocument();
      });
    });

    it('displays error when startTTSBatch reports all clips already cached', async () => {
      vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockResolvedValue([]);
      vi.spyOn(APIClient, 'startTTSBatch').mockResolvedValue(null);

      render(<TTSBatchPanel />);

      await waitFor(() => {
        expect(screen.getByText('Backfill a campaign...')).toBeInTheDocument();
      });

      // Select campaign to backfill
      const selects = screen.getAllByRole('combobox');
      const startSelect = selects[1];
      fireEvent.change(startSelect, { target: { value: 'game-1' } });

      const startBtn = screen.getByRole('button', { name: 'Start' });
      expect(startBtn).not.toBeDisabled();
      fireEvent.click(startBtn);

      await waitFor(() => {
        expect(screen.getByText('Every clip is already cached for that campaign.')).toBeInTheDocument();
      });
    });

    it('displays error when clearFinished finds no completed jobs', async () => {
      vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockResolvedValue([activeJob]);
      vi.spyOn(APIClient, 'clearTTSBatchJobs').mockResolvedValue(0);

      render(<TTSBatchPanel />);

      await waitFor(() => {
        expect(screen.getByText('Processing')).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole('button', { name: 'Clear finished' }));

      await waitFor(() => {
        expect(screen.getByText('No finished jobs to clear.')).toBeInTheDocument();
      });
    });

    it('displays job-level last_error in the job card', async () => {
      vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockResolvedValue([failedJob]);

      render(<TTSBatchPanel />);

      await waitFor(() => {
        expect(screen.getByText('Provider quota exhausted')).toBeInTheDocument();
        expect(screen.getByText('Failed')).toBeInTheDocument();
      });
    });
  });

  describe('empty states', () => {
    it('displays "No batch jobs." when the job list is empty', async () => {
      vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockResolvedValue([]);

      render(<TTSBatchPanel />);

      await waitFor(() => {
        expect(screen.getByText('No batch jobs.')).toBeInTheDocument();
      });
    });

    it('displays "No batch jobs." when filtered by campaign with no jobs', async () => {
      vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockResolvedValue([completedJob]);

      render(<TTSBatchPanel />);

      await waitFor(() => {
        expect(screen.getByText('Completed')).toBeInTheDocument();
      });

      const filterSelect = screen.getAllByRole('combobox')[0];
      fireEvent.change(filterSelect, { target: { value: 'game-1' } });

      await waitFor(() => {
        expect(screen.getByText('No batch jobs.')).toBeInTheDocument();
      });
    });

    it('displays "none" for failed keys in expanded details when none failed', async () => {
      vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockResolvedValue([activeJob]);

      render(<TTSBatchPanel />);

      await waitFor(() => {
        expect(screen.getByTitle('Show job details')).toBeInTheDocument();
      });

      fireEvent.click(screen.getByTitle('Show job details'));

      await waitFor(() => {
        expect(screen.getByText('4 clips cached')).toBeInTheDocument();
        expect(screen.getByText('10 submitted')).toBeInTheDocument();
        expect(screen.getAllByText('none')).toHaveLength(2);
      });
    });
  });

  describe('actions and interactions', () => {
    it('cancels an active job when cancel button is clicked', async () => {
      vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockResolvedValue([activeJob]);
      const cancelSpy = vi.spyOn(APIClient, 'cancelTTSBatchJob').mockResolvedValue(undefined);

      render(<TTSBatchPanel />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Cancel this job' })).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole('button', { name: 'Cancel this job' }));

      await waitFor(() => {
        expect(cancelSpy).toHaveBeenCalledWith('game-1', 'job-1');
      });
    });

    it('deletes a finished job when delete button is clicked', async () => {
      vi.spyOn(APIClient, 'listAllTTSBatchJobs').mockResolvedValue([completedJob]);
      const deleteSpy = vi.spyOn(APIClient, 'deleteTTSBatchJob').mockResolvedValue(undefined);

      render(<TTSBatchPanel />);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Remove this finished job' })).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole('button', { name: 'Remove this finished job' }));

      await waitFor(() => {
        expect(deleteSpy).toHaveBeenCalledWith('game-2', 'job-3');
      });
    });
  });
});
