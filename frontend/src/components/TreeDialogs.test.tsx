import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DeleteFolderDialog, DeleteEntityDialog } from './TreeDialogs';

// Every dialog in this file is the same shell, so these two stand for the four:
// the shell is what carries the semantics, the focus and Escape.
describe('the tree dialogs', () => {
  it('announces the deletion, its folder and its blast radius', () => {
    render(
      <DeleteFolderDialog
        isOpen
        path="places/harbour"
        noteCount={3}
        onCancel={() => undefined}
        onSubmit={() => undefined}
      />,
    );
    expect(screen.getByRole('dialog')).toHaveAccessibleName('Delete folder');
    expect(screen.getByText('places/harbour')).toBeInTheDocument();
    expect(screen.getByText(/3 notes in it will be deleted too/)).toBeInTheDocument();
  });

  it('cancels on Escape rather than deleting', async () => {
    const onCancel = vi.fn();
    const onSubmit = vi.fn();
    render(<DeleteEntityDialog isOpen name="Lady Evelyn" onCancel={onCancel} onSubmit={onSubmit} />);
    await userEvent.keyboard('{Escape}');
    expect(onCancel).toHaveBeenCalled();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('submits from the destructive button', async () => {
    const onSubmit = vi.fn();
    render(<DeleteEntityDialog isOpen name="Lady Evelyn" onCancel={() => undefined} onSubmit={onSubmit} />);
    await userEvent.click(screen.getByRole('button', { name: 'Delete note' }));
    expect(onSubmit).toHaveBeenCalled();
  });
});
