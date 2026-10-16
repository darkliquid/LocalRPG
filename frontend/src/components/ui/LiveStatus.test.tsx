import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { LiveStatus } from './LiveStatus';
import { expectNoViolations } from '../../test/a11y';

describe('LiveStatus', () => {
  it('is a status region in the tree, empty when there is nothing to say', async () => {
    const { container } = render(<LiveStatus message="" />);

    expect(screen.getByRole('status')).toHaveTextContent('');

    await expectNoViolations(container);
  });

  it('carries the sentence when it is given one', () => {
    render(<LiveStatus message="Turn 12 ready" />);

    expect(screen.getByRole('status')).toHaveTextContent('Turn 12 ready');
  });

  it('stays in the tree when the sentence is cleared', () => {
    const { rerender } = render(<LiveStatus message="Narrating the turn…" />);

    rerender(<LiveStatus message="" />);

    // A region that unmounts and remounts with its text is not announced, so the
    // element has to survive the empty message.
    expect(screen.getByRole('status')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('');
  });
});
