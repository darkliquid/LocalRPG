import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { PreferencesPanel } from './PreferencesPanel';
import type { AppConfig, StylePacks } from '../../types';

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
  styles: { pack: '' },
};

const mockStylePacks: StylePacks = {
  packs: [
    { id: 'dark-fantasy', label: 'Dark Fantasy', path: '/packs/dark-fantasy' },
    { id: 'broken-pack', label: 'Broken Pack', path: '/packs/broken', problems: ['Missing manifest'] },
  ],
  warnings: ['Failed to load custom fonts for Broken Pack'],
};

describe('PreferencesPanel', () => {
  it('toggles cinematic backdrop overlays', () => {
    const setConfig = vi.fn();
    render(<PreferencesPanel config={mockConfig} setConfig={setConfig} stylePacks={null} />);

    const cinematicBtn = screen.getByRole('button', { name: 'Disabled' });
    fireEvent.click(cinematicBtn);

    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      preferences: {
        ...mockConfig.preferences,
        cinematic_effects: true,
      },
    });
  });

  it('changes typography scaling to small and large', () => {
    const setConfig = vi.fn();
    render(<PreferencesPanel config={mockConfig} setConfig={setConfig} stylePacks={null} />);

    const smallBtn = screen.getByRole('button', { name: 'small' });
    fireEvent.click(smallBtn);

    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      preferences: {
        ...mockConfig.preferences,
        font_scale: 'small',
      },
    });

    const largeBtn = screen.getByRole('button', { name: 'large' });
    fireEvent.click(largeBtn);

    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      preferences: {
        ...mockConfig.preferences,
        font_scale: 'large',
      },
    });
  });

  it('selects style pack and displays style pack warnings', () => {
    const setConfig = vi.fn();
    render(<PreferencesPanel config={mockConfig} setConfig={setConfig} stylePacks={mockStylePacks} />);

    expect(screen.getByText('Failed to load custom fonts for Broken Pack')).toBeInTheDocument();

    const select = screen.getByRole('combobox');
    fireEvent.change(select, { target: { value: 'dark-fantasy' } });

    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      styles: {
        ...mockConfig.styles,
        pack: 'dark-fantasy',
      },
    });
  });

  it('toggles story token streaming', () => {
    const setConfig = vi.fn();
    render(<PreferencesPanel config={mockConfig} setConfig={setConfig} stylePacks={null} />);

    const streamBtn = screen.getByRole('button', { name: 'Instant Render' });
    fireEvent.click(streamBtn);

    expect(setConfig).toHaveBeenCalledWith({
      ...mockConfig,
      preferences: {
        ...mockConfig.preferences,
        streaming: true,
      },
    });
  });
});
