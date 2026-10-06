import { describe, expect, it } from 'vitest';
import type { AppConfig, ImageConfig, STTConfig, TTSConfig } from '../types';
import { entryNames, mediaEntryValue, providersKey, purposesFor, setMediaEntry, uniqueName } from './mediaProviders';

const tts = (over: Partial<TTSConfig> = {}): TTSConfig => ({ type: 'disabled', auto_play: false, master_volume: 1, ...over });
const stt = (over: Partial<STTConfig> = {}): STTConfig => ({ type: 'disabled', ...over });
const image = (over: Partial<ImageConfig> = {}): ImageConfig => ({ type: 'disabled', auto_generate: false, ...over });

const baseConfig = (): AppConfig => ({
  version: '1',
  paths: { systems: '', worlds: '', games: '', cache: '' },
  agents: { default_role: 'gm', roles: {} },
  media: { tts: tts(), stt: stt(), image: image() },
  preferences: { streaming: false, typing_speed_ms: 0, cinematic_effects: false, font_scale: 'medium' },
});

describe('media provider entries', () => {
  it('names the field that holds a family of providers', () => {
    expect(providersKey('tts')).toBe('tts_providers');
    expect(providersKey('stt')).toBe('stt_providers');
    expect(providersKey('image')).toBe('image_providers');
  });

  it('reads the singleton for the default and the map entry for a name', () => {
    const config = baseConfig();
    config.media.tts = tts({ type: 'http', endpoint: 'singleton' });
    config.media.tts_providers = { alt: tts({ type: 'http', endpoint: 'named' }) };

    expect(mediaEntryValue(config, 'tts', 'default')).toBe(config.media.tts);
    expect(mediaEntryValue(config, 'tts', 'alt')).toBe(config.media.tts_providers.alt);
  });

  it('falls back to the singleton when a name has no entry', () => {
    const config = baseConfig();
    expect(mediaEntryValue(config, 'image', 'missing')).toBe(config.media.image);
  });

  it('writes a named entry into the map without touching the singleton', () => {
    const config = baseConfig();
    const next = setMediaEntry(config, 'tts', 'alt', tts({ type: 'http', endpoint: 'named' }));

    expect(next.media.tts_providers?.alt).toEqual(tts({ type: 'http', endpoint: 'named' }));
    expect(next.media.tts).toBe(config.media.tts);
  });

  it('writes the default into the singleton without adding an empty map', () => {
    const config = baseConfig();
    const next = setMediaEntry(config, 'tts', 'default', tts({ type: 'http', endpoint: 'singleton' }));

    expect(next.media.tts).toEqual(tts({ type: 'http', endpoint: 'singleton' }));
    expect('tts_providers' in next.media).toBe(false);
  });

  it('round-trips an untouched config byte-identically', () => {
    const config = baseConfig();
    const next = setMediaEntry(config, 'tts', 'default', mediaEntryValue(config, 'tts', 'default'));
    expect(JSON.stringify(next)).toBe(JSON.stringify(config));
  });

  it('lists default first, then the names sorted', () => {
    const config = baseConfig();
    config.media.image_providers = { zeta: image(), alpha: image() };
    expect(entryNames(config, 'image')).toEqual(['default', 'alpha', 'zeta']);
  });

  it('suffixes a taken name until it is free', () => {
    expect(uniqueName('new-provider', {})).toBe('new-provider');
    expect(uniqueName('new-provider', { 'new-provider': {} })).toBe('new-provider-2');
    expect(uniqueName('new-provider', { 'new-provider': {}, 'new-provider-2': {} })).toBe('new-provider-3');
  });

  it('names the purposes of each family', () => {
    expect(purposesFor('tts')).toEqual(['narrator', 'npc']);
    expect(purposesFor('image')).toEqual(['scene', 'portrait', 'placeholder']);
    expect(purposesFor('stt')).toEqual([]);
  });
});
