import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { ChronicleView } from './ChronicleView';
import { CampaignProvider } from '../context/CampaignContext';
import { SettingsProvider } from '../context/SettingsContext';
import { genreCopy } from '../lib/genre';
import type { Turn } from '../types';

// ChronicleView's empty and loading lines come from the genre copy table, so a
// cyberpunk campaign does not read like a generic one.
// The playback and the campaign's state come from the provider, so the component
// renders inside one; with no campaign open the effects make no requests.
function renderChronicle(element: ReactElement) {
  return render(
    <SettingsProvider>
      <CampaignProvider>{element}</CampaignProvider>
    </SettingsProvider>,
  );
}

describe('ChronicleView genre copy', () => {
  it('shows a genre empty line when there are no turns', () => {
    renderChronicle(<ChronicleView turns={[]} onWikilinkClick={() => undefined} genre="cyberpunk" />);
    expect(screen.getByText(genreCopy('cyberpunk').empty)).toBeInTheDocument();
  });

  it('falls back to the neutral line for an unknown genre', () => {
    renderChronicle(<ChronicleView turns={[]} onWikilinkClick={() => undefined} genre="nonsense" />);
    expect(screen.getByText(genreCopy(undefined).empty)).toBeInTheDocument();
  });

  it('shows a genre loading line while a turn is in flight', () => {
    renderChronicle(
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
    renderChronicle(
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
    renderChronicle(
      <ChronicleView turns={[turn]} onWikilinkClick={() => undefined} onGenerateImage={() => undefined} />,
    );
    expect(screen.queryByText(/No illustration/)).toBeNull();
  });
});

describe('ChronicleView continuity findings', () => {
  const turnWithFinding = (): Turn => ({
    turn_number: 3,
    input_text: '',
    mode: 'do',
    prose: 'The guard steps aside.',
    continuity_notes: ['Mira was introduced without a note.'],
  });

  it('offers to correct or dismiss each finding', () => {
    const onCorrect = vi.fn();
    const onAddress = vi.fn();

    renderChronicle(
      <ChronicleView
        turns={[turnWithFinding()]}
        onWikilinkClick={() => undefined}
        onCorrect={onCorrect}
        onAddress={onAddress}
      />,
    );

    expect(screen.getByText('Mira was introduced without a note.')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Correct' }));
    expect(onCorrect).toHaveBeenCalledWith('Mira was introduced without a note.', 3);

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(onAddress).toHaveBeenCalledWith(3);
  });

  it('marks a finding that has been dealt with, with nothing left to press', () => {
    renderChronicle(
      <ChronicleView
        turns={[turnWithFinding()]}
        onWikilinkClick={() => undefined}
        addressedTurns={new Set([3])}
        onAddress={() => undefined}
      />,
    );

    expect(screen.getByText('Addressed')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Dismiss' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Correct' })).not.toBeInTheDocument();
  });
});
