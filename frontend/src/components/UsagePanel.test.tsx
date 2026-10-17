import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { UsagePanel } from './UsagePanel';
import { APIClient } from '../api/client';
import type { Usage } from '../types';

const emptyUsage: Usage = {
  currency: 'USD',
  total_cost_micros: 0,
  by_provider: {},
  by_role: {},
  rows: [],
};

const populatedUsage: Usage = {
  currency: 'USD',
  total_cost_micros: 150000,
  by_provider: {
    gemini: 100000,
    openai: 50000,
  },
  by_role: {
    gm: 120000,
    extractor: 30000,
  },
  campaigns: [
    {
      game_id: 'campaign-1',
      name: 'Curse of Strahd',
      total_cost_micros: 150000,
    },
  ],
  rows: [
    {
      turn_number: 1,
      role: 'gm',
      provider: 'gemini',
      model: 'gemini-1.5-flash',
      input_tokens: 500,
      output_tokens: 150,
      cost_micros: 100000,
    },
    {
      turn_number: 2,
      role: 'extractor',
      provider: 'openai',
      model: 'gpt-4o-mini',
      characters: 800,
      requests: 1,
      cost_micros: 0, // unpriced
    },
  ],
};

describe('UsagePanel', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(APIClient, 'getGlobalUsage').mockResolvedValue(emptyUsage);
    vi.spyOn(APIClient, 'getGameUsage').mockResolvedValue(emptyUsage);
  });

  describe('loading state', () => {
    it('disables the refresh button while loading', async () => {
      let resolveUsage: (value: Usage) => void = () => {};
      const pendingPromise = new Promise<Usage>((resolve) => {
        resolveUsage = resolve;
      });
      vi.spyOn(APIClient, 'getGlobalUsage').mockReturnValue(pendingPromise);

      render(<UsagePanel />);

      const refreshButton = screen.getByTitle('Refresh usage data');
      expect(refreshButton).toBeDisabled();

      resolveUsage(emptyUsage);
      await waitFor(() => {
        expect(refreshButton).not.toBeDisabled();
      });
    });
  });

  describe('error state', () => {
    it('displays an error alert when loading fails', async () => {
      vi.spyOn(APIClient, 'getGlobalUsage').mockRejectedValue(new Error('Network connection failed'));

      render(<UsagePanel />);

      await waitFor(() => {
        expect(screen.getByText('Network connection failed')).toBeInTheDocument();
      });
    });
  });

  describe('empty states', () => {
    it('shows empty breakdown labels and empty ledger message when no usage records exist', async () => {
      vi.spyOn(APIClient, 'getGlobalUsage').mockResolvedValue(emptyUsage);

      render(<UsagePanel activeGameID="test-game" />);

      await waitFor(() => {
        expect(screen.getByText('No provider breakdown')).toBeInTheDocument();
        expect(screen.getByText('No role breakdown')).toBeInTheDocument();
        expect(screen.getByText('No usage records logged for this scope.')).toBeInTheDocument();
      });
      expect(screen.getByText('0.00 USD')).toBeInTheDocument();
    });

    it('shows filter mismatch message when filters filter out all rows', async () => {
      vi.spyOn(APIClient, 'getGameUsage').mockResolvedValue(populatedUsage);

      render(<UsagePanel activeGameID="campaign-1" />);

      await waitFor(() => {
        expect(screen.getByText('#1')).toBeInTheDocument();
      });

      // Filter by a provider that does not match row 2 when filtering for specific role
      const roleSelect = screen.getByDisplayValue('All Roles');
      fireEvent.change(roleSelect, { target: { value: 'nonexistent-role' } });

      await waitFor(() => {
        expect(screen.getByText('No usage records match the current filter.')).toBeInTheDocument();
      });
    });

    it('shows "no price configured" button for unpriced consumption and invokes onOpenDocs', async () => {
      vi.spyOn(APIClient, 'getGameUsage').mockResolvedValue(populatedUsage);
      const onOpenDocs = vi.fn();

      render(<UsagePanel activeGameID="campaign-1" onOpenDocs={onOpenDocs} />);

      const unpricedBtn = await screen.findByRole('button', { name: /no price configured/i });
      expect(unpricedBtn).toBeInTheDocument();

      fireEvent.click(unpricedBtn);
      expect(onOpenDocs).toHaveBeenCalledWith('11-usage-and-pricing');
    });

    it('supports drilldown into a campaign and navigating back to all campaigns', async () => {
      vi.spyOn(APIClient, 'getGlobalUsage').mockResolvedValue(populatedUsage);
      const getGameUsageSpy = vi.spyOn(APIClient, 'getGameUsage').mockResolvedValue(populatedUsage);

      render(<UsagePanel />);

      await waitFor(() => {
        expect(screen.getByText('Curse of Strahd')).toBeInTheDocument();
      });

      fireEvent.click(screen.getByText('Curse of Strahd'));

      await waitFor(() => {
        expect(getGameUsageSpy).toHaveBeenCalledWith('campaign-1');
        expect(screen.getByText('Back to all campaigns')).toBeInTheDocument();
        expect(screen.getByText('Campaign: campaign-1')).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole('button', { name: /Back to all campaigns/i }));

      await waitFor(() => {
        expect(screen.queryByText('Back to all campaigns')).not.toBeInTheDocument();
      });
    });
  });
});
