import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { ProloguePanel } from './ProloguePanel';

describe('ProloguePanel', () => {
  it('renders game name and default message when playerName is omitted', () => {
    render(<ProloguePanel gameName="Lost Mine of Phandelver" onBeginStory={vi.fn()} onBeginWithAction={vi.fn()} />);

    expect(screen.getByText('Lost Mine of Phandelver')).toBeInTheDocument();
    expect(screen.getByText('Let the Game Master set the opening scene, then take it from there.')).toBeInTheDocument();
  });

  it('renders player name when provided', () => {
    render(
      <ProloguePanel
        gameName="Lost Mine of Phandelver"
        playerName="Aelar"
        onBeginStory={vi.fn()}
        onBeginWithAction={vi.fn()}
      />,
    );

    expect(screen.getByText(/Aelar has not stepped into the story yet\./i)).toBeInTheDocument();
  });

  it('allows editing opening prompt and triggers onBeginStory with custom prompt', () => {
    const onBeginStory = vi.fn();
    render(
      <ProloguePanel
        gameName="Lost Mine of Phandelver"
        initialPrompt="Start in a tavern"
        onBeginStory={onBeginStory}
        onBeginWithAction={vi.fn()}
      />,
    );

    const textarea = screen.getByPlaceholderText(/Where should the story open\?/i);
    expect(textarea).toHaveValue('Start in a tavern');

    fireEvent.change(textarea, { target: { value: 'Begin near the forest entrance at dawn' } });
    expect(textarea).toHaveValue('Begin near the forest entrance at dawn');

    fireEvent.click(screen.getByRole('button', { name: /Begin the story/i }));
    expect(onBeginStory).toHaveBeenCalledWith('Begin near the forest entrance at dawn');
  });

  it('triggers onBeginWithAction when clicking first step button', () => {
    const onBeginWithAction = vi.fn();
    render(
      <ProloguePanel gameName="Lost Mine of Phandelver" onBeginStory={vi.fn()} onBeginWithAction={onBeginWithAction} />,
    );

    fireEvent.click(screen.getByRole('button', { name: /I'll take the first step/i }));
    expect(onBeginWithAction).toHaveBeenCalledTimes(1);
  });

  it('disables controls and displays busy status when busy is true', () => {
    render(
      <ProloguePanel
        gameName="Lost Mine of Phandelver"
        busy={true}
        onBeginStory={vi.fn()}
        onBeginWithAction={vi.fn()}
      />,
    );

    expect(screen.getByPlaceholderText(/Where should the story open\?/i)).toBeDisabled();
    expect(screen.getByRole('button', { name: /Setting the scene\.\.\./i })).toBeDisabled();
    expect(screen.getByRole('button', { name: /I'll take the first step/i })).toBeDisabled();
  });
});
