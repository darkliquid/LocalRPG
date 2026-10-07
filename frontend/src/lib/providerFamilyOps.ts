import type { AppConfig, EmbeddingProviderConfig } from '../types';
import {
  ProviderFamily,
  entryNames,
  mediaEntryValue,
  providersKey,
  purposesFor,
  setMediaEntry,
} from './mediaProviders';
import {
  embeddingEntryNames,
  embeddingEntryValue,
  embeddingSelected,
  removeEmbeddingEntry,
  renameEmbeddingEntry,
  setEmbeddingEntry,
  setEmbeddingSelected,
} from './embeddingProviders';

// ManagerFamily is the set of families the provider manager can render. It is
// the media families plus llm (roles) and embedding (its own top-level config).
export type ManagerFamily = ProviderFamily | 'llm' | 'embedding';

// FamilyOps is the config-shape adapter the provider manager works through. The
// media families share one shape (a singleton default, a named map, and a purpose
// map); embeddings sit at the top level with a selector and no default row, so
// they supply their own.
export interface FamilyOps {
  inspect: string;
  heading: string;
  addLabel: string;
  isLLM: boolean;
  names(config: AppConfig): string[];
  providerMap(config: AppConfig): Record<string, unknown>;
  value(config: AppConfig, name: string): unknown;
  write(config: AppConfig, name: string, value: unknown): AppConfig;
  template(config: AppConfig): unknown;
  setDefault(config: AppConfig, name: string): AppConfig;
  remove(config: AppConfig, name: string): AppConfig;
  rename(config: AppConfig, oldName: string, newName: string): AppConfig;
  defaultName(config: AppConfig): string;
  purposes(config: AppConfig): Record<string, string> | undefined;
  purposeList: string[];
  hasDefaultRow: boolean;
  initialSelection(config: AppConfig): string;
}

// renameInPurposes rewrites every purpose that pointed at oldName.
function renameInPurposes(purposes: Record<string, string> | undefined, oldName: string, newName: string): Record<string, string> {
  const next: Record<string, string> = { ...(purposes ?? {}) };
  for (const use of Object.keys(next)) {
    if (next[use] === oldName) next[use] = newName;
  }
  return next;
}

function mediaOps(family: ProviderFamily): FamilyOps {
  const providersOf = (config: AppConfig) => (config.media[providersKey(family)] ?? {}) as Record<string, unknown>;
  const withProviders = (config: AppConfig, providers: Record<string, unknown>): AppConfig => ({
    ...config,
    media: { ...config.media, [providersKey(family)]: providers },
  });

  return {
    inspect: family,
    heading: `${family.toUpperCase()} Providers`,
    addLabel: `Add ${family.toUpperCase()} Provider`,
    isLLM: false,
    names: (config) => entryNames(config, family),
    providerMap: (config) => providersOf(config),
    value: (config, name) => mediaEntryValue(config, family, name),
    write: (config, name, value) => setMediaEntry(config, family, name, value),
    template: (config) => ({ ...(config.media[family] as object) }),
    setDefault: (config, name) => setMediaEntry(config, family, 'default', { ...(providersOf(config)[name] as object) }),
    remove: (config, name) => {
      const providers = { ...providersOf(config) };
      delete providers[name];
      return withProviders(config, providers);
    },
    rename: (config, oldName, newName) => {
      const providers = { ...providersOf(config) };
      providers[newName] = providers[oldName];
      delete providers[oldName];
      return {
        ...withProviders(config, providers),
        media: {
          ...config.media,
          [providersKey(family)]: providers,
          purposes: renameInPurposes(config.media.purposes, oldName, newName),
        },
      };
    },
    defaultName: () => 'default',
    purposes: (config) => config.media.purposes,
    purposeList: purposesFor(family),
    hasDefaultRow: true,
    initialSelection: () => 'default',
  };
}

const embeddingOps: FamilyOps = {
  inspect: 'embedding',
  heading: 'Embedding Providers',
  addLabel: 'Add Embedding Provider',
  isLLM: false,
  names: (config) => embeddingEntryNames(config),
  providerMap: (config) => (config.embeddings?.providers ?? {}) as Record<string, unknown>,
  value: (config, name) => embeddingEntryValue(config, name),
  write: (config, name, value) => setEmbeddingEntry(config, name, value as EmbeddingProviderConfig),
  template: () => ({ type: 'builtin' }),
  setDefault: (config, name) => setEmbeddingSelected(config, name),
  remove: (config, name) => removeEmbeddingEntry(config, name),
  rename: (config, oldName, newName) => renameEmbeddingEntry(config, oldName, newName),
  defaultName: (config) => embeddingSelected(config),
  purposes: () => undefined,
  purposeList: [],
  hasDefaultRow: false,
  initialSelection: (config) => embeddingEntryNames(config)[0] ?? 'default',
};

// familyOps resolves the adapter for a manager family.
export function familyOps(family: ManagerFamily): FamilyOps {
  if (family === 'embedding') return embeddingOps;
  if (family === 'llm') throw new Error('llm roles do not use the config adapter');
  return mediaOps(family);
}
