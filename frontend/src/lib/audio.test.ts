import { describe, expect, it } from 'vitest';
import { groupLastIndex, groupLeaderIndex } from './audio';
import { TurnSegment } from '../types';

const seg = (text: string, clip_group?: string): TurnSegment => ({ kind: 'speech', text, clip_group });

describe('groupLeaderIndex', () => {
  it('returns the first index of a group', () => {
    const segments = [seg('a', 'g1'), seg('b', 'g1'), seg('c', 'g1'), seg('d', 'g2')];
    expect(groupLeaderIndex(segments, 0)).toBe(0);
    expect(groupLeaderIndex(segments, 1)).toBe(0);
    expect(groupLeaderIndex(segments, 2)).toBe(0);
    expect(groupLeaderIndex(segments, 3)).toBe(3);
  });

  it('returns the index itself when ungrouped', () => {
    const segments = [seg('a'), seg('b')];
    expect(groupLeaderIndex(segments, 1)).toBe(1);
  });
});

describe('groupLastIndex', () => {
  it('returns the last index of a group', () => {
    const segments = [seg('a', 'g1'), seg('b', 'g1'), seg('c', 'g1'), seg('d', 'g2')];
    expect(groupLastIndex(segments, 0)).toBe(2);
    expect(groupLastIndex(segments, 1)).toBe(2);
    expect(groupLastIndex(segments, 3)).toBe(3);
  });

  it('returns the index itself when ungrouped', () => {
    const segments = [seg('a'), seg('b')];
    expect(groupLastIndex(segments, 0)).toBe(0);
  });
});
