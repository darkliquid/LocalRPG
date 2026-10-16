import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ModelDownloadModal } from './ModelDownloadModal';
import { expectNoViolations } from '../test/a11y';

// The modal subscribes to model events on mount and returns the unsubscribe as
// its cleanup, so the mock has to answer with a function rather than undefined.
vi.mock('../api/client', () => ({
  APIClient: {
    subscribeModelEvents: vi.fn(() => () => undefined),
    downloadModel: vi.fn().mockResolvedValue(undefined),
  },
}));

describe('ModelDownloadModal', () => {
  it('is a dialog named by its heading, and takes focus', async () => {
    const { container } = render(
      <ModelDownloadModal modelId="tts" modelName="Kokoro" sizeBytes={1000} onClose={() => undefined} />,
    );
    expect(screen.getByRole('dialog')).toHaveAccessibleName('Enable Voice Narration');
    expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus();
    await expectNoViolations(container);
  });

  it('closes on Escape', async () => {
    const onClose = vi.fn();
    render(<ModelDownloadModal modelId="tts" modelName="Kokoro" sizeBytes={1000} onClose={onClose} />);
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalled();
  });
});
