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

  it('renders verified provenance chip when trust is verified', () => {
    render(
      <ContentImportDialog
        manifest={{
          id: 'x',
          name: 'X',
          version: '1',
          trust: { state: 'verified', publisher: 'Alice' },
        }}
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />
    );
    expect(screen.getByText(/Verified by Alice/)).toBeInTheDocument();
  });

  it('renders unknown key provenance chip when trust is unknown_key', () => {
    render(
      <ContentImportDialog
        manifest={{
          id: 'x',
          name: 'X',
          version: '1',
          trust: { state: 'unknown_key', publisher: 'Bob' },
        }}
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />
    );
    expect(screen.getByText(/Signed by untrusted key/)).toBeInTheDocument();
  });
});
