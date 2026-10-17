import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { VoiceCombobox } from './VoiceCombobox';
import { ProviderVoice } from '../types';

// The combobox is a filter-as-you-type picker: focus opens the whole catalogue so
// it is browsable, typing narrows it, Escape closes it, and picking one takes it.
// None of that had a test.

const voices: ProviderVoice[] = [
  { id: 'af_bella', name: 'Bella', language: 'en-US' },
  { id: 'af_sarah', name: 'Sarah', language: 'en-US' },
  { id: 'bm_george', name: 'George', language: 'en-GB' },
];

function renderCombobox(onChange = vi.fn()) {
  render(<VoiceCombobox value="" onChange={onChange} voices={voices} />);
  return onChange;
}

const toggle = () => screen.getByRole('button', { name: /voices$/ });

afterEach(() => {
  vi.restoreAllMocks();
});

describe('VoiceCombobox keyboard', () => {
  it('opens the catalogue on focus, so it is browsable without typing', () => {
    renderCombobox();

    expect(toggle()).toHaveAttribute('aria-expanded', 'false');

    fireEvent.focus(screen.getByPlaceholderText(/voice/i));

    expect(toggle()).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('Bella')).toBeInTheDocument();
    expect(screen.getByText('George')).toBeInTheDocument();
  });

  it('narrows the catalogue as the player types', () => {
    renderCombobox();
    const input = screen.getByPlaceholderText(/voice/i);

    fireEvent.change(input, { target: { value: 'geo' } });

    expect(screen.getByText('George')).toBeInTheDocument();
    expect(screen.queryByText('Bella')).not.toBeInTheDocument();
  });

  it('says when nothing matches rather than showing an empty box', () => {
    renderCombobox();

    fireEvent.change(screen.getByPlaceholderText(/voice/i), { target: { value: 'nobody' } });

    expect(screen.getByText(/no matching voices/i)).toBeInTheDocument();
  });

  it('closes on Escape', () => {
    renderCombobox();
    const input = screen.getByPlaceholderText(/voice/i);
    fireEvent.focus(input);
    expect(toggle()).toHaveAttribute('aria-expanded', 'true');

    fireEvent.keyDown(input, { key: 'Escape' });

    expect(toggle()).toHaveAttribute('aria-expanded', 'false');
  });

  it('takes the voice the player picks, and closes', () => {
    const onChange = renderCombobox();

    fireEvent.focus(screen.getByPlaceholderText(/voice/i));
    fireEvent.click(screen.getByText('Sarah'));

    expect(onChange).toHaveBeenCalledWith('af_sarah');
    expect(toggle()).toHaveAttribute('aria-expanded', 'false');
  });
});
