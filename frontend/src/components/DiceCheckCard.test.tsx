import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DiceCheckCard } from './DiceCheckCard';
import { TurnCheck } from '../types';

const base: TurnCheck = {
  check_id: 'c1',
  outcome: 'weak',
  roll: { notation: '2d6', total: 9 },
};

describe('DiceCheckCard', () => {
  it('shows the profile and stakes', () => {
    render(
      <DiceCheckCard
        check={{ ...base, profile: 'blades', position: 'risky', effect: 'limited', successes: 2 }}
      />,
    );
    expect(screen.getByText('blades')).toBeInTheDocument();
    expect(screen.getByText('risky')).toBeInTheDocument();
    expect(screen.getByText('limited')).toBeInTheDocument();
  });

  it('omits the stakes line when there is none', () => {
    render(<DiceCheckCard check={base} />);
    expect(screen.queryByText('risky')).toBeNull();
    expect(screen.queryByText('blades')).toBeNull();
  });
});
