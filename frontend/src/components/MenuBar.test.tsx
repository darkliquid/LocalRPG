import { render, screen, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { fireEvent } from '@testing-library/react';
import { MenuBar } from './MenuBar';

// MenuBar keeps itself out of the way until the player taps Alt, which is the
// gesture a native Windows menu bar answers to. The three behaviours worth pinning
// are the tap, the chord that must not count, and the two ways it closes.

function renderMenuBar() {
  return render(<MenuBar onOpenDocs={vi.fn()} onOpenAbout={vi.fn()} onOpenContentStudio={vi.fn()} />);
}

// Alt is held and released: the bar is rendered only once the tap is complete.
function tapAlt() {
  fireEvent.keyDown(window, { key: 'Alt' });
  fireEvent.keyUp(window, { key: 'Alt' });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('MenuBar keyboard', () => {
  it('stays out of the way until Alt is tapped', () => {
    renderMenuBar();

    expect(screen.queryByRole('menubar')).not.toBeInTheDocument();

    tapAlt();

    expect(screen.getByRole('menubar', { name: 'Application menu' })).toBeInTheDocument();
  });

  it('hides again on a second tap', async () => {
    renderMenuBar();

    tapAlt();
    expect(screen.getByRole('menubar')).toBeInTheDocument();

    tapAlt();

    // Hiding is an animated unmount, so the bar leaves when the transition ends.
    await waitFor(() => expect(screen.queryByRole('menubar')).not.toBeInTheDocument());
  });

  it('does not open on an Alt chord', () => {
    renderMenuBar();

    // Alt+Tab is a window switch, not a request for the menu.
    fireEvent.keyDown(window, { key: 'Alt' });
    fireEvent.keyDown(window, { key: 'Tab' });
    fireEvent.keyUp(window, { key: 'Tab' });
    fireEvent.keyUp(window, { key: 'Alt' });

    expect(screen.queryByRole('menubar')).not.toBeInTheDocument();
  });

  it('closes a revealed menu on Escape', async () => {
    renderMenuBar();
    tapAlt();
    expect(screen.getByRole('menubar')).toBeInTheDocument();

    fireEvent.keyDown(window, { key: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('menubar')).not.toBeInTheDocument());
  });

  it('closes when the player clicks somewhere else, the way a native menu does', async () => {
    renderMenuBar();
    tapAlt();
    expect(screen.getByRole('menubar')).toBeInTheDocument();

    fireEvent.mouseDown(document.body);

    await waitFor(() => expect(screen.queryByRole('menubar')).not.toBeInTheDocument());
  });

  it('opens a group and moves between them once a menu is open', () => {
    renderMenuBar();
    tapAlt();

    fireEvent.click(screen.getByRole('menuitem', { name: 'View' }));
    expect(screen.getByRole('menu')).toBeInTheDocument();

    // Sliding across the bar switches groups, which is how a native menu behaves.
    fireEvent.mouseEnter(screen.getByRole('menuitem', { name: 'Help' }));
    expect(screen.getByRole('menuitem', { name: 'Help' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('menuitem', { name: 'View' })).toHaveAttribute('aria-expanded', 'false');
  });
});
