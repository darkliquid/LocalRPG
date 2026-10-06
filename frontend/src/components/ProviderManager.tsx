import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { AgentRoleConfig, AppConfig, MediaInspectEntry } from '../types';
import { APIClient } from '../api/client';
import { TierBadge } from './providers/TierBadge';
import {
  ProviderFamily,
  entryNames,
  mediaEntryValue,
  providersKey,
  purposesFor,
  setMediaEntry,
  uniqueName,
} from '../lib/mediaProviders';

export type ManagerFamily = ProviderFamily | 'llm';

// RoleListItem describes one LLM role for the manager's list: its display label,
// the catalogue descriptor of the adapter it resolves to, and its key state.
export interface RoleListItem {
  name: string;
  label: string;
  providerLabel?: string;
  tier?: string;
  caveat?: string;
  keyRequired?: boolean;
  keyPresent?: boolean;
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
  family: ManagerFamily;
  config: AppConfig;
  onChange: (config: AppConfig) => void;
  selected?: string;
  onSelect?: (name: string) => void;
  renderEditor?: (name: string, value: unknown, onChange: (value: unknown) => void) => React.ReactNode;
  roles?: RoleListItem[];
}

interface Row {
  name: string;
  label: string;
  providerLabel?: string;
  tier?: string;
  caveat?: string;
  keyRequired?: boolean;
  keyPresent?: boolean;
}

