import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MechanicsStrip } from './MechanicsStrip';

describe('MechanicsStrip image budget', () => {
  it('shows the images left when a budget is set', () => {
    render(<MechanicsStrip turn={{ engagement: 'ask', checks: [] }} budget={{ max_images: 200, used_images: 195 }} />);
    expect(screen.getByText('images: 5 left')).toBeInTheDocument();
  });

  it('says so when the budget is spent', () => {
    render(<MechanicsStrip turn={{ engagement: 'ask', checks: [] }} budget={{ max_images: 10, used_images: 10 }} />);
    expect(screen.getByText('images: budget spent')).toBeInTheDocument();
  });

  it('renders nothing for an unlimited budget with mechanics off', () => {
    const { container } = render(<MechanicsStrip turn={{ engagement: 'off', checks: [] }} budget={{ max_images: 0 }} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('still shows the mechanics line alone', () => {
    render(<MechanicsStrip turn={{ engagement: 'auto', checks: [] }} />);
    expect(screen.getByText(/Mechanics: auto/)).toBeInTheDocument();
    expect(screen.queryByText(/images:/)).toBeNull();
  });
});
