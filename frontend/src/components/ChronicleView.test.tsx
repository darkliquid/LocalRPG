import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ChronicleView } from './ChronicleView';
import { genreCopy } from '../lib/genre';

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
