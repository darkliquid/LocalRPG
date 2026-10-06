import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { AppConfig, MediaInspectEntry } from '../types';
import { APIClient } from '../api/client';
import { TierBadge } from './providers/TierBadge';

export type ProviderFamily = 'tts' | 'stt' | 'image';

// purposesFor is the family's uses, in display order. STT has none yet.
export function purposesFor(family: ProviderFamily): string[] {
  if (family === 'tts') return ['narrator', 'npc'];
  if (family === 'image') return ['scene', 'portrait', 'placeholder'];
  return [];
}

// providersKey is the config field holding a family's named providers.
export function providersKey(family: ProviderFamily): 'tts_providers' | 'stt_providers' | 'image_providers' {
  return `${family}_providers` as const;
}

// uniqueName appends a numeric suffix until the name is free.
export function uniqueName(base: string, taken: Record<string, unknown>): string {
  if (!(base in taken)) return base;
  let n = 2;
  while (`${base}-${n}` in taken) n++;
  return `${base}-${n}`;
}

// renameInPurposes rewrites every purpose that pointed at oldName.
export function renameInPurposes(purposes: Record<string, string> | undefined, oldName: string, newName: string): Record<string, string> {
  const next: Record<string, string> = { ...(purposes ?? {}) };
  for (const use of Object.keys(next)) {
    if (next[use] === oldName) next[use] = newName;
  }
  return next;
}

// purposeRefs reports the purposes that reference a provider name.
export function purposeRefs(purposes: Record<string, string> | undefined, name: string): string[] {
  if (!purposes) return [];
  return Object.keys(purposes).filter((use) => purposes[use] === name);
}

interface ProviderManagerProps {
  family: ProviderFamily;
  config: AppConfig;
  onChange: (config: AppConfig) => void;
}

