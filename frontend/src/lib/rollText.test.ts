import { describe, it, expect } from 'vitest';
import { stripDiceTelegraphy } from './rollText';

describe('stripDiceTelegraphy', () => {
  it('removes a bracketed roll report', () => {
    expect(stripDiceTelegraphy('You slip past (2d6+3 = 9) and run.')).toBe('You slip past and run.');
  });

  it('removes a line that is only a roll report', () => {
    expect(stripDiceTelegraphy('Roll: 2d6+3 → 9')).toBe('');
  });

  it('removes a bare notation line', () => {
    expect(stripDiceTelegraphy('2d6+3 = 9')).toBe('');
  });

  it('leaves a number in ordinary prose', () => {
    expect(stripDiceTelegraphy('You have 4 rations and a 7-foot pole.')).toBe(
      'You have 4 rations and a 7-foot pole.',
    );
  });

  it('leaves speech alone', () => {
    expect(stripDiceTelegraphy('"I rolled a seven", she said.')).toBe('"I rolled a seven", she said.');
  });
});