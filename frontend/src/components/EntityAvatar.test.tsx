import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { EntityAvatar, avatarColor } from './EntityAvatar';

describe('EntityAvatar', () => {
  it('renders the image when a src is set', () => {
    render(<EntityAvatar src="/portrait.png" name="Lady Evelyn" />);
    expect(screen.getByRole('img')).toHaveAttribute('src', '/portrait.png');
  });

  it('falls back to initials when the image fails', () => {
    render(<EntityAvatar src="/missing.png" name="Lady Evelyn" />);
    fireEvent.error(screen.getByRole('img'));
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
    expect(screen.getByText('LE')).toBeInTheDocument();
  });

  it('falls back to initials with no src', () => {
    render(<EntityAvatar name="Kaelen" />);
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
    expect(screen.getByText('K')).toBeInTheDocument();
  });

  it('gives a name a stable colour', () => {
    expect(avatarColor('Lady Evelyn')).toBe(avatarColor('Lady Evelyn'));
    expect(avatarColor('Kaelen')).toMatch(/^#/);
  });
});