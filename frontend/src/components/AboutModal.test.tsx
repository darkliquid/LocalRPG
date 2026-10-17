import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { AboutModal } from './AboutModal';
import * as projectLib from '../lib/project';

vi.mock('../lib/project', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/project')>();
  return {
    ...actual,
    openExternal: vi.fn(),
  };
});

describe('AboutModal', () => {
  it('does not render dialog when isOpen is false', () => {
    render(<AboutModal isOpen={false} onClose={vi.fn()} onOpenDocs={vi.fn()} />);

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('renders dialog with title, tagline, highlights and version', () => {
    render(<AboutModal isOpen={true} onClose={vi.fn()} onOpenDocs={vi.fn()} version="0.5.0" />);

    expect(screen.getByRole('dialog', { name: 'About LocalRPG' })).toBeInTheDocument();
    expect(screen.getByText('LocalRPG')).toBeInTheDocument();
    expect(
      screen.getByText('Runs entirely on your machine, with no account and no cloud round-trip.'),
    ).toBeInTheDocument();
    expect(screen.getByText('Version 0.5.0')).toBeInTheDocument();
  });

  it('renders development build when version is omitted', () => {
    render(<AboutModal isOpen={true} onClose={vi.fn()} onOpenDocs={vi.fn()} />);

    expect(screen.getByText('Development build')).toBeInTheDocument();
  });

  it('navigates to documentation and closes modal when Documentation button is clicked', () => {
    const onClose = vi.fn();
    const onOpenDocs = vi.fn();
    render(<AboutModal isOpen={true} onClose={onClose} onOpenDocs={onOpenDocs} />);

    fireEvent.click(screen.getByRole('button', { name: /Documentation/i }));

    expect(onClose).toHaveBeenCalled();
    expect(onOpenDocs).toHaveBeenCalled();
  });

  it('opens repository and issues URLs via openExternal', () => {
    render(<AboutModal isOpen={true} onClose={vi.fn()} onOpenDocs={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Project Repository' }));
    expect(projectLib.openExternal).toHaveBeenCalledWith(projectLib.PROJECT_REPO_URL);

    fireEvent.click(screen.getByRole('button', { name: 'Report an Issue' }));
    expect(projectLib.openExternal).toHaveBeenCalledWith(projectLib.PROJECT_ISSUES_URL);
  });

  it('closes when close button is clicked', () => {
    const onClose = vi.fn();
    render(<AboutModal isOpen={true} onClose={onClose} onOpenDocs={vi.fn()} />);

    const closeButtons = screen.getAllByRole('button', { name: 'Close' });
    expect(closeButtons.length).toBeGreaterThan(0);
    fireEvent.click(closeButtons[0]);
    expect(onClose).toHaveBeenCalled();
  });
});
