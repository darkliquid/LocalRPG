import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { LivingWorldDrawer } from './LivingWorldDrawer';
import type { GameState, Recap } from '../types';

describe('LivingWorldDrawer', () => {
  describe('empty states', () => {
    it('shows message when summaries are disabled in settings', () => {
      const recap: Recap = {
        enabled: false,
        summary: '',
        through_turn: 0,
        threads: [],
      };

      render(<LivingWorldDrawer recap={recap} />);

      expect(screen.getByText('Summaries are switched off in Settings.')).toBeInTheDocument();
      expect(screen.getByText('No active world narrative arcs registered.')).toBeInTheDocument();
      expect(screen.getByText('All factions are quiet in the shadows.')).toBeInTheDocument();
    });

    it('shows message when story is not far enough for summary', () => {
      const recap: Recap = {
        enabled: true,
        summary: '',
        through_turn: 0,
        threads: [],
      };

      render(<LivingWorldDrawer recap={recap} />);

      expect(screen.getByText('The story has not turned far enough to be summarised yet.')).toBeInTheDocument();
    });

    it('shows empty arcs and clocks messages when state is empty', () => {
      const state: GameState = {
        game_id: 'game-1',
        game_name: 'Test Campaign',
        player: { id: 'p1', name: 'Hero', type: 'pc', state: {} },
        locations: [],
        arcs: [],
        clocks: [],
      };

      render(<LivingWorldDrawer state={state} />);

      expect(screen.getByText('No active world narrative arcs registered.')).toBeInTheDocument();
      expect(screen.getByText('All factions are quiet in the shadows.')).toBeInTheDocument();
    });
  });

  describe('populated states', () => {
    it('renders summary and calls onRefreshRecap when refresh button is clicked', () => {
      const onRefresh = vi.fn();
      const recap: Recap = {
        enabled: true,
        summary: 'The adventurers entered the dark forest.',
        through_turn: 5,
        threads: [],
      };

      render(<LivingWorldDrawer recap={recap} onRefreshRecap={onRefresh} />);

      expect(screen.getByText('The adventurers entered the dark forest.')).toBeInTheDocument();
      expect(screen.getByText('Through turn 5')).toBeInTheDocument();

      const refreshBtn = screen.getByRole('button', { name: 'Refresh' });
      fireEvent.click(refreshBtn);
      expect(onRefresh).toHaveBeenCalled();
    });

    it('renders threads with idle status and highlights stale threads', () => {
      const recap: Recap = {
        enabled: true,
        summary: 'Story summary.',
        through_turn: 12,
        threads: [
          {
            id: 'thread-active',
            name: 'Rescue the blacksmith',
            status: 'investigating',
            idle: 2,
            last_advanced: 10,
          },
          {
            id: 'thread-stale',
            name: 'Find the ancient artifact',
            status: 'dormant',
            idle: 15,
            last_advanced: 1,
          },
        ],
      };

      render(<LivingWorldDrawer recap={recap} idleTurns={10} />);

      expect(screen.getByText('Rescue the blacksmith')).toBeInTheDocument();
      expect(screen.getByText('investigating')).toBeInTheDocument();
      expect(screen.getByText('Last advanced at turn 10 (2 turns ago)')).toBeInTheDocument();

      expect(screen.getByText('Find the ancient artifact')).toBeInTheDocument();
      expect(screen.getByText('dormant')).toBeInTheDocument();
      expect(screen.getByText(/Last advanced at turn 1 \(15 turns ago\) - worth returning to/)).toBeInTheDocument();
    });

    it('renders narrative arcs with progress fractions', () => {
      const state: GameState = {
        game_id: 'game-1',
        game_name: 'Test Campaign',
        player: { id: 'p1', name: 'Hero', type: 'pc', state: {} },
        locations: [],
        clocks: [],
        arcs: [
          {
            id: 'arc-1',
            name: 'The Siege of Shadowkeep',
            progress: 3,
            max_progress: 10,
            status: 'active',
          },
        ],
      };

      render(<LivingWorldDrawer state={state} />);

      expect(screen.getByText('The Siege of Shadowkeep')).toBeInTheDocument();
      expect(screen.getByText('3/10')).toBeInTheDocument();
    });

    it('renders faction clocks with ticks and faction labels', () => {
      const state: GameState = {
        game_id: 'game-1',
        game_name: 'Test Campaign',
        player: { id: 'p1', name: 'Hero', type: 'pc', state: {} },
        locations: [],
        arcs: [],
        clocks: [
          {
            name: 'Cult Mobilisation',
            faction: 'Cult of the Dragon',
            ticks: 4,
            max_ticks: 6,
          },
        ],
      };

      render(<LivingWorldDrawer state={state} />);

      expect(screen.getByText('Cult Mobilisation (Cult of the Dragon)')).toBeInTheDocument();
      expect(screen.getByText('4/6')).toBeInTheDocument();
    });
  });
});
