import { describe, expect, it } from 'vitest';
import { readingDurationMs } from './pacing';

describe('readingDurationMs', () => {
  it('scales with length and floors at the minimum', () => {
    const short = readingDurationMs('Go.');
    const long = readingDurationMs('word '.repeat(60));
    expect(short).toBeGreaterThanOrEqual(900);
    expect(long).toBeGreaterThan(short);
  });

  it('scales by the words-per-minute factor', () => {
    const normal = readingDurationMs('word '.repeat(40));
    const fast = readingDurationMs('word '.repeat(40), 400);
    expect(fast).toBeLessThan(normal);
  });
});
