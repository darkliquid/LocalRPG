import { describe, expect, it } from 'vitest';
import { diceRange, isOutOfRange, parseDiceEntry } from './diceRange';

describe('diceRange', () => {
  it('ranges a simple notation', () => {
    expect(diceRange('2d6')).toEqual({ min: 2, max: 12 });
    expect(diceRange('5d10')).toEqual({ min: 5, max: 50 });
  });

  it('returns null for a notation it cannot range', () => {
    expect(diceRange('5d10>=8')).toBeNull();
    expect(diceRange('1d6+2')).toBeNull();
    expect(diceRange('')).toBeNull();
    expect(diceRange(undefined)).toBeNull();
  });
});

describe('parseDiceEntry', () => {
  it('reads dice and sums them', () => {
    expect(parseDiceEntry('4 3')).toEqual({ dice: [4, 3], total: 7 });
    expect(parseDiceEntry('4,3')).toEqual({ dice: [4, 3], total: 7 });
  });

  it('reads a single total as one die', () => {
    expect(parseDiceEntry('7')).toEqual({ dice: [7], total: 7 });
  });

  it('rejects anything that is not a whole number', () => {
    expect(parseDiceEntry('')).toBeNull();
    expect(parseDiceEntry('four')).toBeNull();
    expect(parseDiceEntry('4 3.5')).toBeNull();
    expect(parseDiceEntry('-2')).toBeNull();
  });
});

describe('isOutOfRange', () => {
  it('flags an implausible entry', () => {
    expect(isOutOfRange({ dice: [20], total: 20 }, diceRange('2d6'))).toBe(true);
    expect(isOutOfRange({ dice: [4, 3], total: 7 }, diceRange('2d6'))).toBe(false);
  });

  it('never flags an unknown notation', () => {
    expect(isOutOfRange({ dice: [9], total: 9 }, diceRange('5d10>=8'))).toBe(false);
  });
});
