import { describe, it, expect, vi, afterEach } from 'vitest';
import { APIClient, HTTPError } from './client';

afterEach(() => {
  vi.restoreAllMocks();
});

// A failed response is read through the error envelope, so a caller gets the
// message and the code rather than the envelope's JSON.
describe('APIClient error envelopes', () => {
  it('surfaces the message and code of an error envelope', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            error: {
              code: 'generation_limit',
              message: 'the source is larger than the chunk limit: it holds 300 chunks and generation.max_chunks is 200',
            },
          }),
          { status: 400 }
        )
      )
    );

    const err = await APIClient.previewWorldEntities('w', { instruction: 'x' }).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(HTTPError);
    expect((err as HTTPError).code).toBe('generation_limit');
    expect((err as HTTPError).message).toMatch(/max_chunks is 200/);
    expect((err as HTTPError).message).not.toMatch(/^\{/);
  });

  it('keeps a non-JSON body as the message', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('boom', { status: 500 })));

    const err = await APIClient.previewWorldEntities('w', { instruction: 'x' }).catch((e: unknown) => e);

    expect((err as HTTPError).message).toBe('boom');
    expect((err as HTTPError).code).toBeUndefined();
  });
});
