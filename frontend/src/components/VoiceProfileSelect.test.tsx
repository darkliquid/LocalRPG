import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { VoiceProfileSelect } from './VoiceProfileSelect';
import { VoiceProfile } from '../types';

// The profile picker is the same filter-as-you-type control as the provider
// catalogue, over the campaign's own profiles, plus a row that clears the
// selection so a role can fall back to the provider's default voice.

const profiles: VoiceProfile[] = [
  {
    id: 'narrator',
    name: 'The Narrator',
    provider: 'builtin',
    voice_id: 'af_bella',
    pitch: 0,
    speech_rate: 1,
  },
  {
    id: 'garrick',
    name: 'Garrick',
    provider: 'builtin',
    voice_id: 'bm_george',
    pitch: -1,
    speech_rate: 0.95,
  },
];

function renderSelect(onChange = vi.fn()) {
  render(
    <VoiceProfileSelect
      profiles={profiles}
      value=""
      onChange={onChange}
      allowDefault
      defaultLabel="Provider default"
    />,
  );
  return onChange;
}

const toggle = () => screen.getByRole('button', { name: /voice profiles$/ });

afterEach(() => {
  vi.restoreAllMocks();
});

describe('VoiceProfileSelect keyboard', () => {
  it('opens the profiles on focus, so they are browsable without typing', () => {
    renderSelect();

    expect(toggle()).toHaveAttribute('aria-expanded', 'false');

    fireEvent.focus(screen.getByPlaceholderText(/voice/i));

    expect(toggle()).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('The Narrator')).toBeInTheDocument();
    expect(screen.getByText('Garrick')).toBeInTheDocument();
  });

  it('narrows the profiles as the player types', () => {
    renderSelect();

    fireEvent.change(screen.getByPlaceholderText(/voice/i), { target: { value: 'garrick' } });

    expect(screen.getByText('Garrick')).toBeInTheDocument();
    expect(screen.queryByText('The Narrator')).not.toBeInTheDocument();
  });

  it('says when nothing matches', () => {
    renderSelect();

    fireEvent.change(screen.getByPlaceholderText(/voice/i), { target: { value: 'nobody' } });

    expect(screen.getByText(/no matching voice profiles/i)).toBeInTheDocument();
  });

  it('closes on Escape', () => {
    renderSelect();
    const input = screen.getByPlaceholderText(/voice/i);
    fireEvent.focus(input);

    fireEvent.keyDown(input, { key: 'Escape' });

    expect(toggle()).toHaveAttribute('aria-expanded', 'false');
  });

  it('takes the profile the player picks', () => {
    const onChange = renderSelect();

    fireEvent.focus(screen.getByPlaceholderText(/voice/i));
    fireEvent.click(screen.getByText('The Narrator'));

    expect(onChange).toHaveBeenCalledWith(profiles[0]);
    expect(toggle()).toHaveAttribute('aria-expanded', 'false');
  });

  it('clears the selection through the default row', () => {
    const onChange = renderSelect();

    fireEvent.focus(screen.getByPlaceholderText(/voice/i));
    fireEvent.click(screen.getByText('Provider default'));

    expect(onChange).toHaveBeenCalledWith(null);
  });
});
