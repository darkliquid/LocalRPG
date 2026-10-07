import { describe, expect, it } from 'vitest';
import type { AppConfig, ImageConfig, STTConfig, TTSConfig } from '../types';
import {
  embeddingEntryNames,
  embeddingEntryValue,
  embeddingSelected,
  removeEmbeddingEntry,
  renameEmbeddingEntry,
  setEmbeddingEntry,
} from './embeddingProviders';

const tts = (): TTSConfig => ({ type: 'disabled', auto_play: false, master_volume: 1 });
const stt = (): STTConfig => ({ type: 'disabled' });
const image = (): ImageConfig => ({ type: 'disabled', auto_generate: false });

const baseConfig = (): AppConfig => ({
  version: '1',
  paths: { systems: '', worlds: '', games: '', cache: '' },
  agents: { default_role: 'gm', roles: {} },
  media: { tts: tts(), stt: stt(), image: image() },
  preferences: { streaming: false, typing_speed_ms: 0, cinematic_effects: false, font_scale: 'medium' },
});

describe('embedding provider entries', () => {
  it('lists the named entries sorted', () => {
    const config: AppConfig = {
      ...baseConfig(),
      embeddings: { enabled: true, provider: 'local', providers: { zeta: { type: 'builtin' }, alpha: { type: 'onnx' } } },
    };
    expect(embeddingEntryNames(config)).toEqual(['alpha', 'zeta']);
  });

  it('reads a named entry and falls back to a builtin template', () => {
    const config: AppConfig = {
      ...baseConfig(),
      embeddings: { enabled: true, provider: 'oa', providers: { oa: { type: 'http', endpoint: 'https://api.openai.com/v1' } } },
    };
    expect(embeddingEntryValue(config, 'oa').type).toBe('http');
    expect(embeddingEntryValue(config, 'missing').type).toBe('builtin');
  });

  it('writes a named entry without touching the selector', () => {
    const config: AppConfig = { ...baseConfig(), embeddings: { enabled: true, provider: 'local', providers: {} } };
    const next = setEmbeddingEntry(config, 'oa', { type: 'http', endpoint: 'https://api.openai.com/v1' });
    expect(next.embeddings?.providers?.oa).toEqual({ type: 'http', endpoint: 'https://api.openai.com/v1' });
    expect(next.embeddings?.provider).toBe('local');
  });

  it('resets the selector when the active entry is removed', () => {
    const config: AppConfig = {
      ...baseConfig(),
      embeddings: { enabled: true, provider: 'local', providers: { local: { type: 'onnx' }, oa: { type: 'http' } } },
    };
    const next = removeEmbeddingEntry(config, 'local');
    expect(next.embeddings?.providers?.local).toBeUndefined();
    expect(next.embeddings?.provider).toBe('');
    expect(embeddingSelected(next)).toBe('default');
  });

  it('follows the selector when the active entry is renamed', () => {
    const config: AppConfig = {
      ...baseConfig(),
      embeddings: { enabled: true, provider: 'local', providers: { local: { type: 'onnx' } } },
    };
    const next = renameEmbeddingEntry(config, 'local', 'encoder');
    expect(next.embeddings?.providers?.encoder).toEqual({ type: 'onnx' });
    expect(next.embeddings?.providers?.local).toBeUndefined();
    expect(next.embeddings?.provider).toBe('encoder');
  });

  it('reports the reserved default when no selector is set', () => {
    expect(embeddingSelected(baseConfig())).toBe('default');
  });
});
