import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { Example } from './Example';

describe('Example', () => {
  it('hides the fragment until opened', () => {
    render(<Example>{'checks:\n  notation: 2d6'}</Example>);

    const details = screen.getByText(/notation: 2d6/).closest('details');
    expect(details).not.toBeNull();
    expect(details?.open).toBe(false);

    fireEvent.click(screen.getByText('Example'));

    expect(details?.open).toBe(true);
  });

  it('takes a custom label', () => {
    render(<Example label="A worked profile">{'{ name: standard }'}</Example>);
    expect(screen.getByText('A worked profile')).toBeInTheDocument();
  });
});