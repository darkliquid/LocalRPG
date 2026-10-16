import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { VoiceCatalogModal } from './VoiceCatalogModal';
import { expectNoViolations } from '../test/a11y';

// No client mock: this surface takes its voices as props, so it renders on its
// own. These are the assertions the conversion could not carry on its own.
function renderModal(overrides: { onClose?: () => void } = {}) {
  return render(
    <VoiceCatalogModal
      isOpen
      onClose={overrides.onClose ?? (() => undefined)}
      voices={[]}
      providerKey="gemini"
      onAddProfile={() => undefined}
    />,
  );
}

describe('VoiceCatalogModal', () => {
  it('is a dialog named by its title', async () => {
    const { container } = renderModal();
    expect(screen.getByRole('dialog')).toHaveAccessibleName('Voice Catalog Browser');
    await expectNoViolations(container);
  });

  it('takes focus into the dialog', () => {
    renderModal();
    expect(screen.getByRole('dialog')).toContainElement(document.activeElement as HTMLElement);
  });

  it('closes on Escape', async () => {
    const onClose = vi.fn();
    renderModal({ onClose });
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });
});
