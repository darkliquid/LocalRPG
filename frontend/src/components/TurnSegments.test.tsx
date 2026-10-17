import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { TurnSegments } from './TurnSegments';
import type { TurnSegment, TurnCheck } from '../types';

vi.mock('../hooks/useSegmentPlayback', () => ({
  useSegmentPlayback: () => ({
    playing: false,
    blocked: false,
    playingIndex: -1,
    play: vi.fn(),
    playFrom: vi.fn(),
    regenerateFrom: vi.fn(),
    stop: vi.fn(),
  }),
}));

describe('TurnSegments', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders fallback text when no segments are provided', () => {
    render(<TurnSegments fallback="The night is silent and still." />);
    expect(screen.getByText('The night is silent and still.')).toBeInTheDocument();
  });

  it('renders narration and speech segments with speaker labels', () => {
    const segments: TurnSegment[] = [
      {
        kind: 'narration',
        text: 'The innkeeper polishes a dirty glass.',
      },
      {
        kind: 'speech',
        speaker: 'Barkeep',
        text: 'What brings strangers to these woods?',
        player: false,
      },
      {
        kind: 'speech',
        speaker: 'Hero',
        text: 'We are seeking shelter from the storm.',
        player: true,
      },
    ];

    render(<TurnSegments segments={segments} />);

    expect(screen.getByText('The innkeeper polishes a dirty glass.')).toBeInTheDocument();
    expect(screen.getByText('Barkeep')).toBeInTheDocument();
    expect(screen.getByText(/What brings strangers to these woods/)).toBeInTheDocument();
    expect(screen.getByText('Hero')).toBeInTheDocument();
    expect(screen.getByText('(you)')).toBeInTheDocument();
    expect(screen.getByText(/We are seeking shelter from the storm/)).toBeInTheDocument();
  });

  it('renders inline dice check cards attached to segments and unattached', () => {
    const checks: TurnCheck[] = [
      {
        check_id: 'check-1',
        stakes: 'Spot the hidden trap',
        outcome: 'success',
      },
      {
        check_id: 'check-2',
        stakes: 'Sneak past the guards',
        outcome: 'failure',
      },
    ];

    const segments: TurnSegment[] = [
      {
        kind: 'narration',
        text: 'You peer into the shadows.',
        check_ref: 'check-1',
      },
    ];

    render(<TurnSegments segments={segments} checks={checks} />);

    expect(screen.getByText('Spot the hidden trap')).toBeInTheDocument();
    expect(screen.getByText('Sneak past the guards')).toBeInTheDocument();
    expect(screen.getByText('You peer into the shadows.')).toBeInTheDocument();
  });

  it('renders server playback controls and triggers callbacks', () => {
    const onPlay = vi.fn();
    const onStop = vi.fn();

    const segments: TurnSegment[] = [
      {
        kind: 'narration',
        text: 'The wind howls outside.',
        audio_urls: ['/audio/seg1.mp3'],
      },
    ];

    const { rerender } = render(
      <TurnSegments
        segments={segments}
        serverPlayback={true}
        turnAudioState="idle"
        onPlayTurn={onPlay}
        onStopTurn={onStop}
      />,
    );

    const playBtn = screen.getByRole('button', { name: /^Play turn$/ });
    expect(playBtn).toBeInTheDocument();
    fireEvent.click(playBtn);
    expect(onPlay).toHaveBeenCalled();

    const regenerateBtn = screen.getByTitle('Force regenerate speech');
    fireEvent.click(regenerateBtn);
    expect(onPlay).toHaveBeenCalledWith(undefined, true);

    // Rerender as playing
    rerender(
      <TurnSegments
        segments={segments}
        serverPlayback={true}
        turnAudioState="playing"
        onPlayTurn={onPlay}
        onStopTurn={onStop}
      />,
    );

    const stopBtn = screen.getByRole('button', { name: /^Stop$/ });
    expect(stopBtn).not.toBeDisabled();
    fireEvent.click(stopBtn);
    expect(onStop).toHaveBeenCalled();
  });

  it('displays generating and error states for server playback', () => {
    const segments: TurnSegment[] = [
      {
        kind: 'narration',
        text: 'Thunder rumbles.',
        audio_urls: ['/audio/seg1.mp3'],
      },
    ];

    const { rerender } = render(<TurnSegments segments={segments} serverPlayback={true} turnAudioState="generating" />);

    expect(screen.getByText(/Generating speech/)).toBeInTheDocument();
    expect(screen.getByText(/Rendering speech/)).toBeInTheDocument();

    rerender(
      <TurnSegments
        segments={segments}
        serverPlayback={true}
        turnAudioState="error"
        turnAudioMessage="TTS service timeout"
      />,
    );

    expect(screen.getByText(/Error: TTS service timeout/)).toBeInTheDocument();
  });
});
