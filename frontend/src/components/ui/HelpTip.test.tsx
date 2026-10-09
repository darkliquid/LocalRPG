import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { HelpTip } from './HelpTip';

describe('HelpTip', () => {
  it('reveals the explanation when opened', () => {
    render(<HelpTip label="Stats">A stat is a named value.</HelpTip>);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /help: stats/i }));
    expect(screen.getByRole('tooltip')).toHaveTextContent('A stat is a named value.');
  });

  it('closes again when opened twice', () => {
    render(<HelpTip label="Stats">Text</HelpTip>);
    const button = screen.getByRole('button', { name: /help: stats/i });
    fireEvent.click(button);
    fireEvent.click(button);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });
});
