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

describe('APIClient registry sources', () => {
  it('lists registry sources with their fetch state', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([{ url: 'https://example.org/index.json', name: 'Example', package_count: 3 }]), {
        status: 200,
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    const sources = await APIClient.listRegistrySources();

    expect(fetchMock).toHaveBeenCalledWith('/api/registry/sources');
    expect(sources[0].name).toBe('Example');
    expect(sources[0].package_count).toBe(3);
  });

  it('adds a registry source', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ url: 'https://example.org/index.json' }), { status: 201 })
    );
    vi.stubGlobal('fetch', fetchMock);

    await APIClient.addRegistrySource('https://example.org/index.json');

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/registry/sources',
      expect.objectContaining({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: 'https://example.org/index.json' }),
      })
    );
  });

  it('removes a registry source', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await APIClient.removeRegistrySource('https://example.org/index.json');

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/registry/sources?url=' + encodeURIComponent('https://example.org/index.json'),
      { method: 'DELETE' }
    );
  });

  it('throws an HTTPError carrying the body on failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('registry source already configured', { status: 409 }))
    );

    const err = await APIClient.addRegistrySource('https://example.org/index.json').catch((e: unknown) => e);

    expect(err).toBeInstanceOf(HTTPError);
    expect((err as Error).message).toMatch(/already configured/);
  });
});

