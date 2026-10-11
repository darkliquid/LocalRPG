import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { SettingsStudio } from './SettingsStudio';

const { config } = vi.hoisted(() => ({
  config: {
    version: '1',
    paths: { systems: '', worlds: '', games: '', cache: '' },
    agents: {
      default_role: 'gm',
      roles: {
        gm: { type: 'gemini', model: 'gemini-3.8-flash' },
        extractor: { type: 'inherit', inherit_from: 'gm' },
      },
    },
    media: {
      tts: { type: 'disabled', auto_play: false, master_volume: 1 },
      stt: { type: 'disabled' },
      image: { type: 'disabled', auto_generate: false },
    },
    preferences: { streaming: false, typing_speed_ms: 0, cinematic_effects: false, font_scale: 'medium' },
  },
}));

vi.mock('../api/client', () => ({
  APIClient: {
    getSettings: vi.fn().mockResolvedValue({
      config,
      config_file_path: '/tmp/localrpg.yaml',
      is_local_override: false,
      warnings: [],
    }),
    getModels: vi.fn().mockResolvedValue([]),
    subscribeModelEvents: vi.fn(() => () => {}),
    getStylePacks: vi.fn().mockResolvedValue(null),
    listProviders: vi.fn().mockResolvedValue({ providers: [] }),
    inspectTTS: vi.fn().mockResolvedValue({}),
    inspectMedia: vi.fn().mockResolvedValue({ entries: [] }),
  },
}));

describe('SettingsStudio secondary tabs', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  const openAgents = async () => {
    render(<SettingsStudio />);
    fireEvent.click(await screen.findByRole('button', { name: 'AI Agents' }));
  };

  it('splits AI Agents into Roles and Routing without an Advanced tab by default', async () => {
    await openAgents();

    expect(screen.getByRole('tab', { name: 'Roles' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Routing' })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Advanced' })).not.toBeInTheDocument();
  });

  it('keeps the role editor limits behind its own disclosure', async () => {
    await openAgents();

    expect(screen.queryByText('Response Limit (max tokens)')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Advanced' }));

    expect(screen.getByText('Response Limit (max tokens)')).toBeInTheDocument();
  });

  it('shows the Advanced tab and expands the limits once the preference is on', async () => {
    await openAgents();

    fireEvent.click(screen.getByRole('checkbox', { name: /show advanced settings/i }));

    expect(screen.getByRole('tab', { name: 'Advanced' })).toBeInTheDocument();
    expect(screen.getByText('Response Limit (max tokens)')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: 'Advanced' }));

    expect(screen.getByText('Context & Response Limits')).toBeInTheDocument();
    expect(screen.getByText('Generation Limits')).toBeInTheDocument();
  });

  it('returns to Roles when the preference is switched off from the Advanced tab', async () => {
    await openAgents();

    const toggle = screen.getByRole('checkbox', { name: /show advanced settings/i });
    fireEvent.click(toggle);
    fireEvent.click(screen.getByRole('tab', { name: 'Advanced' }));
    fireEvent.click(toggle);

    expect(screen.queryByRole('tab', { name: 'Advanced' })).not.toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Roles' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByText('Context & Response Limits')).not.toBeInTheDocument();
  });

  it('shows one media engine at a time', async () => {
    render(<SettingsStudio />);
    fireEvent.click(await screen.findByRole('button', { name: 'Media Engines' }));

    expect(screen.getByRole('tab', { name: 'TTS' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByText('Text-to-Speech (TTS) Engine')).toBeInTheDocument();
    expect(screen.queryByText('Speech-to-Text (STT) Engine')).not.toBeInTheDocument();
    expect(screen.queryByText('Scene Art / Image Generator')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: 'STT' }));

    expect(screen.getByText('Speech-to-Text (STT) Engine')).toBeInTheDocument();
    expect(screen.queryByText('Text-to-Speech (TTS) Engine')).not.toBeInTheDocument();
  });
});