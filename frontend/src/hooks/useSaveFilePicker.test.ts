import { renderHook, act, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { useSaveFilePicker } from './useSaveFilePicker';
import { APIClient, HTTPError } from '../api/client';

afterEach(() => {
  vi.restoreAllMocks();
});

describe('useSaveFilePicker', () => {
  it('resolves with the path the save dialog returned', async () => {
    vi.spyOn(APIClient, 'exportCapabilities').mockResolvedValue({ native_dialog: true });
    vi.spyOn(APIClient, 'startSaveFileChoice').mockResolvedValue(undefined);
    const poll = vi
      .spyOn(APIClient, 'saveFileChoice')
      .mockResolvedValueOnce({ status: 'pending' })
      .mockResolvedValueOnce({ status: 'selected', path: '/home/you/export.lrpgworld' });

    const { result } = renderHook(() => useSaveFilePicker());

    await waitFor(() => expect(result.current.nativeDialog).toBe(true));

    let chosen: string | null = null;
    await act(async () => {
      chosen = await result.current.pick({ title: 'Save World' });
    });

    expect(chosen).toBe('/home/you/export.lrpgworld');
    expect(poll).toHaveBeenCalledTimes(2);
    await waitFor(() => expect(result.current.picking).toBe(false));
    expect(result.current.note).toBeNull();
  });

  it('treats a cancellation as no choice', async () => {
    vi.spyOn(APIClient, 'exportCapabilities').mockResolvedValue({ native_dialog: true });
    vi.spyOn(APIClient, 'startSaveFileChoice').mockResolvedValue(undefined);
    vi.spyOn(APIClient, 'saveFileChoice').mockResolvedValue({ status: 'cancelled' });

    const { result } = renderHook(() => useSaveFilePicker());
    let chosen: string | null = 'not null';
    await act(async () => {
      chosen = await result.current.pick({ title: 'Save World' });
    });

    expect(chosen).toBeNull();
    expect(result.current.note).toBeNull();
  });

  it('explains when no native dialog is available', async () => {
    vi.spyOn(APIClient, 'exportCapabilities').mockResolvedValue({ native_dialog: false });
    vi.spyOn(APIClient, 'startSaveFileChoice').mockRejectedValue(new HTTPError(501, 'no dialog'));
    const poll = vi.spyOn(APIClient, 'saveFileChoice');

    const { result } = renderHook(() => useSaveFilePicker());
    await act(async () => {
      expect(await result.current.pick({ title: 'Save World' })).toBeNull();
    });

    expect(result.current.note).toMatch(/no native save file dialog/i);
    expect(poll).not.toHaveBeenCalled();
  });

  it('explains a dialog that is already open', async () => {
    vi.spyOn(APIClient, 'exportCapabilities').mockResolvedValue({ native_dialog: true });
    vi.spyOn(APIClient, 'startSaveFileChoice').mockRejectedValue(new HTTPError(409, 'in flight'));

    const { result } = renderHook(() => useSaveFilePicker());
    await act(async () => {
      expect(await result.current.pick({ title: 'Save World' })).toBeNull();
    });

    expect(result.current.note).toMatch(/already open/i);
  });
});
