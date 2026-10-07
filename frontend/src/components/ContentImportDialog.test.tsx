import { render, screen } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { ContentImportDialog } from './ContentImportDialog';

describe('ContentImportDialog', () => {
  it('import shows the manifest and a mechanics.js warning', () => {
    render(
      <ContentImportDialog
        manifest={{ id: 'x', name: 'X', version: '1', has_script: true }}
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />
    );
    expect(screen.getByText(/mechanics\.js/)).toBeInTheDocument();
  });
});
