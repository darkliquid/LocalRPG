import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MediaPanel } from './MediaPanel';
import { APIClient } from '../../api/client';
import type { AppConfig, TTSConfig, STTConfig, ImageConfig } from '../../types';

vi.mock('../../hooks/useTTSInspect', () => ({
  useTTSInspect: () => ({
    inspect: {
      provider_key: 'tts-disabled',
      metered: false,
      catalog: { available: false, stale: false, voices: [] },
      key_present: false,
      key_required: false,
      speech_cues: { audio_tags: false, markdown_emphasis: false },
    },
    loading: false,
    refresh: vi.fn(),
    error: null,
  }),
}));

const mockConfig: AppConfig = {
  version: '1',
  paths: { systems: '', worlds: '', games: '', cache: '' },
  agents: { default_role: 'gm', roles: {} },
  media: {
    tts: { type: 'disabled', auto_play: false, master_volume: 1 },
    stt: { type: 'disabled' },
    image: { type: 'disabled', auto_generate: false },
  },
  preferences: {
    streaming: false,
    typing_speed_ms: 0,
    cinematic_effects: false,
    font_scale: 'medium',
  },
};

const mockTtsPresets: Record<string, { label: string; description: string; config: TTSConfig }> = {
  'kokoro-local': {
    label: 'Kokoro (Local)',
    description: 'Local neural TTS',
    config: { type: 'builtin', builtin_name: 'kokoro', auto_play: false, master_volume: 1 },
  },
};

const mockSttPresets: Record<string, { label: string; description: string; config: STTConfig }> = {
  'web-speech': {
    label: 'Web Speech API',
    description: 'Browser native speech recognition',
    config: { type: 'web-speech' },
  },
};

const mockImagePresets: Record<string, { label: string; description: string; config: ImageConfig }> = {
  'comfy-ui': {
    label: 'ComfyUI Local',
    description: 'Local Stable Diffusion via ComfyUI',
    config: { type: 'comfyui', auto_generate: false },
  },
};

describe('MediaPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(APIClient, 'inspectMedia').mockResolvedValue({ entries: [] });
  });

  it('renders TTS subtab by default with provider manager', () => {
    render(
      <MediaPanel
        config={mockConfig}
        setConfig={vi.fn()}
        ttsPresets={mockTtsPresets}
        sttPresets={mockSttPresets}
        imagePresets={mockImagePresets}
        models={[]}
        handleTestProvider={vi.fn()}
        testingCategory={null}
        testResult={null}
        setMissingModelPrompt={vi.fn()}
      />,
    );

    expect(screen.getByRole('tab', { name: 'TTS' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByText('TTS Providers')).toBeInTheDocument();
  });

  it('switches to STT tab and renders speech-to-text editor', () => {
    render(
      <MediaPanel
        config={mockConfig}
        setConfig={vi.fn()}
        ttsPresets={mockTtsPresets}
        sttPresets={mockSttPresets}
        imagePresets={mockImagePresets}
        models={[]}
        handleTestProvider={vi.fn()}
        testingCategory={null}
        testResult={null}
        setMissingModelPrompt={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('tab', { name: 'STT' }));

    expect(screen.getByRole('tab', { name: 'STT' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByText('STT Providers')).toBeInTheDocument();
  });

  it('switches to Images tab and renders image generation editor', () => {
    render(
      <MediaPanel
        config={mockConfig}
        setConfig={vi.fn()}
        ttsPresets={mockTtsPresets}
        sttPresets={mockSttPresets}
        imagePresets={mockImagePresets}
        models={[]}
        handleTestProvider={vi.fn()}
        testingCategory={null}
        testResult={null}
        setMissingModelPrompt={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('tab', { name: 'Images' }));

    expect(screen.getByRole('tab', { name: 'Images' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByText('IMAGE Providers')).toBeInTheDocument();
  });
});
