import { describe, expect, it } from 'vitest';
import {
  KEN_BURNS_PAN,
  KEN_BURNS_ZOOM,
  kenBurns,
  moodTint,
  parallaxOffset,
  weatherOverlay,
} from './effects';

describe('kenBurns', () => {
  it('is the identity at the start of a beat', () => {
    expect(kenBurns(1, 0)).toEqual({ scale: 1, dx: 0, dy: 0 });
  });

  it('is bounded and deterministic', () => {
    const a = kenBurns(1, 0.5);
    expect(a.scale).toBeGreaterThan(1);
    expect(a.scale).toBeLessThan(1.1);
    expect(kenBurns(1, 0.5)).toEqual(a);
    expect(kenBurns(1, 1).scale).toBeCloseTo(1 + KEN_BURNS_ZOOM, 5);
    expect(Math.abs(kenBurns(1, 1).dx)).toBeCloseTo(KEN_BURNS_PAN, 5);
  });

  it('alternates the pan direction by seed', () => {
    expect(kenBurns(1, 1).dx).toBeGreaterThan(0);
    expect(kenBurns(2, 1).dx).toBeLessThan(0);
  });

  it('clamps progress outside [0,1]', () => {
    expect(kenBurns(1, -1)).toEqual({ scale: 1, dx: 0, dy: 0 });
    expect(kenBurns(1, 2).scale).toBeCloseTo(1 + KEN_BURNS_ZOOM, 5);
  });
});

describe('parallaxOffset', () => {
  it('moves the foreground more than the background', () => {
    expect(parallaxOffset(0, 1)).toBe(0);
    expect(parallaxOffset(1, 1)).toBeGreaterThan(parallaxOffset(0.5, 1));
  });
});

describe('moodTint', () => {
  it('tints a failure and leaves an unknown outcome untinted', () => {
    expect(moodTint('miss').opacity).toBeGreaterThan(0);
    expect(moodTint('success').opacity).toBeGreaterThan(0);
    expect(moodTint('').opacity).toBe(0);
    expect(moodTint(undefined).opacity).toBe(0);
  });

  it('is case-insensitive and trims', () => {
    expect(moodTint('  MISS ').opacity).toBe(moodTint('miss').opacity);
  });
});

describe('weatherOverlay', () => {
  it('knows rain, snow, and fog', () => {
    expect(weatherOverlay('rain')).toEqual({ kind: 'rain' });
    expect(weatherOverlay('SNOW')).toEqual({ kind: 'snow' });
    expect(weatherOverlay('fog')).toEqual({ kind: 'fog' });
  });

  it('is null for clear or unknown weather', () => {
    expect(weatherOverlay('')).toBeNull();
    expect(weatherOverlay(undefined)).toBeNull();
    expect(weatherOverlay('sunny')).toBeNull();
  });
});
