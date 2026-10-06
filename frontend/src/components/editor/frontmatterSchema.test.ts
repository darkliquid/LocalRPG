import { afterEach, describe, expect, it, vi } from 'vitest';
import type { FrontmatterSchema } from '../../types';

afterEach(() => {
  vi.resetModules();
  vi.unstubAllGlobals();
});

const schema: FrontmatterSchema = {
  allowUnknown: true,
  keys: [{ name: 'id', type: 'string', required: true, description: 'The note id.' }],
};

describe('loadFrontmatterSchema', () => {
  it('returns the schema the server serves', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => schema })));
    const { loadFrontmatterSchema } = await import('./frontmatterSchema');
    await expect(loadFrontmatterSchema()).resolves.toEqual(schema);
  });

  it('returns null on a non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false })));
    const { loadFrontmatterSchema } = await import('./frontmatterSchema');
    await expect(loadFrontmatterSchema()).resolves.toBeNull();
  });

  it('returns null when the request throws', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('offline');
      }),
    );
    const { loadFrontmatterSchema } = await import('./frontmatterSchema');
    await expect(loadFrontmatterSchema()).resolves.toBeNull();
  });
});
