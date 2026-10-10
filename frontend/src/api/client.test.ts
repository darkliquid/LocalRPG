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

describe('APIClient playback ledger and audio status', () => {
  it('audioStatus parses owner', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({ available: true, playing: false, owner: 'device' }),
          { status: 200 }
        )
      )
    );

    const status = await APIClient.audioStatus();
    expect(status.available).toBe(true);
    expect(status.owner).toBe('device');
  });

  it('resends offsets to the server with mergePlaybackLedger', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          turn: 1,
          entries: {
            'clip-1': { played_ms: 1200, total_ms: 3000, complete: false },
          },
        }),
        { status: 200 }
      )
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await APIClient.mergePlaybackLedger('game-1', {
      turn: 1,
      entries: {
        'clip-1': { played_ms: 1200, total_ms: 3000, complete: false },
      },
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/game/game-1/turn/1/ledger',
      expect.objectContaining({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          turn: 1,
          entries: {
            'clip-1': { played_ms: 1200, total_ms: 3000, complete: false },
          },
        }),
      })
    );
    expect(result.entries['clip-1'].played_ms).toBe(1200);
  });
});

describe('APIClient entity deletion', () => {
  it('sends DELETE to the entity route', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);

    await new APIClient('game-1').deleteEntity('saltmarch');

    expect(fetchMock).toHaveBeenCalledWith('/api/game/game-1/entity/saltmarch', { method: 'DELETE' });
  });

  it('throws the body as the message when the delete fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('delete entity saltmarch: file does not exist', { status: 404 }))
    );

    const err = await new APIClient('game-1').deleteEntity('saltmarch').catch((e: unknown) => e);

    expect((err as Error).message).toMatch(/does not exist/);
  });
});