// ProviderManager lists every provider of a family and lets the user add,
// duplicate, rename, remove, and choose defaults and purposes. In `llm` mode it
// lists the configured roles instead, with each role's resolved adapter. It
// edits a draft config through onChange; it never saves. The selection is
// controlled when `selected`/`onSelect` are supplied and local otherwise, so the
// manager stays usable on its own.
export const ProviderManager: React.FC<ProviderManagerProps> = ({ family, config, onChange, selected, onSelect, renderEditor, roles }) => {
  const isLLM = family === 'llm';
  // The media family the address helpers use; only meaningful when not in llm mode.
  const mediaFamily = (isLLM ? 'tts' : family) as ProviderFamily;

  const [entries, setEntries] = useState<MediaInspectEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [internalSelected, setInternalSelected] = useState<string>('default');
  const [refreshToken, setRefreshToken] = useState(0);
  const [defaultSource, setDefaultSource] = useState<string>('default');
  const [defaultNotice, setDefaultNotice] = useState<string | null>(null);

  const activeSelected = selected ?? internalSelected;
  const select = useCallback(
    (name: string) => {
      setDefaultNotice(null);
      if (onSelect) onSelect(name);
      else setInternalSelected(name);
    },
    [onSelect]
  );

  const providers = (isLLM ? {} : (config.media[providersKey(mediaFamily)] ?? {})) as Record<string, unknown>;
  const purposes = config.media.purposes;

  useEffect(() => {
    if (isLLM) return;
    let active = true;
    APIClient.inspectMedia(mediaFamily)
      .then((res) => {
        if (active) setEntries(res.entries ?? []);
      })
      .catch((err: unknown) => {
        if (active) setError(err instanceof Error ? err.message : 'Could not inspect providers');
      });
    return () => {
      active = false;
    };
  }, [mediaFamily, providers, refreshToken, isLLM]);

  const byName = useMemo(() => {
    const map: Record<string, MediaInspectEntry> = {};
    for (const entry of entries) map[entry.name] = entry;
    return map;
  }, [entries]);

  const rolesByName = useMemo(() => {
    const map: Record<string, RoleListItem> = {};
    for (const item of roles ?? []) map[item.name] = item;
    return map;
  }, [roles]);

  const names = useMemo(
    () => (isLLM ? Object.keys(config.agents.roles) : entryNames(config, mediaFamily)),
    [isLLM, config, mediaFamily]
  );

  const rows: Row[] = names.map((name) => {
    if (isLLM) {
      const item = rolesByName[name];
      return {
        name,
        label: item?.label ?? name,
        providerLabel: item?.providerLabel,
        tier: item?.tier,
        caveat: item?.caveat,
        keyRequired: item?.keyRequired,
        keyPresent: item?.keyPresent,
      };
    }
    const entry = byName[name];
    return {
      name,
      label: name,
      tier: entry?.tier,
      caveat: entry?.provider_key,
      keyRequired: entry?.key_required,
      keyPresent: entry?.key_present,
    };
  });

  const emit = useCallback(
    (next: AppConfig) => {
      setDefaultNotice(null);
      onChange(next);
    },
    [onChange]
  );

  const setProviders = useCallback(
    (next: Record<string, unknown>) => {
      emit({ ...config, media: { ...config.media, [providersKey(mediaFamily)]: next } });
    },
    [config, emit, mediaFamily]
  );

  const addProvider = useCallback(() => {
    const name = uniqueName('new-provider', providers);
    setProviders({ ...providers, [name]: { ...config.media[mediaFamily] } });
    select(name);
  }, [config.media, mediaFamily, providers, select, setProviders]);

  const duplicateProvider = useCallback(
    (name: string) => {
      const source = name === 'default' ? config.media[mediaFamily] : providers[name];
      const copyName = uniqueName(`${name}-copy`, providers);
      setProviders({ ...providers, [copyName]: { ...(source as object) } });
      select(copyName);
    },
    [config.media, mediaFamily, providers, select, setProviders]
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
          [providersKey(mediaFamily)]: next,
          purposes: renameInPurposes(purposes, oldName, trimmed),
        },
      });
    },
    [config, emit, mediaFamily, providers, purposes]
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
      emit(setMediaEntry(config, mediaFamily, 'default', { ...source }));
      setDefaultSource(name);
      setRefreshToken((value) => value + 1);
      setDefaultNotice(`Default is now ${name}.`);
    },
    [config, emit, mediaFamily, providers]
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

  const selectedValue = isLLM
    ? config.agents.roles[activeSelected] ?? {}
    : mediaEntryValue(config, mediaFamily, activeSelected);
  const writeSelected = (value: unknown) => {
    if (isLLM) {
      emit({ ...config, agents: { ...config.agents, roles: { ...config.agents.roles, [activeSelected]: value as AgentRoleConfig } } });
    } else {
      emit(setMediaEntry(config, mediaFamily, activeSelected, value));
    }
  };

  const defaultMarker = isLLM ? config.agents.default_role : defaultSource;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="font-sans text-xs uppercase font-bold text-stone-200">
          {isLLM ? 'LLM Roles' : `${family.toUpperCase()} Providers`}
        </h4>
        {!isLLM && (
          <button
            type="button"
            onClick={addProvider}
            className="rounded-lg border border-purple-500/40 bg-purple-500/10 px-2.5 py-1 text-xs font-medium text-purple-200 hover:bg-purple-500/20"
          >
            Add {family.toUpperCase()} Provider
          </button>
        )}
      </div>

      {error && <p className="text-xs text-amber-300">{error}</p>}
      {defaultNotice && <p className="text-xs text-emerald-300">{defaultNotice}</p>}

      <ul className="space-y-1">
        {rows.map((row) => {
          const isDefault = row.name === 'default';
          const isDefaultSource = row.name === defaultMarker;
          return (
            <li key={row.name} className="flex flex-wrap items-center gap-2 rounded-lg border border-stone-800 bg-stone-950/40 px-3 py-2">
              <span className={`h-2 w-2 rounded-full ${isDefaultSource ? 'bg-emerald-400' : 'bg-stone-600'}`} title={isDefaultSource ? 'Default' : ''} />
              {isLLM ? (
                <>
                  <span className="text-xs text-stone-100">{row.label}</span>
                  <span className="font-mono text-[11px] text-stone-500">{row.name}</span>
                  {row.providerLabel && <span className="font-mono text-[11px] text-purple-300/80">{row.providerLabel}</span>}
                </>
              ) : (
                <span className="font-mono text-xs text-stone-100">{row.name}</span>
              )}
              <TierBadge tier={row.tier} caveat={row.caveat} />
              {row.keyRequired !== undefined && (
                <span className={`text-[11px] ${row.keyRequired && !row.keyPresent ? 'text-amber-300' : 'text-stone-500'}`}>
                  {row.keyRequired ? (row.keyPresent ? 'key set' : 'key missing') : 'no key needed'}
                </span>
              )}
              <span className="flex-1" />
              <button
                type="button"
                onClick={() => select(row.name)}
                className={`text-[11px] ${activeSelected === row.name ? 'text-purple-300' : 'text-stone-400 hover:text-purple-300'}`}
              >
                edit
              </button>
              {!isLLM && !isDefault && (
                <button type="button" onClick={() => setDefault(row.name)} className="text-[11px] text-stone-400 hover:text-emerald-300">
                  set default
                </button>
              )}
              {!isLLM && (
                <button type="button" onClick={() => duplicateProvider(row.name)} className="text-[11px] text-stone-400 hover:text-purple-300">
                  duplicate
                </button>
              )}
              {!isLLM && !isDefault && (
                <>
                  <button
                    type="button"
                    onClick={() => {
                      const next = window.prompt('Rename provider', row.name);
                      if (next) renameProvider(row.name, next);
                    }}
                    className="text-[11px] text-stone-400 hover:text-purple-300"
                  >
                    rename
                  </button>
                  <button type="button" onClick={() => removeProvider(row.name)} className="text-[11px] text-stone-400 hover:text-rose-400">
                    remove
                  </button>
                </>
              )}
            </li>
          );
        })}
      </ul>

      {renderEditor && (
        <div className="rounded-lg border border-stone-800 bg-stone-950/40 p-3">
          {renderEditor(activeSelected, selectedValue, writeSelected)}
        </div>
      )}

      {!isLLM && purposesFor(mediaFamily).length > 0 && (
        <div className="rounded-lg border border-stone-800 bg-stone-950/40 p-3">
          <p className="mb-2 font-sans text-[11px] uppercase text-stone-400">Purposes</p>
          <div className="space-y-1.5">
            {purposesFor(mediaFamily).map((use) => (
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
