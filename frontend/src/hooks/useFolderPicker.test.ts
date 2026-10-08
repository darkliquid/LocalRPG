import { renderHook, act, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { useFolderPicker } from './useFolderPicker';
import { APIClient, HTTPError } from '../api/client';

afterEach(() => {
  vi.restoreAllMocks();
});

describe('useFolderPicker', () => {
  it('resolves with the path the dialog returned', async () => {
    vi.spyOn(APIClient, 'startDirectoryChoice').mockResolvedValue(undefined);
    const poll = vi
      .spyOn(APIClient, 'directoryChoice')
      .mockResolvedValueOnce({ status: 'pending' })
      .mockResolvedValueOnce({ status: 'selected', path: '/home/you/notes' });

    const { result } = renderHook(() => useFolderPicker());

    let chosen: string | null = null;
    await act(async () => {
      chosen = await result.current.pick('Choose a source folder');
    });

    expect(chosen).toBe('/home/you/notes');
    expect(poll).toHaveBeenCalledTimes(2);
    await waitFor(() => expect(result.current.picking).toBe(false));
    expect(result.current.note).toBeNull();
  });

  it('treats a cancellation as no choice', async () => {
    vi.spyOn(APIClient, 'startDirectoryChoice').mockResolvedValue(undefined);
    vi.spyOn(APIClient, 'directoryChoice').mockResolvedValue({ status: 'cancelled' });

    const { result } = renderHook(() => useFolderPicker());
    let chosen: string | null = 'not null';
    await act(async () => {
      chosen = await result.current.pick('Choose a source folder');
    });

    expect(chosen).toBeNull();
    expect(result.current.note).toBeNull();
  });

  it('explains a build with no native dialog', async () => {
    vi.spyOn(APIClient, 'startDirectoryChoice').mockRejectedValue(new HTTPError(501, 'no dialog'));
    const poll = vi.spyOn(APIClient, 'directoryChoice');

    const { result } = renderHook(() => useFolderPicker());
    await act(async () => {
      expect(await result.current.pick('Choose a source folder')).toBeNull();
    });

    expect(result.current.note).toMatch(/no native folder dialog/i);
    expect(poll).not.toHaveBeenCalled();
  });

  it('explains a dialog that is already open', async () => {
    vi.spyOn(APIClient, 'startDirectoryChoice').mockRejectedValue(new HTTPError(409, 'in flight'));

    const { result } = renderHook(() => useFolderPicker());
    await act(async () => {
      expect(await result.current.pick('Choose a source folder')).toBeNull();
    });

    expect(result.current.note).toMatch(/already open/i);
  });
});
