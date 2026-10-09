import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { useStreamedSpeech } from './useStreamedSpeech';

class MockAudio {
  url: string;
  volume = 1;
  currentTime = 0;
  duration = 5;
  paused = true;
  onended: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onpause: (() => void) | null = null;

  static instances: MockAudio[] = [];

  constructor(url: string) {
    this.url = url;
    MockAudio.instances.push(this);
  }

  play() {
    this.paused = false;
    return Promise.resolve();
  }

  pause() {
    this.paused = true;
    this.onpause?.();
  }
}

describe('useStreamedSpeech playback ledger', () => {
  const originalAudio = globalThis.Audio;

  beforeEach(() => {
    MockAudio.instances = [];
    globalThis.Audio = MockAudio as unknown as typeof Audio;
  });

  afterEach(() => {
    globalThis.Audio = originalAudio;
    vi.restoreAllMocks();
  });

  it('records an offset on pause', () => {
    const { result } = renderHook(() => useStreamedSpeech(true, 1));

    act(() => {
      result.current.enqueue('/audio/c1.opus', 'c1', 0);
    });

    expect(MockAudio.instances.length).toBe(1);
    const audio = MockAudio.instances[0];

    // Simulate partial playback reaching 1.25s
    audio.currentTime = 1.25;

    act(() => {
      audio.pause();
    });

    const entry = result.current.ledger()['c1'];
    expect(entry).toBeDefined();
    expect(entry.played_ms).toBe(1250);
    expect(entry.complete).toBeFalsy();
  });

  it('resumes from the offset', () => {
    const initialLedger = {
      c1: { played_ms: 2400, total_ms: 5000, complete: false },
    };
    const { result } = renderHook(() => useStreamedSpeech(true, 1, initialLedger));

    act(() => {
      result.current.enqueue('/audio/c1.opus', 'c1', 0);
    });

    expect(MockAudio.instances.length).toBe(1);
    const audio = MockAudio.instances[0];

    // Playback should seek to 2.4s before/upon play
    expect(audio.currentTime).toBe(2.4);
  });

  it('marks complete on ended', () => {
    const { result } = renderHook(() => useStreamedSpeech(true, 1));

    act(() => {
      result.current.enqueue('/audio/c1.opus', 'c1', 0);
    });

    expect(MockAudio.instances.length).toBe(1);
    const audio = MockAudio.instances[0];
    audio.currentTime = 5.0;

    act(() => {
      audio.onended?.();
    });

    const entry = result.current.ledger()['c1'];
    expect(entry).toBeDefined();
    expect(entry.complete).toBe(true);
    expect(result.current.playedKeys().has('c1')).toBe(true);
  });
});
