import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ContextDrawer } from './ContextDrawer';
import { APIClient } from '../api/client';
import type { TurnContext, WorkingEntry } from '../types';

const mockContext: TurnContext = {
  turn_number: 5,
  mode: 'action',
  strategy: 'cached_prefix',
  estimated_tokens: 1250,
  budget: 4000,
  cached_tokens: 800,
  prompt_hash: 'abcdef1234567890abcdef',
  prefix_hash: '123456abcdef7890123456',
  session: {
    provider: 'test-provider',
    id: 'sess-001',
    through_turn: 4,
  },
  refs: [],
  working_set: [],
  summary_version: 1,
  sections: [
    {
      name: 'System Prompt',
      included: true,
      tokens: 400,
      source: 'rules',
      refs: [{ id: 'combat-rules', kind: 'rule', relation: 'enforces' }],
    },
    {
      name: 'Memory Recall',
      included: false,
      tokens: 150,
    },
  ],
  prompt: 'You are the Game Master narrating a grim fantasy world.',
};

const mockWorkingSet: WorkingEntry[] = [
  {
    id: 'sir-aldous',
    name: 'Sir Aldous',
    kind: 'npc',
    role: 'ally',
    last_turn: 4,
    weight: 0.9,
  },
  {
    id: 'black-dagger',
    name: 'The Black Dagger',
    kind: 'item',
    last_turn: 3,
    weight: 0.6,
  },
];

describe('ContextDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(APIClient, 'listGames').mockResolvedValue([
      {
        id: 'game-1',
        name: 'Curse of Strahd',
        system_id: 'dnd5e',
        world_id: 'barovia',
        player_name: 'Hero',
        turn_count: 5,
        last_played: '',
        thumbnail_url: '',
      },
    ]);
    vi.spyOn(APIClient, 'getTurnContext').mockResolvedValue(mockContext);
    vi.spyOn(APIClient, 'getWorkingSet').mockResolvedValue(mockWorkingSet);
  });

  it('loads and renders turn context and working set for specified game', async () => {
    const onWikilink = vi.fn();
    render(<ContextDrawer gameID="game-1" onSelectWikilink={onWikilink} />);

    await waitFor(() => {
      expect(screen.getByText(/Turn 5 \(action\)/)).toBeInTheDocument();
    });

    expect(screen.getByText('cached_prefix')).toBeInTheDocument();
    expect(screen.getByText('800 cached tokens')).toBeInTheDocument();
    expect(screen.getByText(/Tokens:/)).toBeInTheDocument();
    expect(screen.getByText('1250')).toBeInTheDocument();

    // Check working set entries
    expect(screen.getByText('Sir Aldous')).toBeInTheDocument();
    expect(screen.getByText('The Black Dagger')).toBeInTheDocument();

    // Click working set item
    fireEvent.click(screen.getByText('Sir Aldous'));
    expect(onWikilink).toHaveBeenCalledWith('sir-aldous');

    // Check sections
    expect(screen.getByText('System Prompt')).toBeInTheDocument();
    expect(screen.getByText('~400 tokens')).toBeInTheDocument();
    expect(screen.getByText('combat-rules')).toBeInTheDocument();

    // Click wikilink ref in section
    fireEvent.click(screen.getByText('combat-rules'));
    expect(onWikilink).toHaveBeenCalledWith('combat-rules');
  });

  it('toggles assembled prompt snapshot view', async () => {
    render(<ContextDrawer gameID="game-1" />);

    await waitFor(() => {
      expect(screen.getByText('Assembled Prompt Snapshot')).toBeInTheDocument();
    });

    expect(screen.queryByText('You are the Game Master narrating a grim fantasy world.')).not.toBeInTheDocument();

    fireEvent.click(screen.getByText('Assembled Prompt Snapshot'));
    expect(screen.getByText('You are the Game Master narrating a grim fantasy world.')).toBeInTheDocument();

    fireEvent.click(screen.getByText('Assembled Prompt Snapshot'));
    expect(screen.queryByText('You are the Game Master narrating a grim fantasy world.')).not.toBeInTheDocument();
  });

  it('displays empty state message when no context or working set available', async () => {
    vi.spyOn(APIClient, 'getTurnContext').mockResolvedValue(null as never);
    vi.spyOn(APIClient, 'getWorkingSet').mockResolvedValue([]);

    render(<ContextDrawer gameID="game-1" />);

    await waitFor(() => {
      expect(screen.getByText('No turn context or working set available yet for this campaign.')).toBeInTheDocument();
    });
  });

  it('refreshes context data when Refresh button is clicked', async () => {
    render(<ContextDrawer gameID="game-1" />);

    await waitFor(() => {
      expect(screen.getByText('Refresh')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Refresh'));
    expect(APIClient.getTurnContext).toHaveBeenCalledTimes(2);
  });
});
