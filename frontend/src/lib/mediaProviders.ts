import type { AppConfig, ImageConfig, STTConfig, TTSConfig } from '../types';

export type ProviderFamily = 'tts' | 'stt' | 'image';

export type MediaEntryConfig = TTSConfig | STTConfig | ImageConfig;

// providersKey is the config field holding a family's named providers.
export function providersKey(family: ProviderFamily): 'tts_providers' | 'stt_providers' | 'image_providers' {
  return `${family}_providers` as const;
}

// mediaEntryValue returns the configuration for a name: the singleton for
// "default", the named map entry otherwise. A name with no entry falls back to
// the singleton, so a removed entry never leaves an editor with nothing to show.
export function mediaEntryValue(config: AppConfig, family: ProviderFamily, name: string): MediaEntryConfig {
  if (name === 'default') return config.media[family];
  const providers = config.media[providersKey(family)] as Record<string, MediaEntryConfig> | undefined;
  return providers?.[name] ?? config.media[family];
}

// setMediaEntry returns a new config with one entry's configuration replaced.
// Writing the default replaces the singleton and never creates an empty named
// map, so an untouched config stays byte-identical.
export function setMediaEntry(config: AppConfig, family: ProviderFamily, name: string, value: unknown): AppConfig {
  if (name === 'default') {
    const media = { ...config.media, [family]: value } as AppConfig['media'];
    return { ...config, media };
  }
  const providers = (config.media[providersKey(family)] ?? {}) as Record<string, unknown>;
  const media = { ...config.media, [providersKey(family)]: { ...providers, [name]: value } } as AppConfig['media'];
  return { ...config, media };
}

// entryNames lists "default" first, then the named entries sorted.
export function entryNames(config: AppConfig, family: ProviderFamily): string[] {
  const providers = (config.media[providersKey(family)] ?? {}) as Record<string, unknown>;
  return ['default', ...Object.keys(providers).sort()];
}

// uniqueName appends a numeric suffix until the name is free.
export function uniqueName(base: string, taken: Record<string, unknown>): string {
  if (!(base in taken)) return base;
  let n = 2;
  while (`${base}-${n}` in taken) n++;
  return `${base}-${n}`;
}

// purposesFor is the family's uses, in display order. STT has none yet.
export function purposesFor(family: ProviderFamily): string[] {
  if (family === 'tts') return ['narrator', 'npc'];
  if (family === 'image') return ['scene', 'portrait', 'placeholder'];
  return [];
}
