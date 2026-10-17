import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { SettingsStudio } from './SettingsStudio';
import { SettingsProvider } from '../context/SettingsContext';
import { APIClient } from '../api/client';

const { config } = vi.hoisted(() => ({
  config: {
    version: '1',
    paths: { systems: '', worlds: '', games: '', cache: '' },
    agents: {
      default_role: 'gm',
      roles: {
        gm: { type: 'gemini' as const, model: 'gemini-3.8-flash' },
        extractor: { type: 'inherit' as const, inherit_from: 'gm' },
      },
    },
    media: {
      tts: { type: 'disabled' as const, auto_play: false, master_volume: 1 },
      stt: { type: 'disabled' as const },
      image: { type: 'disabled' as const, auto_generate: false },
    },
    preferences: { streaming: false, typing_speed_ms: 0, cinematic_effects: false, font_scale: 'medium' as const },
  },
}));

vi.mock('../api/client', () => ({
  APIClient: {
    getSettings: vi.fn(),
    saveSettings: vi.fn(),
    getModels: vi.fn().mockResolvedValue([]),
    subscribeModelEvents: vi.fn(() => () => {}),
    getStylePacks: vi.fn().mockResolvedValue({ packs: [], warnings: [] }),
    listProviders: vi.fn().mockResolvedValue({ providers: [] }),
    inspectTTS: vi.fn().mockResolvedValue({}),
    inspectMedia: vi.fn().mockResolvedValue({ entries: [] }),
    listGeminiModels: vi.fn(),
  },
}));

const renderStudio = () =>
  render(
    <SettingsProvider>
      <SettingsStudio />
    </SettingsProvider>,
  );

describe('SettingsStudio', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    vi.mocked(APIClient.getSettings).mockResolvedValue({
      config,
      config_file_path: '/tmp/localrpg.yaml',
      is_local_override: false,
      warnings: [],
    });
    vi.mocked(APIClient.saveSettings).mockResolvedValue({
      config,
      config_file_path: '/tmp/localrpg.yaml',
      is_local_override: false,
      warnings: [],
    });
    vi.mocked(APIClient.getStylePacks).mockResolvedValue({ packs: [], warnings: [] });
    vi.mocked(APIClient.listGeminiModels).mockResolvedValue({ models: [] });
  });

  describe('loading state', () => {
    it('shows loading message while settings are fetching', () => {
      vi.mocked(APIClient.getSettings).mockReturnValue(new Promise(() => {}));

      renderStudio();

      expect(screen.getByText('Loading system configuration...')).toBeInTheDocument();
    });
  });

  describe('configuration warnings', () => {
    it('displays configuration problems when warnings are returned', async () => {
      vi.mocked(APIClient.getSettings).mockResolvedValue({
        config,
        config_file_path: '/tmp/localrpg.yaml',
        is_local_override: false,
        warnings: ['GEMINI_API_KEY is not set', 'Audio cache directory is read-only'],
      });

      renderStudio();

      await waitFor(() => {
        expect(screen.getByText('Configuration problems')).toBeInTheDocument();
        expect(screen.getByText('GEMINI_API_KEY is not set')).toBeInTheDocument();
        expect(screen.getByText('Audio cache directory is read-only')).toBeInTheDocument();
      });
    });
  });

  describe('save settings feedback and error states', () => {
    it('shows success feedback banner when saving settings succeeds', async () => {
      renderStudio();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Save Settings' })).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole('button', { name: 'Save Settings' }));

      await waitFor(() => {
        expect(screen.getByText('Settings saved and live-reloaded successfully.')).toBeInTheDocument();
      });
    });

    it('shows error feedback banner when saving settings fails', async () => {
      vi.mocked(APIClient.saveSettings).mockRejectedValue(new Error('Permission denied writing config file'));

      renderStudio();

      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Save Settings' })).toBeInTheDocument();
      });

      fireEvent.click(screen.getByRole('button', { name: 'Save Settings' }));

      await waitFor(() => {
        expect(screen.getByText('Permission denied writing config file')).toBeInTheDocument();
      });
    });
  });

  describe('style pack warnings in preferences', () => {
    it('renders style pack warnings in PreferencesPanel', async () => {
      vi.mocked(APIClient.getStylePacks).mockResolvedValue({
        packs: [
          {
            id: 'broken-pack',
            label: 'Broken Pack',
            path: '/tmp/broken-pack',
            problems: ['Missing manifest'],
          },
        ],
        warnings: ['Failed to parse theme for broken-pack'],
      });

      renderStudio();

      fireEvent.click(await screen.findByRole('button', { name: 'Preferences' }));

      await waitFor(() => {
        expect(screen.getByText('Failed to parse theme for broken-pack')).toBeInTheDocument();
        expect(screen.getByText('Broken Pack (invalid)')).toBeInTheDocument();
      });
    });
  });

  describe('AI Agents secondary tabs and controls', () => {
    const openAgents = async () => {
      renderStudio();
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
      renderStudio();
      fireEvent.click(await screen.findByRole('button', { name: 'Media Engines' }));

      expect(screen.getByRole('tab', { name: 'TTS' })).toHaveAttribute('aria-selected', 'true');
      expect(screen.getByText('Text-to-Speech (TTS) Engine')).toBeInTheDocument();
      expect(screen.queryByText('Speech-to-Text (STT) Engine')).not.toBeInTheDocument();
      expect(screen.queryByText('Scene Art / Image Generator')).not.toBeInTheDocument();

      fireEvent.click(screen.getByRole('tab', { name: 'STT' }));

      expect(screen.getByText('Speech-to-Text (STT) Engine')).toBeInTheDocument();
      expect(screen.queryByText('Text-to-Speech (TTS) Engine')).not.toBeInTheDocument();
    });

    it('displays error when fetching Gemini models fails', async () => {
      vi.mocked(APIClient.listGeminiModels).mockRejectedValue(new Error('Invalid Gemini API Key'));

      await openAgents();

      const fetchBtn = screen.getByText('Fetch available models');
      fireEvent.click(fetchBtn);

      await waitFor(() => {
        expect(screen.getByText('Invalid Gemini API Key')).toBeInTheDocument();
      });
    });
  });
});
