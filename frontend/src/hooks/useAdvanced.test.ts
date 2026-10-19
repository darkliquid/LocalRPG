import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, beforeEach } from 'vitest';
import { STORAGE_KEYS } from '../lib/constants';
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
    localStorage.setItem(STORAGE_KEYS.SETTINGS_ADVANCED, 'true');

    const { result } = renderHook(() => useAdvanced());

    expect(result.current[0]).toBe(true);
  });

  it('persists a change, both ways', () => {
    const { result } = renderHook(() => useAdvanced());

    act(() => result.current[1](true));
    expect(result.current[0]).toBe(true);
    expect(localStorage.getItem(STORAGE_KEYS.SETTINGS_ADVANCED)).toBe('true');

    act(() => result.current[1](false));
    expect(result.current[0]).toBe(false);
    expect(localStorage.getItem(STORAGE_KEYS.SETTINGS_ADVANCED)).toBe('false');
  });

  it('ignores a stored value that is not a stored preference', () => {
    localStorage.setItem(STORAGE_KEYS.SETTINGS_ADVANCED, 'nonsense');

    const { result } = renderHook(() => useAdvanced());

    expect(result.current[0]).toBe(false);
  });
});
