import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { ImportProgressPanel, formatRemaining } from './ImportProgressPanel';

describe('formatRemaining', () => {
  it('stays vague when there is nothing worth saying', () => {
    expect(formatRemaining(2)).toBe('a few seconds');
  });

  it('rounds seconds, then switches to minutes', () => {
    expect(formatRemaining(37)).toBe('about 35 seconds');
    expect(formatRemaining(150)).toBe('about 3 min');
  });
});

describe('ImportProgressPanel', () => {
  it('says how much is coming before the first batch finishes', () => {
    render(<ImportProgressPanel progress={{ batch: 0, batches: 51, found: 0, total: 0 }} />);
    expect(screen.getByTestId('import-progress-summary')).toHaveTextContent(
      'Reading 51 batches of the source'
    );
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0');
  });

  it('reports what it read and what it found', () => {
    render(
      <ImportProgressPanel
        progress={{
          batch: 12,
          batches: 51,
          sources: ['people-of-emberheart/guests/quezta.md', 'culture/customs.md'],
          found: 4,
          total: 37,
          names: ['Quezta', 'The Mourning March'],
        }}
      />
    );

    expect(screen.getByTestId('import-progress-summary')).toHaveTextContent('Read 12 of 51 batches');
    expect(screen.getByText(/quezta\.md, culture\/customs\.md/)).toBeInTheDocument();
    expect(screen.getByText(/37 entities so far, 4 from this batch/)).toBeInTheDocument();
    expect(screen.getByText(/Quezta, The Mourning March/)).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '24');
  });

  it('drops the countdown on the last batch', () => {
    render(<ImportProgressPanel progress={{ batch: 51, batches: 51, found: 2, total: 60 }} />);
    expect(screen.getByTestId('import-progress-summary')).not.toHaveTextContent('left');
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '100');
  });
});
