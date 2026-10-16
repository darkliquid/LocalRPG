import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { APIClient } from '../api/client';
import { SettingsResponse } from '../types';
import { useFailures } from './FailuresContext';

interface SettingsValue {
  /** settings is the whole response: the config, the file it came from, and the override. */
  settings: SettingsResponse | null;
  isLoading: boolean;
  reload: () => void;
  /** applySettings takes a response the server has already accepted, without asking again. */
  applySettings: (response: SettingsResponse) => void;
}

const SettingsContext = createContext<SettingsValue | null>(null);

// SettingsProvider is the one owner of the configuration. It fetches once for the
// whole app, so the settings studio edits a copy of what every other surface is
// already reading, and a save is applied back rather than fetched again.
export function SettingsProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<SettingsResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const { report } = useFailures();

  const load = useCallback(() => {
    setIsLoading(true);
    APIClient.getSettings()
      .then(setSettings)
      .catch((err) =>
        report({ text: 'Could not load the settings.', detail: err instanceof Error ? err.message : String(err) }),
      )
      .finally(() => setIsLoading(false));
  }, [report]);

  useEffect(load, [load]);

  const applySettings = useCallback((response: SettingsResponse) => setSettings(response), []);

  const value = useMemo(
    () => ({ settings, isLoading, reload: load, applySettings }),
    [settings, isLoading, load, applySettings],
  );

  return <SettingsContext.Provider value={value}>{children}</SettingsContext.Provider>;
}

// useSettings returns the settings response. Outside a provider it is null and
// loading forever rather than throwing, so a component rendered on its own in a
// test does not have to be wrapped in a provider it does not need.
export function useSettings(): SettingsValue {
  return useContext(SettingsContext) ?? noProviderSettings;
}

const noProviderSettings: SettingsValue = {
  settings: null,
  isLoading: true,
  reload: () => undefined,
  applySettings: () => undefined,
};
