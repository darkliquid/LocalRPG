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

  it('renders type mismatch warning and disables button when types do not match', () => {
    render(
      <ContentImportDialog
        manifest={{
          id: 'test_world',
          name: 'Test World',
          version: '1.0.0',
          type: 'world',
        }}
        expectedType="system"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />
    );
    expect(screen.getByText(/cannot be imported here/i)).toBeInTheDocument();
    const button = screen.getByRole('button', { name: /install package/i });
    expect(button).toBeDisabled();
  });

  it('enables install button when expectedType matches manifest type', () => {
    render(
      <ContentImportDialog
        manifest={{
          id: 'test_system',
          name: 'Test System',
          version: '1.0.0',
          type: 'system',
        }}
        expectedType="system"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />
    );
    expect(screen.queryByText(/cannot be imported here/i)).not.toBeInTheDocument();
    const button = screen.getByRole('button', { name: /install package/i });
    expect(button).not.toBeDisabled();
  });
});
