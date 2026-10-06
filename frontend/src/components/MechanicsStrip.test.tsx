import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MechanicsStrip } from './MechanicsStrip';
import { TurnCheck } from '../types';

const check = (overrides: Partial<TurnCheck> = {}): TurnCheck => ({
  check_id: 'c1',
  outcome: 'weak',
  roll: { notation: '2d6', total: 9 },
  ...overrides,
});

const turn = (overrides: { engagement?: string; checks?: TurnCheck[] } = {}) => ({
  engagement: 'auto',
  checks: [] as TurnCheck[],
  ...overrides,
});

describe('MechanicsStrip', () => {
  it('summarises checks and engagement', () => {
    render(<MechanicsStrip turn={turn({ engagement: 'auto', checks: [check({ outcome: 'weak' })] })} />);
    expect(screen.getByText(/checks: 1/i)).toBeInTheDocument();
    expect(screen.getByText(/auto/)).toBeInTheDocument();
  });

  it('says none on a quiet turn', () => {
    render(<MechanicsStrip turn={turn({ engagement: 'auto', checks: [] })} />);
    expect(screen.getByText(/checks: none/i)).toBeInTheDocument();
  });

  it('renders nothing when mechanics are off', () => {
    const { container } = render(<MechanicsStrip turn={turn({ engagement: 'off' })} />);
    expect(container).toBeEmptyDOMElement();
  });
});
