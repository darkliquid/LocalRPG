import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { GraphDrawer } from './GraphDrawer';
import type { GraphData } from '../types';

describe('GraphDrawer', () => {
  beforeEach(() => {
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
      clearRect: vi.fn(),
      beginPath: vi.fn(),
      moveTo: vi.fn(),
      lineTo: vi.fn(),
      stroke: vi.fn(),
      arc: vi.fn(),
      fill: vi.fn(),
      fillText: vi.fn(),
    } as never);
  });
  it('renders empty message when data is undefined or nodes are empty', () => {
    const { rerender } = render(<GraphDrawer onSelectNode={vi.fn()} />);

    expect(screen.getByText('No entities yet. Play a turn and the world will grow.')).toBeInTheDocument();

    rerender(<GraphDrawer data={{ nodes: [], links: [] }} onSelectNode={vi.fn()} />);
    expect(screen.getByText('No entities yet. Play a turn and the world will grow.')).toBeInTheDocument();
  });

  it('renders node and link counts, canvas, and legend when data is present', () => {
    const data: GraphData = {
      nodes: [
        { id: 'hero', label: 'Hero', type: 'character' },
        { id: 'tavern', label: 'Tavern', type: 'location' },
      ],
      links: [{ source: 'hero', target: 'tavern' }],
    };

    render(<GraphDrawer data={data} onSelectNode={vi.fn()} />);

    expect(screen.getByText('2 nodes · 1 links')).toBeInTheDocument();
    expect(document.querySelector('canvas')).toBeInTheDocument();
    expect(screen.getByText(/Gold: Character/)).toBeInTheDocument();
    expect(screen.getByText(/Purple: Location/)).toBeInTheDocument();
  });

  it('handles canvas click and calls onSelectNode for closest node', () => {
    const onSelect = vi.fn();
    const data: GraphData = {
      nodes: [{ id: 'hero', label: 'Hero', type: 'character' }],
      links: [],
    };

    render(<GraphDrawer data={data} onSelectNode={onSelect} />);

    const canvas = document.querySelector('canvas')!;

    // Mock getBoundingClientRect
    vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      width: 360,
      height: 360,
      right: 360,
      bottom: 360,
      x: 0,
      y: 0,
      toJSON: () => {},
    });

    // The single node is placed at:
    // centerX = 180, centerY = 180
    // radius = 180 - 46 = 134
    // angle = (0 / 1) * 2 * PI - PI/2 = -PI/2
    // pos.x = 180 + 134 * cos(-PI/2) = 180
    // pos.y = 180 + 134 * sin(-PI/2) = 180 - 134 = 46
    fireEvent.click(canvas, { clientX: 180, clientY: 46 });

    expect(onSelect).toHaveBeenCalledWith('hero');
  });
});
