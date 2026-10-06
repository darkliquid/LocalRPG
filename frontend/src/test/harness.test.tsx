import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';

describe('the test harness', () => {
  it('runs in a DOM with the jest-dom matchers installed', () => {
    document.body.innerHTML = '<p>ready</p>';
    expect(screen.getByText('ready')).toBeInTheDocument();
  });

  it('renders a component', () => {
    render(<span>hello</span>);
    expect(screen.getByText('hello')).toBeVisible();
  });
});
