import { describe, expect, it } from 'vitest';
import { formatGenerationError, generationAttemptLines } from './generationError';

describe('formatGenerationError', () => {
  it('leads with the message and suffixes the code', () => {
    expect(formatGenerationError({ code: 'rate_limited', message: 'Slow down' })).toBe('Slow down (rate_limited)');
  });

  it('falls back to the code when there is no message', () => {
    expect(formatGenerationError({ code: 'timeout', message: '   ' })).toBe('timeout');
  });
});

describe('generationAttemptLines', () => {
  it('renders one line per attempt with an optional detail', () => {
    expect(
      generationAttemptLines({
        code: 'provider_error',
        message: 'all providers failed',
        attempts: [
          { role: 'gm', provider: 'openai', code: 'timeout', duration_ms: 1200 },
          { role: 'gm', provider: 'ollama', code: 'empty_response', detail: 'recovered', duration_ms: 40 },
        ],
      }),
    ).toEqual(['gm/openai [timeout]', 'gm/ollama [empty_response]: recovered']);
  });

  it('returns an empty list when there are no attempts', () => {
    expect(generationAttemptLines({ code: 'provider_error', message: 'failed' })).toEqual([]);
  });
});
