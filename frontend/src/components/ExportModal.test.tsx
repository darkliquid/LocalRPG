import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ExportModal } from './ExportModal';

// This surface does touch the client on mount — it asks what export formats are
// available and subscribes to the job's event stream — so both are mocked. The
// capabilities answer is an empty object rather than undefined because the
// component reads a field off it.
vi.mock('../api/client', () => ({
  APIClient: {
    exportCapabilities: vi.fn().mockResolvedValue({}),
    subscribeExportEvents: vi.fn(() => () => undefined),
    startExport: vi.fn().mockResolvedValue({ job_id: 'job' }),
    cancelExport: vi.fn().mockResolvedValue(undefined),
  },
}));

function renderModal(overrides: { onClose?: () => void } = {}) {
  return render(<ExportModal isOpen gameID="campaign-01" onClose={overrides.onClose ?? (() => undefined)} />);
}

describe('ExportModal', () => {
  it('is a dialog named by its title', () => {
    renderModal();
    expect(screen.getByRole('dialog')).toHaveAccessibleName('Export Story Replay');
  });

  it('takes focus into the dialog', () => {
    renderModal();
    expect(screen.getByRole('dialog')).toContainElement(document.activeElement as HTMLElement);
  });

  it('closes on Escape', async () => {
    const onClose = vi.fn();
    renderModal({ onClose });
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });
});
