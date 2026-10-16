import { render, screen, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { SettingsProvider, useSettings } from './SettingsContext';
import { APIClient } from '../api/client';
import { AppConfig } from '../types';

const config = { media: { tts: { master_volume: 1 } } } as unknown as AppConfig;
const saved = { ...config, media: { tts: { master_volume: 0.5 } } } as unknown as AppConfig;

function Consumer({ label }: { label: string }) {
  const { settings, isLoading } = useSettings();
  return (
    <span>
      {label}:{isLoading ? 'loading' : (settings?.config.media.tts.master_volume ?? 'none')}
    </span>
  );
}

function ApplyButton() {
  const { settings, applySettings } = useSettings();
  return (
    <button type="button" onClick={() => applySettings({ ...settings!, config: saved })}>
      Apply
    </button>
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('SettingsProvider', () => {
  it('fetches once for every consumer', async () => {
    const get = vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
      config,
      config_file_path: '',
      is_local_override: false,
    });

    render(
      <SettingsProvider>
        <Consumer label="one" />
        <Consumer label="two" />
      </SettingsProvider>,
    );

    await waitFor(() => expect(screen.getByText('one:1')).toBeInTheDocument());
    expect(screen.getByText('two:1')).toBeInTheDocument();
    expect(get).toHaveBeenCalledTimes(1);
  });

  it('applies a saved config without asking for it again', async () => {
    const get = vi.spyOn(APIClient, 'getSettings').mockResolvedValue({
      config,
      config_file_path: '',
      is_local_override: false,
    });

    render(
      <SettingsProvider>
        <Consumer label="one" />
        <ApplyButton />
      </SettingsProvider>,
    );

    await waitFor(() => expect(screen.getByText('one:1')).toBeInTheDocument());
    screen.getByRole('button', { name: 'Apply' }).click();

    // A config the server has already accepted is the freshest copy there is, so a
    // save must not cost a second round trip.
    await waitFor(() => expect(screen.getByText('one:0.5')).toBeInTheDocument());
    expect(get).toHaveBeenCalledTimes(1);
  });

  it('reports a failed load and still stops loading', async () => {
    vi.spyOn(APIClient, 'getSettings').mockRejectedValue(new Error('offline'));

    render(
      <SettingsProvider>
        <Consumer label="one" />
      </SettingsProvider>,
    );

    await waitFor(() => expect(screen.getByText('one:none')).toBeInTheDocument());
  });

  it('reads as loading with no provider rather than throwing', () => {
    render(<Consumer label="alone" />);

    expect(screen.getByText('alone:loading')).toBeInTheDocument();
  });
});
