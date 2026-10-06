import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SegmentAudioControls } from './SegmentAudioControls';

const noop = () => {};

describe('SegmentAudioControls', () => {
  it('disables play while a clip is generating', () => {
    render(<SegmentAudioControls state="generating" onPlay={noop} onStop={noop} onRegenerate={noop} />);
    expect(screen.getByLabelText('Play this line')).toBeDisabled();
    expect(screen.getByLabelText('Regenerate this line')).toBeDisabled();
  });

  it('plays when idle', async () => {
    const onPlay = vi.fn();
    render(<SegmentAudioControls state="idle" onPlay={onPlay} onStop={noop} onRegenerate={noop} />);
    await userEvent.click(screen.getByLabelText('Play this line'));
    expect(onPlay).toHaveBeenCalledOnce();
  });

  it('names a grouped control after the group', () => {
    render(<SegmentAudioControls state="idle" grouped onPlay={noop} onStop={noop} onRegenerate={noop} />);
    expect(screen.getByLabelText('Play this group')).toBeInTheDocument();
  });

  it('shows the error message when a clip fails', () => {
    render(<SegmentAudioControls state="error" message="boom" onPlay={noop} onStop={noop} onRegenerate={noop} />);
    expect(screen.getByText('Error: boom')).toBeInTheDocument();
  });
});
