import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { CharacterSheetDrawer } from './CharacterSheetDrawer';
import type { PlayerState } from '../types';

describe('CharacterSheetDrawer', () => {
  it('renders authored appearance and current appearance separately', () => {
    const player: PlayerState = {
      id: 'kaelen',
      name: 'Kaelen',
      type: 'character',
      state: {},
      appearance: 'Silver hair and blast goggles.',
      current_appearance: 'Soaked through from the crossing.',
    };

    render(<CharacterSheetDrawer player={player} />);

    expect(screen.getByText('Silver hair and blast goggles.')).toBeInTheDocument();
    expect(screen.getByText(/Currently:/i)).toBeInTheDocument();
    expect(screen.getByText(/Soaked through from the crossing\./i)).toBeInTheDocument();
  });

  it('renders only baseline appearance when current appearance is unset', () => {
    const player: PlayerState = {
      id: 'kaelen',
      name: 'Kaelen',
      type: 'character',
      state: {},
      appearance: 'Silver hair and blast goggles.',
    };

    render(<CharacterSheetDrawer player={player} />);

    expect(screen.getByText('Silver hair and blast goggles.')).toBeInTheDocument();
    expect(screen.queryByText(/Currently:/i)).not.toBeInTheDocument();
  });
});
