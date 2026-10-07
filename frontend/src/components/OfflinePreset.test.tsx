import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { OfflinePreset } from './OfflinePreset';
import { APIClient } from '../api/client';

vi.mock('../api/client', () => ({
  APIClient: {
    applyOfflinePreset: vi.fn().mockResolvedValue({
      changes: [
        'GM: set to narrative-oracle (builtin)',
        'TTS: set to native-os (builtin)',
        'Image: set to procedural-art (builtin)',
        'STT: disabled',
        'Embeddings: set to builtin-local (hash-projection)',
      ],
    }),
    checkOffline: vi.fn().mockResolvedValue({
      offline: true,
      issues: [],
    }),
    getSettings: vi.fn().mockResolvedValue({
      config: {
        version: '1',
        paths: { systems: '', worlds: '', games: '', cache: '' },
        agents: { default_role: 'gm', roles: {} },
        media: {
          tts: { type: 'builtin', builtin_name: 'native-os' },
          stt: { type: 'disabled' },
          image: { type: 'builtin', builtin_name: 'procedural-art' },
        },
        preferences: { streaming: false, typing_speed_ms: 0, cinematic_effects: false, font_scale: 'medium' },
      },
    }),
  },
}));

describe('OfflinePreset', () => {
  it('shows what the offline preset will change', () => {
    render(<OfflinePreset onChange={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /offline preset/i }));

    // Assert a confirmation listing the affected roles/families appears
    expect(screen.getByText(/confirm offline preset/i)).toBeInTheDocument();
    expect(screen.getByText(/narrative-oracle/i)).toBeInTheDocument();
    expect(screen.getByText(/procedural-art/i)).toBeInTheDocument();
  });

  it('applies the offline preset upon confirmation and reports changes', async () => {
    const onChange = vi.fn();
    render(<OfflinePreset onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: /offline preset/i }));
    fireEvent.click(screen.getByRole('button', { name: /apply preset/i }));

    await waitFor(() => {
      expect(APIClient.applyOfflinePreset).toHaveBeenCalledWith('native-os');
      expect(onChange).toHaveBeenCalled();
    });
    expect(await screen.findByText(/applied offline preset/i)).toBeInTheDocument();
  });

  it('checks offline status and displays a clean report', async () => {
    render(<OfflinePreset onChange={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /check offline/i }));

    await waitFor(() => {
      expect(APIClient.checkOffline).toHaveBeenCalled();
    });
    expect(await screen.findByText(/all configured providers are offline/i)).toBeInTheDocument();
  });

  it('checks offline status and displays issues when present', async () => {
    vi.mocked(APIClient.checkOffline).mockResolvedValueOnce({
      offline: false,
      issues: [
        {
          role: 'gm',
          provider_key: 'llm:gemini',
          tier: 'cloud',
          reason: 'reaches external network or cloud services',
        },
      ],
    });

    render(<OfflinePreset onChange={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: /check offline/i }));

    expect(await screen.findByText(/provider configuration is not fully offline/i)).toBeInTheDocument();
    expect(screen.getByText(/llm:gemini/i)).toBeInTheDocument();
  });
});
