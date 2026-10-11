import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { ScriptReference, GLOBALS, HOOKS } from './ScriptReference';

describe('ScriptReference', () => {
  it('documents an example for every entry', () => {
    for (const entry of [...GLOBALS, ...HOOKS]) {
      expect(entry.example.trim(), `${entry.sig} needs an example`).not.toBe('');
    }
  });

  it('is collapsed until opened', () => {
    render(<ScriptReference />);
    expect(screen.queryByText('roll(notation)')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /sandbox api reference/i }));

    expect(screen.getByText('roll(notation)')).toBeInTheDocument();
    expect(screen.getByText('onAction(name, fn)')).toBeInTheDocument();
    expect(screen.getByText('grantXP(amount)')).toBeInTheDocument();
  });

  it('collapses again when toggled off', () => {
    render(<ScriptReference />);
    const toggle = screen.getByRole('button', { name: /sandbox api reference/i });
    fireEvent.click(toggle);
    fireEvent.click(toggle);
    expect(screen.queryByText('roll(notation)')).not.toBeInTheDocument();
  });
});
