import { describe, expect, it } from 'vitest';
import { outcomeTone } from './checkTone';

describe('outcomeTone', () => {
  it('maps vocabulary position to tone', () => {
    const v = ['strong', 'weak', 'miss'];
    expect(outcomeTone('strong', v)).toBe('best');
    expect(outcomeTone('weak', v)).toBe('neutral');
    expect(outcomeTone('miss', v)).toBe('worst');
    expect(outcomeTone('unknown', v)).toBe('neutral');
    expect(outcomeTone('pass', [])).toBe('best');
    expect(outcomeTone('fail', [])).toBe('worst');
  });
});
