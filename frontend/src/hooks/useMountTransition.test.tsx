import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { useMountTransition } from './useMountTransition';

afterEach(() => {
  vi.useRealTimers();
});

describe('useMountTransition', () => {
  it('enters synchronously when opened', () => {
    const { result, rerender } = renderHook(({ open }: { open: boolean }) => useMountTransition(open, 200), {
      initialProps: { open: false },
    });
    expect(result.current.mounted).toBe(false);
    expect(result.current.state).toBe('exit');

    rerender({ open: true });
    expect(result.current.mounted).toBe(true);
    expect(result.current.state).toBe('enter');
  });

  it('stays mounted through the exit, then unmounts after the duration', () => {
    vi.useFakeTimers();
    const { result, rerender } = renderHook(({ open }: { open: boolean }) => useMountTransition(open, 200), {
      initialProps: { open: true },
    });
    expect(result.current.mounted).toBe(true);

    rerender({ open: false });
    expect(result.current.state).toBe('exit');
    expect(result.current.mounted).toBe(true);

    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(result.current.mounted).toBe(false);
  });
});
