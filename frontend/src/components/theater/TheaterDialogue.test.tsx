import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { TheaterDialogue } from './TheaterDialogue';

describe('TheaterDialogue', () => {
  it('shows the no-audio chip for a silent beat', () => {
    render(<TheaterDialogue fallback="The hall is silent." noAudio onAdvance={() => {}} />);
    expect(screen.getByText(/no audio/i)).toBeInTheDocument();
  });

  it('hides the chip when the beat has audio', () => {
    render(<TheaterDialogue fallback="A line." noAudio={false} onAdvance={() => {}} />);
    expect(screen.queryByText(/no audio/i)).toBeNull();
  });
});
