import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { TheaterDialogue } from './TheaterDialogue';
import { TheaterTransport } from './TheaterTransport';
import { TurnSegment } from '../../types';

const speech: TurnSegment = { kind: 'speech', speaker: 'Garrick', text: 'Keep walking.' };

describe('TheaterDialogue', () => {
  it('shows the no-audio chip for a silent beat', () => {
    render(<TheaterDialogue fallback="The hall is silent." noAudio onAdvance={() => {}} />);
    expect(screen.getByText(/no audio/i)).toBeInTheDocument();
  });

  it('hides the chip when the beat has audio', () => {
    render(<TheaterDialogue fallback="A line." noAudio={false} onAdvance={() => {}} />);
    expect(screen.queryByText(/no audio/i)).toBeNull();
  });

  it('shows a caption for a spoken beat when enabled', () => {
    const { container } = render(<TheaterDialogue segment={speech} fallback="" caption onAdvance={() => {}} />);
    expect(container.querySelector('[data-caption]')).not.toBeNull();
    expect(screen.getByText(/Garrick:/)).toBeInTheDocument();
  });

  it('hides the caption when disabled', () => {
    const { container } = render(<TheaterDialogue segment={speech} fallback="" onAdvance={() => {}} />);
    expect(container.querySelector('[data-caption]')).toBeNull();
  });

  it('shows a caption for a silent spoken beat', () => {
    const { container } = render(
      <TheaterDialogue segment={speech} fallback="" caption noAudio onAdvance={() => {}} />,
    );
    expect(container.querySelector('[data-caption]')).not.toBeNull();
  });

  it('does not caption a narration beat', () => {
    const narration: TurnSegment = { kind: 'narration', text: 'The hall is quiet.' };
    const { container } = render(<TheaterDialogue segment={narration} fallback="" caption onAdvance={() => {}} />);
    expect(container.querySelector('[data-caption]')).toBeNull();
  });
});

describe('TheaterTransport captions', () => {
  const transport = (captions: boolean, onToggleCaptions?: () => void) => (
    <TheaterTransport
      progress={0}
      isPlaying={false}
      speed={1}
      audioState="idle"
      captions={captions}
      onToggleCaptions={onToggleCaptions}
      onToggle={() => {}}
      onPrev={() => {}}
      onNext={() => {}}
      onCycleSpeed={() => {}}
    />
  );

  it('offers a labelled, keyboard-reachable caption toggle', () => {
    render(transport(false, () => {}));
    const button = screen.getByRole('button', { name: /show captions/i });
    expect(button).toBeInTheDocument();
    expect(button).toHaveAttribute('aria-pressed', 'false');
  });

  it('reflects the on state', () => {
    render(transport(true, () => {}));
    const button = screen.getByRole('button', { name: /hide captions/i });
    expect(button).toHaveAttribute('aria-pressed', 'true');
  });

  it('hides the toggle when no handler is given', () => {
    const { container } = render(transport(false));
    expect(container.querySelector('[data-transport="captions"]')).toBeNull();
  });
});
