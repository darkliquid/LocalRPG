import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import ContentStudio from './ContentStudio';

// The editor is CodeMirror, which needs a real layout to mount; the studio's
// dialog behaviour does not depend on it, so it is replaced with a stub.
vi.mock('./editor/MarkdownEditor', () => ({ default: () => <div data-testid="editor" /> }));

describe('ContentStudio', () => {
  it('is a dialog named Content Studio and closes on Escape', () => {
    const onClose = vi.fn();

    render(<ContentStudio isOpen onClose={onClose} gameID="" />);

    const dialog = screen.getByRole('dialog', { name: 'Content Studio' });
    expect(dialog.contains(document.activeElement)).toBe(true);

    fireEvent.keyDown(dialog, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('renders nothing while closed', () => {
    render(<ContentStudio isOpen={false} onClose={vi.fn()} gameID="" />);

    expect(screen.queryByRole('dialog', { name: 'Content Studio' })).not.toBeInTheDocument();
  });
});
