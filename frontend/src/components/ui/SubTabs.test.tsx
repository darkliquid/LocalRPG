import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { SubTabs } from './SubTabs';

const tabs = [
  { id: 'one', label: 'First' },
  { id: 'two', label: 'Second' },
];

describe('SubTabs', () => {
  it('marks the active tab and reports a change', () => {
    const onChange = vi.fn();
    render(<SubTabs tabs={tabs} active="one" onChange={onChange} />);

    expect(screen.getByRole('tab', { name: 'First' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tab', { name: 'Second' })).toHaveAttribute('aria-selected', 'false');

    fireEvent.click(screen.getByRole('tab', { name: 'Second' }));

    expect(onChange).toHaveBeenCalledWith('two');
  });

  it('names the tab list', () => {
    render(<SubTabs tabs={tabs} active="one" onChange={() => {}} ariaLabel="Providers" />);
    expect(screen.getByRole('tablist', { name: 'Providers' })).toBeInTheDocument();
  });
});
