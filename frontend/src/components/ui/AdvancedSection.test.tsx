import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { AdvancedSection } from './AdvancedSection';

describe('AdvancedSection', () => {
  it('hides its children until expanded', () => {
    render(
      <AdvancedSection>
        <span>secret field</span>
      </AdvancedSection>,
    );

    expect(screen.queryByText('secret field')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /advanced/i }));

    expect(screen.getByText('secret field')).toBeInTheDocument();
  });

  it('collapses again when toggled off', () => {
    render(
      <AdvancedSection>
        <span>secret field</span>
      </AdvancedSection>,
    );

    const toggle = screen.getByRole('button', { name: /advanced/i });
    fireEvent.click(toggle);
    fireEvent.click(toggle);

    expect(screen.queryByText('secret field')).not.toBeInTheDocument();
  });

  it('takes a custom label', () => {
    render(
      <AdvancedSection label="Limits" defaultOpen>
        <span>a field</span>
      </AdvancedSection>,
    );

    expect(screen.getByRole('button', { name: /limits/i })).toBeInTheDocument();
    expect(screen.getByText('a field')).toBeInTheDocument();
  });
});