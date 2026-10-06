import { describe, expect, it } from 'vitest';
import { beatGapMs } from './StoryTheater';

describe('beatGapMs', () => {
  it('is small at normal speed', () => {
    expect(beatGapMs(1)).toBeLessThanOrEqual(120);
  });

  it('tightens at faster speeds and widens at slower ones', () => {
    expect(beatGapMs(2)).toBeLessThan(beatGapMs(1));
    expect(beatGapMs(0.5)).toBeGreaterThan(beatGapMs(1));
  });

  it('never falls below the floor', () => {
    expect(beatGapMs(1000)).toBeGreaterThanOrEqual(40);
  });
});
