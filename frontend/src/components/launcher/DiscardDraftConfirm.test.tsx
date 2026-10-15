import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DiscardDraftConfirm } from './DiscardDraftConfirm';

describe('DiscardDraftConfirm', () => {
  it('is a dialog named by its title, with the description inside it', () => {
    render(<DiscardDraftConfirm isOpen onCancel={() => undefined} onDiscard={() => undefined} />);
    expect(screen.getByRole('dialog')).toHaveAccessibleName('Discard unsaved world?');
    expect(screen.getByText(/every field you entered/)).toBeInTheDocument();
  });

  it('treats Escape as keeping the draft', async () => {
    const onCancel = vi.fn();
    const onDiscard = vi.fn();
    render(<DiscardDraftConfirm isOpen onCancel={onCancel} onDiscard={onDiscard} />);
    await userEvent.keyboard('{Escape}');
    expect(onCancel).toHaveBeenCalled();
    expect(onDiscard).not.toHaveBeenCalled();
  });

  it('discards when the destructive button is chosen', async () => {
    const onCancel = vi.fn();
    const onDiscard = vi.fn();
    render(<DiscardDraftConfirm isOpen onCancel={onCancel} onDiscard={onDiscard} />);
    await userEvent.click(screen.getByRole('button', { name: 'Discard' }));
    expect(onDiscard).toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
  });

  it('renders nothing while it is closed', () => {
    render(<DiscardDraftConfirm isOpen={false} onCancel={() => undefined} onDiscard={() => undefined} />);
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
