import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { LimitChip } from './LimitChip';

afterEach(() => {
  vi.useRealTimers();
});

describe('LimitChip', () => {
  it('renders a countdown for a future limit', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-06T12:00:00Z'));
    render(<LimitChip block={{ provider: 'openai', role: 'gm', until: '2026-10-06T12:00:30Z' }} />);
    expect(screen.getByText(/openai \(gm\): 30s/)).toBeInTheDocument();
  });

  it('renders nothing once the limit has passed', () => {
    render(<LimitChip block={{ provider: 'openai', role: 'gm', until: '2020-01-01T00:00:00Z' }} />);
    expect(screen.queryByText(/openai/)).toBeNull();
  });

  it('renders nothing without an expiry', () => {
    render(<LimitChip block={{ provider: 'openai', role: 'gm' }} />);
    expect(screen.queryByText(/openai/)).toBeNull();
  });
});
