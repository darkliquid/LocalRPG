import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { PendingCheckCard } from './PendingCheckCard';
import { PendingCheck } from '../types';

const pending = (overrides: Partial<PendingCheck> = {}): PendingCheck => ({
  ref: 'r',
  notation: '2d6',
  request: {
    actor: 'kaelen',
    check_kind: 'do',
    stakes: 'the alarm sounds',
    outcomes: { weak: 'at a cost', miss: 'trouble' },
  },
  bonuses: [{ source: 'Edge', value: 3 }],
  ...overrides,
});

describe('PendingCheckCard', () => {
  it('renders the stakes, bonuses, and outcomes', () => {
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={() => {}} onArgue={() => {}} />);
    expect(screen.getByText(/the alarm sounds/)).toBeInTheDocument();
    expect(screen.getByText(/Edge/)).toBeInTheDocument();
    expect(screen.getByText(/at a cost/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /roll/i })).toBeInTheDocument();
  });

  it('calls onRoll', () => {
    const onRoll = vi.fn();
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={onRoll} onManual={() => {}} onArgue={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /roll/i }));
    expect(onRoll).toHaveBeenCalled();
  });

  it('submits a manual total', () => {
    const onManual = vi.fn();
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={onManual} onArgue={() => {}} />);
    fireEvent.change(screen.getByLabelText(/manual roll/i), { target: { value: '9' } });
    fireEvent.click(screen.getByRole('button', { name: /enter/i }));
    expect(onManual).toHaveBeenCalledWith(9);
  });

  it('renders nothing without a pending check', () => {
    const { container } = render(
      <PendingCheckCard pending={undefined} busy={false} onRoll={() => {}} onManual={() => {}} onArgue={() => {}} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
