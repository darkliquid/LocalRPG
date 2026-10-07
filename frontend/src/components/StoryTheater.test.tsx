import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { StoryTheater } from './StoryTheater';
import { Turn, TurnSegment } from '../types';

const seg = (text: string, clip_group: string, audio: string): TurnSegment => ({
  kind: 'speech',
  text,
  clip_group,
  audio_urls: [audio],
});

const turns: Turn[] = [
  {
    turn_number: 1,
    input_text: '',
    mode: 'do',
    prose: '',
    segments: [seg('one', 'g1', '/a'), seg('two', 'g1', '/a'), seg('three', 'g2', '/b')],
  },
];

describe('StoryTheater grouping', () => {
  it('does not restart audio when stepping within a group', () => {
    const onPlayAudio = vi.fn();
    render(
      <StoryTheater
        turns={turns}
        isOpen
        onClose={() => {}}
        serverPlayback
        onPlayAudio={onPlayAudio}
        segmentAudioStatus={{}}
      />,
    );

    // The first group's clip is requested once on open.
    expect(onPlayAudio).toHaveBeenCalledWith(1, 0);
    const afterOpen = onPlayAudio.mock.calls.length;

    // Stepping to the second line of the same group shows it but leaves the audio
    // alone.
    fireEvent.click(screen.getByTitle('Next line'));
    expect(screen.getByText(/two/)).toBeInTheDocument();
    expect(onPlayAudio.mock.calls.length).toBe(afterOpen);

    // Stepping into the next group requests its clip.
    fireEvent.click(screen.getByTitle('Next line'));
    expect(screen.getByText(/three/)).toBeInTheDocument();
    expect(onPlayAudio).toHaveBeenLastCalledWith(1, 2);
  });

  it('steps back to the previous line', () => {
    render(
      <StoryTheater turns={turns} isOpen onClose={() => {}} serverPlayback segmentAudioStatus={{}} />,
    );
    fireEvent.click(screen.getByTitle('Next line'));
    fireEvent.click(screen.getByTitle('Previous line'));
    expect(screen.getByText(/one/)).toBeInTheDocument();
  });
});
