import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { TheaterStage } from './TheaterStage';

const stage = (container: HTMLElement, selector: string) => container.querySelector(selector) as HTMLElement | null;

describe('TheaterStage effects', () => {
  it('applies a ken burns transform driven by progress', () => {
    const { container } = render(<TheaterStage backgroundURL="/art.png" progress={1} seed={1} />);
    const background = stage(container, '[data-stage="background"]');
    expect(background?.style.transform).toContain('scale(1.08');
    expect(background?.style.transform).not.toBe('');
  });

  it('is a plain still at the start of a beat', () => {
    const { container } = render(<TheaterStage backgroundURL="/art.png" progress={0} seed={1} />);
    const background = stage(container, '[data-stage="background"]');
    expect(background?.style.transform).toBe('scale(1) translate(0%, 0%)');
  });

  it('applies a mood tint for a failure', () => {
    const { container } = render(<TheaterStage backgroundURL="/art.png" outcome="miss" />);
    expect(stage(container, '[data-stage="tint"]')).not.toBeNull();
  });

  it('shows a rain overlay', () => {
    const { container } = render(<TheaterStage backgroundURL="/art.png" weather="rain" />);
    expect(stage(container, '[data-weather="rain"]')).not.toBeNull();
  });

  it('respects reduced motion', () => {
    const { container } = render(<TheaterStage backgroundURL="/art.png" progress={1} seed={1} reducedMotion />);
    const background = stage(container, '[data-stage="background"]');
    expect(background?.style.transform).toBe('');
    // The tint and the overlay are not motion, so they remain.
    const { container: tinted } = render(<TheaterStage backgroundURL="/art.png" outcome="miss" reducedMotion />);
    expect(stage(tinted, '[data-stage="tint"]')).not.toBeNull();
  });

  it('moves layers at different rates', () => {
    const { container } = render(
      <TheaterStage
        progress={1}
        seed={1}
        layers={[
          { depth: 0, url: '/back.png' },
          { depth: 1, url: '/front.png' },
        ]}
      />,
    );
    const layers = container.querySelectorAll('[data-stage="layer"]');
    expect(layers).toHaveLength(2);
    const back = (layers[0] as HTMLElement).style.transform;
    const front = (layers[1] as HTMLElement).style.transform;
    expect(back).not.toBe(front);
  });

  it('renders a plain still when there is no art, outcome, or weather', () => {
    const { container } = render(<TheaterStage />);
    expect(stage(container, '[data-stage="tint"]')).toBeNull();
    expect(stage(container, '[data-stage="weather"]')).toBeNull();
    const background = stage(container, '[data-stage="background"]');
    expect(background?.style.transform).toBe('');
  });
});
