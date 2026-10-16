import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Modal } from './Modal';
import { expectNoViolations } from '../test/a11y';

describe('Modal', () => {
  it('has no accessibility violations', async () => {
    const { container } = render(
      <Modal open onClose={() => undefined} title="Campaign settings">
        <label htmlFor="name">Name</label>
        <input id="name" />
        <button type="button">Save</button>
      </Modal>,
    );

    await expectNoViolations(container);
  });

  it('announces itself as a dialog and labels it with its title', () => {
    render(
      <Modal open onClose={() => undefined} title="Campaign settings">
        <button type="button">Save</button>
      </Modal>,
    );
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAccessibleName('Campaign settings');
  });

  it('moves focus to the first control when it opens', async () => {
    render(
      <Modal open onClose={() => undefined} title="Campaign settings">
        <button type="button">Save</button>
      </Modal>,
    );
    expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus();
  });

  it('closes on Escape', async () => {
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="Campaign settings">
        <button type="button">Save</button>
      </Modal>,
    );
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });

  it('keeps Tab inside the dialog', async () => {
    render(
      <Modal open onClose={() => undefined} title="Campaign settings">
        <button type="button">Save</button>
      </Modal>,
    );
    const close = screen.getByRole('button', { name: 'Close' });
    const save = screen.getByRole('button', { name: 'Save' });
    save.focus();
    await userEvent.tab();
    expect(close).toHaveFocus();
  });

  it('refuses Escape and the backdrop when it is not dismissable', async () => {
    const onClose = vi.fn();
    render(
      <Modal open onClose={onClose} title="Delete the campaign" dismissable={false}>
        <button type="button">Delete</button>
      </Modal>,
    );
    await userEvent.keyboard('{Escape}');
    await userEvent.click(screen.getByTestId('modal-backdrop'));
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: 'Close' })).toBeNull();
  });

  it('returns focus to what opened it', () => {
    const opener = document.createElement('button');
    document.body.appendChild(opener);
    opener.focus();
    const { rerender } = render(
      <Modal open onClose={() => undefined} title="Campaign settings">
        <button type="button">Save</button>
      </Modal>,
    );
    expect(opener).not.toHaveFocus();
    rerender(
      <Modal open={false} onClose={() => undefined} title="Campaign settings">
        <button type="button">Save</button>
      </Modal>,
    );
    expect(opener).toHaveFocus();
  });
});
