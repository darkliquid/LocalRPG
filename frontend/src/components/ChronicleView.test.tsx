import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ChronicleView } from './ChronicleView';
import { genreCopy } from '../lib/genre';
import type { Turn } from '../types';

// ChronicleView's empty and loading lines come from the genre copy table, so a
// cyberpunk campaign does not read like a generic one.
describe('ChronicleView genre copy', () => {
  it('shows a genre empty line when there are no turns', () => {
    render(<ChronicleView turns={[]} onWikilinkClick={() => undefined} genre="cyberpunk" />);
    expect(screen.getByText(genreCopy('cyberpunk').empty)).toBeInTheDocument();
  });

  it('falls back to the neutral line for an unknown genre', () => {
    render(<ChronicleView turns={[]} onWikilinkClick={() => undefined} genre="nonsense" />);
    expect(screen.getByText(genreCopy(undefined).empty)).toBeInTheDocument();
  });

  it('shows a genre loading line while a turn is in flight', () => {
    render(
      <ChronicleView
        turns={[]}
        onWikilinkClick={() => undefined}
        genre="fantasy"
        turnInFlight
        pendingAction={{ mode: 'Do', text: 'I look around' }}
      />,
    );
    expect(screen.getByText(genreCopy('fantasy').loading)).toBeInTheDocument();
  });
});

// A turn with no illustration should account for itself when the server said why
// it withheld one, so the absence reads as a policy rather than a failure.
describe('ChronicleView withheld illustrations', () => {
  const turn: Turn = { turn_number: 1, input_text: 'look around', mode: 'Do', prose: 'The hall is dark.' };

  it('explains a beat the server withheld an illustration from', () => {
    render(
      <ChronicleView
        turns={[turn]}
        onWikilinkClick={() => undefined}
        onGenerateImage={() => undefined}
        sceneImagePending={{ 1: 'budget_exhausted' }}
      />,
    );
    expect(screen.getByText('No illustration: this campaign has spent its image budget.')).toBeInTheDocument();
  });

  it('says nothing when the server did not explain the absence', () => {
    render(<ChronicleView turns={[turn]} onWikilinkClick={() => undefined} onGenerateImage={() => undefined} />);
    expect(screen.queryByText(/No illustration/)).toBeNull();
  });
});
