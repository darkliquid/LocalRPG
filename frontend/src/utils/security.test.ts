import { describe, expect, it } from 'vitest';
import { safeImagePreview } from './security';

describe('safeImagePreview', () => {
  it('accepts the allowed schemes', () => {
    expect(safeImagePreview('blob:http://localhost/abc')).toBe('blob:http://localhost/abc');
    expect(safeImagePreview('/api/game/1/asset')).toBe('/api/game/1/asset');
    expect(safeImagePreview('data:image/png;base64,AAAA')).toBe('data:image/png;base64,AAAA');
  });

  it('rejects anything else', () => {
    expect(safeImagePreview('javascript:alert(1)')).toBeUndefined();
    expect(safeImagePreview('https://evil.example/x.png')).toBeUndefined();
    expect(safeImagePreview('')).toBeUndefined();
    expect(safeImagePreview(null)).toBeUndefined();
    expect(safeImagePreview(undefined)).toBeUndefined();
  });
});
