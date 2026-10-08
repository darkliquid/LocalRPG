import { useCallback, useEffect, useRef, useState } from 'react';
import { APIClient, HTTPError } from '../api/client';

// pollMs is how often the pending choice is checked. The dialog is answered by
// the user, so a few hundred milliseconds of latency is invisible.
const pollMs = 400;

// ceilingMs stops the UI waiting on a dialog that never answers. A user may
// browse for a while, so it is generous.
const ceilingMs = 3 * 60 * 1000;

export interface FolderPicker {
  // picking is true while a dialog is open and the UI is waiting for it.
  picking: boolean;
  // note explains why the picker could not be used, so the path field is the way
  // in. Null when there is nothing to say.
  note: string | null;
  // pick opens the native folder dialog and resolves with the chosen path, or
  // null when the user cancelled or no dialog is available.
  pick: (title: string) => Promise<string | null>;
}

// useFolderPicker drives the desktop window's native folder dialog. The dialog
// is modal, so the server opens it and returns at once; this hook polls for the
// result. Waiting on the request itself is what left the webview unable to
// repaint.
export function useFolderPicker(): FolderPicker {
  const [picking, setPicking] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const unmounted = useRef(false);

  useEffect(() => {
    return () => {
      unmounted.current = true;
    };
  }, []);

  const pick = useCallback(async (title: string): Promise<string | null> => {
    setNote(null);
    try {
      await APIClient.startDirectoryChoice(title);
    } catch (err) {
      if (err instanceof HTTPError && err.status === 501) {
        setNote('No native folder dialog is available here, so type the path instead.');
      } else if (err instanceof HTTPError && err.status === 409) {
        setNote('A folder dialog is already open.');
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
          choice = await APIClient.directoryChoice();
        } catch (err) {
          setNote(err instanceof Error ? err.message : String(err));
          return null;
        }
        if (choice.status === 'selected') return choice.path ?? null;
        if (choice.status !== 'pending') return null;
        if (Date.now() > deadline) {
          setNote('The folder dialog did not answer.');
          return null;
        }
      }
    } finally {
      if (!unmounted.current) setPicking(false);
    }
  }, []);

  return { picking, note, pick };
}

export default useFolderPicker;
