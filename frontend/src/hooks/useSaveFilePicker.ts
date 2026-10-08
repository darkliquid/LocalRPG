import { useCallback, useEffect, useRef, useState } from 'react';
import { APIClient, HTTPError } from '../api/client';
import { ChooseSaveFileRequest } from '../types';

// pollMs is how often the pending choice is checked.
const pollMs = 400;

// ceilingMs stops the UI waiting on a dialog that never answers.
const ceilingMs = 3 * 60 * 1000;

export interface SaveFilePicker {
  // picking is true while a dialog is open and the UI is waiting for it.
  picking: boolean;
  // nativeDialog is true when running under Wails desktop with native dialog support.
  nativeDialog: boolean;
  // note explains why the picker could not be used. Null when there is nothing to say.
  note: string | null;
  // pick opens the native save file dialog and resolves with the chosen path, or
  // null when the user cancelled or no dialog is available.
  pick: (req?: ChooseSaveFileRequest) => Promise<string | null>;
}

// useSaveFilePicker drives the desktop window's native save file dialog.
export function useSaveFilePicker(): SaveFilePicker {
  const [picking, setPicking] = useState(false);
  const [nativeDialog, setNativeDialog] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const unmounted = useRef(false);

  useEffect(() => {
    APIClient.exportCapabilities()
      .then((caps) => {
        if (!unmounted.current) {
          setNativeDialog(Boolean(caps.native_dialog));
        }
      })
      .catch(() => {});

    return () => {
      unmounted.current = true;
    };
  }, []);

  const pick = useCallback(async (req: ChooseSaveFileRequest = {}): Promise<string | null> => {
    setNote(null);
    try {
      await APIClient.startSaveFileChoice(req);
    } catch (err) {
      if (err instanceof HTTPError && err.status === 501) {
        setNote('No native save file dialog is available here.');
      } else if (err instanceof HTTPError && err.status === 409) {
        setNote('A save file dialog is already open.');
      } else {
        setNote(err instanceof Error ? err.message : String(err));
      }
      return null;
    }

    setPicking(true);
    const deadline = Date.now() + ceilingMs;
    try {
      for (;;) {
        await new Promise((resolve) => setTimeout(resolve, pollMs));
        if (unmounted.current) return null;

        let choice;
        try {
          choice = await APIClient.saveFileChoice();
        } catch (err) {
          setNote(err instanceof Error ? err.message : String(err));
          return null;
        }
        if (choice.status === 'selected') return choice.path ?? null;
        if (choice.status !== 'pending') return null;
        if (Date.now() > deadline) {
          setNote('The save file dialog did not answer.');
          return null;
        }
      }
    } finally {
      if (!unmounted.current) setPicking(false);
    }
  }, []);

  return { picking, nativeDialog, note, pick };
}

export default useSaveFilePicker;
