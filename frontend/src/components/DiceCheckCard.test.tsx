import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DiceCheckCard } from './DiceCheckCard';
import { TurnCheck } from '../types';

const check = (overrides: Partial<TurnCheck> = {}): TurnCheck => ({
  check_id: 'c1',
  outcome: 'weak',
  roll: { notation: '2d6', total: 9 },
  ...overrides,
});

describe('DiceCheckCard', () => {
  it('shows the profile and stakes', () => {
    render(
      <DiceCheckCard
        check={check({ profile: 'blades', position: 'risky', effect: 'limited', successes: 2 })}
      />,
    );
    expect(screen.getByText('blades')).toBeInTheDocument();
    expect(screen.getByText('risky')).toBeInTheDocument();
    expect(screen.getByText('limited')).toBeInTheDocument();
  });

  it('shows the modifier breakdown', () => {
    render(
      <DiceCheckCard
        check={check({ applied: [{ source: 'Stealth', value: 3 }, { source: 'wounded', value: -2 }] })}
      />,
    );
    expect(screen.getByText(/Stealth/)).toBeInTheDocument();
    expect(screen.getByText(/-2/)).toBeInTheDocument();
  });

  it('shows stakes and outcome text', () => {
    render(
      <DiceCheckCard
        check={check({ stakes: 'the bridge holds', outcome: 'weak', outcome_text: 'you succeed, at a cost' })}
      />,
    );
    expect(screen.getByText(/the bridge holds/)).toBeInTheDocument();
    expect(screen.getByText(/at a cost/)).toBeInTheDocument();
  });

  it('simple check renders compactly', () => {
    const { container } = render(<DiceCheckCard check={check({ outcome: 'strong' })} />);
    expect(container.querySelectorAll('[data-section]').length).toBeLessThanOrEqual(2);
  });

  it('omits the profile line when there is none', () => {
    render(<DiceCheckCard check={check()} />);
    expect(screen.queryByText('risky')).toBeNull();
    expect(screen.queryByText('blades')).toBeNull();
  });
});
