import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ImageLightbox } from './ImageLightbox';
import { expectNoViolations } from '../test/a11y';

describe('ImageLightbox', () => {
  it('is a dialog named by the image it shows, and takes focus', async () => {
    const { container } = render(
      <ImageLightbox src="/api/game/c/character/x/portrait" alt="Lady Evelyn" onClose={() => undefined} />,
    );
    expect(screen.getByRole('dialog')).toHaveAccessibleName('Lady Evelyn');
    expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus();
    await expectNoViolations(container);
  });

  it('closes on Escape', async () => {
    const onClose = vi.fn();
    render(<ImageLightbox src="/x.png" alt="Lady Evelyn" onClose={onClose} />);
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });

  it('closes on a backdrop click but not on the image', async () => {
    const onClose = vi.fn();
    render(<ImageLightbox src="/x.png" alt="Lady Evelyn" onClose={onClose} />);
    await userEvent.click(screen.getByRole('dialog'));
    expect(onClose).not.toHaveBeenCalled();
    await userEvent.click(screen.getByAltText('Lady Evelyn').closest('div')!.parentElement!);
    expect(onClose).toHaveBeenCalled();
  });

  it('returns focus to what opened it', () => {
    const opener = document.createElement('button');
    document.body.appendChild(opener);
    opener.focus();
    const { rerender } = render(<ImageLightbox src="/x.png" alt="Lady Evelyn" onClose={() => undefined} />);
    expect(opener).not.toHaveFocus();
    rerender(<ImageLightbox src="/x.png" alt="Lady Evelyn" onClose={() => undefined} isOpen={false} />);
    expect(opener).toHaveFocus();
  });
});
