import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, beforeEach } from 'vitest';
import { useAdvanced } from './useAdvanced';

describe('useAdvanced', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('defaults to off', () => {
    const { result } = renderHook(() => useAdvanced());

    expect(result.current[0]).toBe(false);
  });

  it('reads a stored preference', () => {
    localStorage.setItem('localrpg.settings.advanced', 'true');

    const { result } = renderHook(() => useAdvanced());

    expect(result.current[0]).toBe(true);
  });

  it('persists a change, both ways', () => {
    const { result } = renderHook(() => useAdvanced());

    act(() => result.current[1](true));
    expect(result.current[0]).toBe(true);
    expect(localStorage.getItem('localrpg.settings.advanced')).toBe('true');

    act(() => result.current[1](false));
    expect(result.current[0]).toBe(false);
    expect(localStorage.getItem('localrpg.settings.advanced')).toBe('false');
  });

  it('ignores a stored value that is not a stored preference', () => {
    localStorage.setItem('localrpg.settings.advanced', 'nonsense');

    const { result } = renderHook(() => useAdvanced());

    expect(result.current[0]).toBe(false);
  });
});
