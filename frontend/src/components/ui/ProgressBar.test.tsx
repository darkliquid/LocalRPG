import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ProgressBar } from './ProgressBar';
import { expectNoViolations } from '../../test/a11y';

describe('ProgressBar', () => {
  it('states its value and its name', async () => {
    const { container } = render(<ProgressBar value={40} label="Download progress" />);

    const bar = screen.getByRole('progressbar', { name: 'Download progress' });
    expect(bar).toHaveAttribute('aria-valuenow', '40');
    expect(bar).toHaveAttribute('aria-valuemin', '0');
    expect(bar).toHaveAttribute('aria-valuemax', '100');
    expect(bar).toHaveStyle({ width: '40%' });

    await expectNoViolations(container);
  });

  it('clamps a value above the maximum, in the number and the width alike', () => {
    render(<ProgressBar value={140} label="Clips stored" />);

    const bar = screen.getByRole('progressbar', { name: 'Clips stored' });
    expect(bar).toHaveAttribute('aria-valuenow', '100');
    expect(bar).toHaveStyle({ width: '100%' });
  });

  it('clamps a value below zero', () => {
    render(<ProgressBar value={-20} label="Clips stored" />);

    const bar = screen.getByRole('progressbar', { name: 'Clips stored' });
    expect(bar).toHaveAttribute('aria-valuenow', '0');
    expect(bar).toHaveStyle({ width: '0%' });
  });

  it('reports a maximum other than a hundred', () => {
    render(<ProgressBar value={3} max={6} label="Arc progress" />);

    const bar = screen.getByRole('progressbar', { name: 'Arc progress' });
    expect(bar).toHaveAttribute('aria-valuenow', '3');
    expect(bar).toHaveAttribute('aria-valuemax', '6');
    expect(bar).toHaveStyle({ width: '50%' });
  });

  it('will not divide by a maximum of zero', () => {
    render(<ProgressBar value={5} max={0} label="Arc progress" />);

    const bar = screen.getByRole('progressbar', { name: 'Arc progress' });
    expect(bar).toHaveAttribute('aria-valuenow', '0');
    expect(bar).toHaveStyle({ width: '0%' });
  });
});
