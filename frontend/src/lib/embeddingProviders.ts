import type { AppConfig, EmbeddingProviderConfig } from '../types';

// embeddingEntryNames lists the named embedding providers, sorted. Unlike the
// media families there is no reserved default row: the top-level provider
// selector names the active entry, so the default is not a name.
export function embeddingEntryNames(config: AppConfig): string[] {
  return Object.keys(config.embeddings?.providers ?? {}).sort();
}

// embeddingEntryValue returns the configuration for a name, or a built-in
// template when it has none, so an editor always has something to show.
export function embeddingEntryValue(config: AppConfig, name: string): EmbeddingProviderConfig {
  return config.embeddings?.providers?.[name] ?? { type: 'builtin' };
}

// setEmbeddingEntry returns a new config with one entry's configuration
// replaced. It leaves the top-level selector and the enabled switch alone.
export function setEmbeddingEntry(config: AppConfig, name: string, value: EmbeddingProviderConfig): AppConfig {
  const embeddings = config.embeddings ?? { enabled: false, provider: '' };
  const providers = { ...(embeddings.providers ?? {}), [name]: value };
  return { ...config, embeddings: { ...embeddings, providers } };
}

// removeEmbeddingEntry deletes a named entry. When it was the active one, the
// selector falls back to the reserved default.
export function removeEmbeddingEntry(config: AppConfig, name: string): AppConfig {
  const embeddings = config.embeddings ?? { enabled: false, provider: '' };
  const providers = { ...(embeddings.providers ?? {}) };
  delete providers[name];
  const provider = embeddings.provider === name ? '' : embeddings.provider;
  return { ...config, embeddings: { ...embeddings, providers, provider } };
}

// renameEmbeddingEntry renames a named entry, following the selector when it
// pointed at the old name.
export function renameEmbeddingEntry(config: AppConfig, oldName: string, newName: string): AppConfig {
  const embeddings = config.embeddings ?? { enabled: false, provider: '' };
  const providers = { ...(embeddings.providers ?? {}) };
  providers[newName] = providers[oldName];
  delete providers[oldName];
  const provider = embeddings.provider === oldName ? newName : embeddings.provider;
  return { ...config, embeddings: { ...embeddings, providers, provider } };
}

// embeddingSelected names the active entry, or the reserved default when none is
// chosen.
export function embeddingSelected(config: AppConfig): string {
  return config.embeddings?.provider || 'default';
}
