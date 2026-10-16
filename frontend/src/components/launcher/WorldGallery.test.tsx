import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { WorldGallery } from './WorldGallery';
import { WorldInfo } from '../../types';

const world: WorldInfo = {
  id: 'the-reach',
  name: 'Ashen Reach',
  description: 'A dying frontier.',
  genre: 'dark fantasy',
  compatible_systems: [],
};

describe('WorldGallery', () => {
  it('is a dialog named Explore Worlds, holds focus, and closes on Escape', () => {
    const onClose = vi.fn();

    render(
      <WorldGallery
        isOpen
        worlds={[world]}
        onSelectWorld={vi.fn()}
        onCreateWorld={vi.fn()}
        onDeleteWorld={vi.fn()}
        onClose={onClose}
      />,
    );

    const dialog = screen.getByRole('dialog', { name: 'Explore Worlds' });
    expect(dialog.contains(document.activeElement)).toBe(true);

    fireEvent.keyDown(dialog, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
