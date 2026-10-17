import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import RowMenu from './RowMenu';

// RowMenu is the per-row action menu, portalled and fixed-positioned so the tree's
// own scroll container cannot clip it. Escape closing it and a click elsewhere
// dismissing it are the two behaviours it implements and neither had a test.

function renderMenu(onSelect = vi.fn()) {
  render(<RowMenu items={[{ label: 'Delete', icon: null, onSelect }]} />);
  return onSelect;
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('RowMenu', () => {
  it('opens the menu from its trigger, which is named', () => {
    renderMenu();

    const trigger = screen.getByRole('button', { name: 'More actions' });
    expect(trigger).toHaveAttribute('aria-expanded', 'false');

    fireEvent.click(trigger);

    expect(trigger).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('menu')).toBeInTheDocument();
    expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeInTheDocument();
  });

  it('closes on Escape', () => {
    renderMenu();
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }));

    fireEvent.keyDown(window, { key: 'Escape' });

    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  it('closes when the player clicks somewhere else', async () => {
    renderMenu();
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }));

    fireEvent.click(document.body);

    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument());
  });

  it('runs the item and closes the menu', () => {
    const onSelect = renderMenu();
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }));

    fireEvent.click(screen.getByRole('menuitem', { name: 'Delete' }));

    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });
});
