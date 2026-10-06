import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ActionConsole } from './ActionConsole';

describe('ActionConsole', () => {
  it('disables Roll when mechanics are off', () => {
    render(<ActionConsole onSubmit={() => {}} engagement="off" />);
    expect(screen.getByRole('button', { name: /roll/i })).toBeDisabled();
  });

  it('enables Roll when mechanics are on', () => {
    render(<ActionConsole onSubmit={() => {}} engagement="auto" />);
    expect(screen.getByRole('button', { name: /roll/i })).toBeEnabled();
  });
});
