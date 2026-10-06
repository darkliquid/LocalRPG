import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MarkdownProse } from './MarkdownProse';

describe('MarkdownProse', () => {
  it('renders a wikilink as a clickable entity', async () => {
    const onEntityClick = vi.fn();
    render(<MarkdownProse text="The quay is quiet by [[the-quay|The Quay]]." onEntityClick={onEntityClick} />);
    await userEvent.click(screen.getByText('The Quay'));
    expect(onEntityClick).toHaveBeenCalledWith('the-quay');
  });

  it('renders a wikilink as plain text without a click handler', () => {
    render(<MarkdownProse text="See [[the-quay|The Quay]]." />);
    expect(screen.getByText('The Quay')).toBeInTheDocument();
  });

  it('renders emphasis', () => {
    render(<MarkdownProse text="A **grim** guard." />);
    expect(screen.getByText('grim').tagName).toBe('STRONG');
  });

  it('renders a performance direction as a tag', () => {
    render(<MarkdownProse text="A [whispers] word." />);
    expect(screen.getByText('whispers')).toBeInTheDocument();
  });

  it('hides a performance direction in hidden mode', () => {
    render(<MarkdownProse text="A [whispers] word." displayMode="hidden" />);
    expect(screen.queryByText('whispers')).toBeNull();
    expect(screen.getByText('A word.')).toBeInTheDocument();
  });
});