// ProviderManager lists every provider of a family and lets the user add,
// duplicate, rename, remove, and choose defaults and purposes. It edits a draft
// config through onChange; it never saves.
export const ProviderManager: React.FC<ProviderManagerProps> = ({ family, config, onChange }) => {
  const [entries, setEntries] = useState<MediaInspectEntry[]>([]);
  const [error, setError] = useState<string | null>(null);

  const providers = (config.media[providersKey(family)] ?? {}) as Record<string, unknown>;
  const purposes = config.media.purposes;

  useEffect(() => {
    let active = true;
    APIClient.inspectMedia(family)
      .then((res) => {
        if (active) setEntries(res.entries ?? []);
      })
      .catch((err: unknown) => {
        if (active) setError(err instanceof Error ? err.message : 'Could not inspect providers');
      });
    return () => {
      active = false;
    };
  }, [family, providers]);

  const byName = useMemo(() => {
    const map: Record<string, MediaInspectEntry> = {};
    for (const entry of entries) map[entry.name] = entry;
    return map;
  }, [entries]);

  const names = useMemo(() => ['default', ...Object.keys(providers).sort()], [providers]);

  const emit = useCallback(
    (next: AppConfig) => onChange(next),
    [onChange]
  );

  const setProviders = useCallback(
    (next: Record<string, unknown>) => {
      emit({ ...config, media: { ...config.media, [providersKey(family)]: next } });
    },
    [config, emit, family]
  );

  const addProvider = useCallback(() => {
    const name = uniqueName('new-provider', providers);
    setProviders({ ...providers, [name]: { ...config.media[family] } });
  }, [config.media, family, providers, setProviders]);

  const duplicateProvider = useCallback(
    (name: string) => {
      const source = name === 'default' ? config.media[family] : providers[name];
      const copyName = uniqueName(`${name}-copy`, providers);
      setProviders({ ...providers, [copyName]: { ...(source as object) } });
    },
    [config.media, family, providers, setProviders]
  );

  const renameProvider = useCallback(
    (oldName: string, newName: string) => {
      const trimmed = newName.trim();
      if (!trimmed || trimmed === oldName || trimmed === 'default') return;
      const next = { ...providers };
      next[trimmed] = next[oldName];
      delete next[oldName];
      emit({
        ...config,
        media: {
          ...config.media,
          [providersKey(family)]: next,
          purposes: renameInPurposes(purposes, oldName, trimmed),
        },
      });
    },
    [config, emit, family, providers, purposes]
  );

  const removeProvider = useCallback(
    (name: string) => {
      if (name === 'default') return;
      if (purposeRefs(purposes, name).length > 0) {
        setError(`Reassign the ${purposeRefs(purposes, name).join(', ')} purpose before removing ${name}.`);
        return;
      }
      const next = { ...providers };
      delete next[name];
      setError(null);
      setProviders(next);
    },
    [providers, purposes, setProviders]
  );

  const setDefault = useCallback(
    (name: string) => {
      if (name === 'default') return;
      const source = providers[name] as object;
      emit({ ...config, media: { ...config.media, [family]: { ...source } } });
    },
    [config, emit, family, providers]
  );

  const setPurpose = useCallback(
    (use: string, name: string) => {
      const next = { ...(purposes ?? {}) };
      if (name === 'default') {
        delete next[use];
      } else {
        next[use] = name;
      }
      emit({ ...config, media: { ...config.media, purposes: next } });
    },
    [config, emit, purposes]
  );

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="font-sans text-xs uppercase font-bold text-stone-200">{family.toUpperCase()} Providers</h4>
        <button
          type="button"
          onClick={addProvider}
          className="rounded-lg border border-purple-500/40 bg-purple-500/10 px-2.5 py-1 text-xs font-medium text-purple-200 hover:bg-purple-500/20"
        >
          Add {family.toUpperCase()} Provider
        </button>
      </div>

      {error && <p className="text-xs text-amber-300">{error}</p>}

      <ul className="space-y-1">
        {names.map((name) => {
          const entry = byName[name];
          const isDefault = name === 'default';
          return (
            <li key={name} className="flex flex-wrap items-center gap-2 rounded-lg border border-stone-800 bg-stone-950/40 px-3 py-2">
              <span className={`h-2 w-2 rounded-full ${isDefault ? 'bg-emerald-400' : 'bg-stone-600'}`} title={isDefault ? 'Default' : ''} />
              <span className="font-mono text-xs text-stone-100">{name}</span>
              {entry && <TierBadge tier={entry.tier} caveat={entry.provider_key} />}
              {entry && (
                <span className={`text-[11px] ${entry.key_required && !entry.key_present ? 'text-amber-300' : 'text-stone-500'}`}>
                  {entry.key_required ? (entry.key_present ? 'key set' : 'key missing') : 'no key needed'}
                </span>
              )}
              <span className="flex-1" />
              {!isDefault && (
                <button type="button" onClick={() => setDefault(name)} className="text-[11px] text-stone-400 hover:text-emerald-300">
                  set default
                </button>
              )}
              <button type="button" onClick={() => duplicateProvider(name)} className="text-[11px] text-stone-400 hover:text-purple-300">
                duplicate
              </button>
              {!isDefault && (
                <>
                  <button
                    type="button"
                    onClick={() => {
                      const next = window.prompt('Rename provider', name);
                      if (next) renameProvider(name, next);
                    }}
                    className="text-[11px] text-stone-400 hover:text-purple-300"
                  >
                    rename
                  </button>
                  <button type="button" onClick={() => removeProvider(name)} className="text-[11px] text-stone-400 hover:text-rose-400">
                    remove
                  </button>
                </>
              )}
            </li>
          );
        })}
      </ul>

      {purposesFor(family).length > 0 && (
        <div className="rounded-lg border border-stone-800 bg-stone-950/40 p-3">
          <p className="mb-2 font-sans text-[11px] uppercase text-stone-400">Purposes</p>
          <div className="space-y-1.5">
            {purposesFor(family).map((use) => (
              <label key={use} className="flex items-center gap-2 text-xs text-stone-300">
                <span className="w-24 font-mono">{use}</span>
                <select
                  aria-label={use}
                  value={purposes?.[use] ?? 'default'}
                  onChange={(e) => setPurpose(use, e.target.value)}
                  className="rounded-lg border border-stone-800 bg-stone-950 px-2 py-1 text-xs text-stone-100"
                >
                  {names.map((name) => (
                    <option key={name} value={name}>
                      {name}
                    </option>
                  ))}
                </select>
              </label>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};
