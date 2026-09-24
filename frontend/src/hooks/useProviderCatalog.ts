import { useCallback, useEffect, useMemo, useState } from 'react';
import { APIClient } from '../api/client';
import { ProviderDescriptor, ProviderFamily, ProviderPreset } from '../types';

export interface ProviderCatalogState {
  providers: ProviderDescriptor[];
  byFamily: (family: ProviderFamily) => ProviderDescriptor[];
  presets: (family: ProviderFamily) => ProviderPreset[];
  loading: boolean;
  error: string | null;
}

// useProviderCatalog fetches the provider catalogue once, so settings can render
// engine choices from capabilities rather than provider names. An empty
// catalogue simply means the caller falls back to its own defaults.
export function useProviderCatalog(enabled = true): ProviderCatalogState {
  const [providers, setProviders] = useState<ProviderDescriptor[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    setLoading(true);
    APIClient.listProviders()
      .then((catalog) => {
        if (!active) return;
        setProviders(catalog.providers ?? []);
        setError(null);
      })
      .catch((err: unknown) => {
        if (!active) return;
        setError(err instanceof Error ? err.message : 'Failed to load the provider catalogue');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [enabled]);

  const byFamily = useCallback(
    (family: ProviderFamily) => providers.filter((p) => p.family === family),
    [providers]
  );

  const presets = useCallback(
    (family: ProviderFamily) =>
      byFamily(family)
        .flatMap((p) => p.presets ?? [])
        .sort((a, b) => a.order - b.order),
    [byFamily]
  );

  return useMemo(
    () => ({ providers, byFamily, presets, loading, error }),
    [providers, byFamily, presets, loading, error]
  );
}
