import { useCallback, useState } from 'react';
import { STORAGE_KEYS } from '../lib/constants';

// A UI preference rather than machine configuration, so it lives in
// localStorage instead of the config file: it never needs to sync, and the
// config schema stays untouched.
const STORAGE_KEY = STORAGE_KEYS.SETTINGS_ADVANCED;

function readStored(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'true';
  } catch {
    // Private browsing can refuse storage; the setting simply stays off.
    return false;
  }
}

// useAdvanced is the studio's "show advanced settings" preference. It is read
// once on mount and written back on every change, so the choice survives a
// reload within the same origin.
export function useAdvanced(): [boolean, (value: boolean) => void] {
  const [advanced, setAdvancedState] = useState<boolean>(readStored);

  const setAdvanced = useCallback((value: boolean) => {
    setAdvancedState(value);
    try {
      localStorage.setItem(STORAGE_KEY, value ? 'true' : 'false');
    } catch {
      // A refused write only costs the preference its persistence.
    }
  }, []);

  return [advanced, setAdvanced];
}
