import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { ProviderManager } from './ProviderManager';
import type { AppConfig, ImageConfig, STTConfig, TTSConfig } from '../types';

vi.mock('../api/client', () => ({
  APIClient: { inspectMedia: vi.fn().mockResolvedValue({ entries: [] }) },
}));

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

describe('ProviderManager', () => {
  it('renders the embedding family and its entries', async () => {
    const config: AppConfig = {
      ...baseConfig(),
      embeddings: { enabled: true, provider: 'local', providers: { local: { type: 'onnx' } } },
    };
    render(<ProviderManager family="embedding" config={config} onChange={() => {}} />);
    expect(await screen.findByText('Embedding Providers')).toBeInTheDocument();
    expect(screen.getByText('local')).toBeInTheDocument();
  });

  it('adds an embedding entry from the builtin template', () => {
    const onChange = vi.fn();
    render(<ProviderManager family="embedding" config={baseConfig()} onChange={onChange} />);
    fireEvent.click(screen.getByText(/add embedding provider/i));
    const next = onChange.mock.calls[onChange.mock.calls.length - 1][0] as AppConfig;
    expect(next.embeddings?.providers?.['new-provider']).toEqual({ type: 'builtin' });
  });

  it('marks an embedding entry as the active provider', () => {
    const onChange = vi.fn();
    const config: AppConfig = {
      ...baseConfig(),
      embeddings: { enabled: true, provider: '', providers: { local: { type: 'onnx' }, oa: { type: 'http' } } },
    };
    render(<ProviderManager family="embedding" config={config} onChange={onChange} />);
    fireEvent.click(screen.getAllByText(/set default/i)[0]);
    const next = onChange.mock.calls[onChange.mock.calls.length - 1][0] as AppConfig;
    expect(next.embeddings?.provider).toBe('local');
  });

  it('still renders the media default row', () => {
    render(<ProviderManager family="tts" config={baseConfig()} onChange={() => {}} />);
    expect(screen.getByText('default', { selector: 'span' })).toBeInTheDocument();
  });
});
