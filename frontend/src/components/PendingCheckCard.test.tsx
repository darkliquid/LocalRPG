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
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={() => {}} onArgue={async () => undefined} />);
    expect(screen.getByText(/the alarm sounds/)).toBeInTheDocument();
    expect(screen.getByText(/Edge/)).toBeInTheDocument();
    expect(screen.getByText(/at a cost/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /roll/i })).toBeInTheDocument();
  });

  it('calls onRoll', () => {
    const onRoll = vi.fn();
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={onRoll} onManual={() => {}} onArgue={async () => undefined} />);
    fireEvent.click(screen.getByRole('button', { name: /roll/i }));
    expect(onRoll).toHaveBeenCalled();
  });

  it('submits a manual total', () => {
    const onManual = vi.fn();
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={onManual} onArgue={async () => undefined} />);
    fireEvent.change(screen.getByLabelText(/manual roll/i), { target: { value: '9' } });
    fireEvent.click(screen.getByRole('button', { name: /enter/i }));
    expect(onManual).toHaveBeenCalledWith({ dice: [9], total: 9 });
  });

  it('submits entered dice', () => {
    const onManual = vi.fn();
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={onManual} onArgue={async () => undefined} />);
    fireEvent.change(screen.getByLabelText(/manual roll/i), { target: { value: '4 3' } });
    fireEvent.click(screen.getByRole('button', { name: /enter/i }));
    expect(onManual).toHaveBeenCalledWith({ dice: [4, 3], total: 7 });
  });

  it('shows the notation range and previews the total', () => {
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={() => {}} onArgue={async () => undefined} />);
    expect(screen.getByText(/2d6 can roll 2-12/)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/manual roll/i), { target: { value: '4 3' } });
    expect(screen.getByText(/4 \+ 3 = 7/)).toBeInTheDocument();
    expect(screen.getByText(/\+ 3 = 10/)).toBeInTheDocument();
  });

  it('warns about an implausible entry without blocking it', () => {
    const onManual = vi.fn();
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={onManual} onArgue={async () => undefined} />);
    fireEvent.change(screen.getByLabelText(/manual roll/i), { target: { value: '20' } });
    expect(screen.getByText(/outside the usual range/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /enter/i }));
    expect(onManual).toHaveBeenCalledWith({ dice: [20], total: 20 });
  });

  it('blocks a non-number', () => {
    const onManual = vi.fn();
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={onManual} onArgue={async () => undefined} />);
    fireEvent.change(screen.getByLabelText(/manual roll/i), { target: { value: 'four' } });
    expect(screen.getByRole('button', { name: /enter/i })).toBeDisabled();
    expect(onManual).not.toHaveBeenCalled();
  });

  it('opens the argue form', () => {
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={() => {}} onArgue={async () => undefined} />);
    expect(screen.queryByLabelText(/approach/i)).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /argue/i }));
    expect(screen.getByLabelText(/approach/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/proposed stakes/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/proposed difficulty/i)).toBeInTheDocument();
  });

  it('submits a counter-proposal and shows an accept ruling', async () => {
    const onArgue = vi.fn().mockResolvedValue({ ruling: 'accept', stakes: 'the bar lifts', difficulty: 'risky' });
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={() => {}} onArgue={onArgue} />);
    fireEvent.click(screen.getByRole('button', { name: /argue/i }));
    fireEvent.change(screen.getByLabelText(/approach/i), { target: { value: 'lift the bar' } });
    fireEvent.change(screen.getByLabelText(/proposed difficulty/i), { target: { value: 'risky' } });
    fireEvent.click(screen.getByRole('button', { name: /make the case/i }));
    expect(onArgue).toHaveBeenCalledWith({ approach: 'lift the bar', stakes: undefined, difficulty: 'risky' });
    expect(await screen.findByText(/accept/)).toBeInTheDocument();
    expect(screen.getByText(/the bar lifts/)).toBeInTheDocument();
  });

  it('shows a hold reason without agreed terms', async () => {
    const onArgue = vi.fn().mockResolvedValue({ ruling: 'hold', reason: 'the lock is the lock' });
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={() => {}} onArgue={onArgue} />);
    fireEvent.click(screen.getByRole('button', { name: /argue/i }));
    fireEvent.change(screen.getByLabelText(/approach/i), { target: { value: 'just open it' } });
    fireEvent.click(screen.getByRole('button', { name: /make the case/i }));
    expect(await screen.findByText(/the lock is the lock/)).toBeInTheDocument();
    expect(screen.queryByText(/Agreed:/)).toBeNull();
  });

  it('disables the case button until something is said', () => {
    render(<PendingCheckCard pending={pending()} busy={false} onRoll={() => {}} onManual={() => {}} onArgue={async () => undefined} />);
    fireEvent.click(screen.getByRole('button', { name: /argue/i }));
    expect(screen.getByRole('button', { name: /make the case/i })).toBeDisabled();
  });

  it('renders nothing without a pending check', () => {
    const { container } = render(
      <PendingCheckCard pending={undefined} busy={false} onRoll={() => {}} onManual={() => {}} onArgue={async () => undefined} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
