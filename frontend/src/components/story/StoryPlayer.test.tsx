import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { StoryPlayer } from './StoryPlayer';
import { Story } from './types';

const story: Story = {
  game_name: 'Campaign One',
  chapters: [
    { title: 'Alden Tavern', start: 0 },
    { title: 'Aldon Harbour', start: 2 },
  ],
  scenes: [
    { location: 'Alden Tavern', beats: [{ kind: 'narration', text: 'Warm light.', duration: 2 }] },
    { location: 'Aldon Harbour', beats: [{ kind: 'narration', text: 'Salt air.', duration: 2 }] },
  ],
};

describe('StoryPlayer chapters', () => {
  it('lists chapters and seeks on click', () => {
    render(<StoryPlayer story={story} />);

    // The story opens on the first scene.
    expect(screen.getByText(/Warm light/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Aldon Harbour' }));
    expect(screen.getByText(/Salt air/)).toBeInTheDocument();
  });

  it('marks the current chapter', () => {
    render(<StoryPlayer story={story} />);
    const first = screen.getByRole('button', { name: 'Alden Tavern' });
    expect(first).toHaveAttribute('aria-current', 'true');
  });
});
