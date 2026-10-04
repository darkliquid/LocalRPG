import { useCallback, useRef } from 'react';

interface QueuedItem {
  sequence: number;
  url: string;
  key: string;
}

// useStreamedSpeech plays narration clips as the turn streams, in strict sequence order,
// and remembers which keys were played. The chronicle then skips exactly those
// keys when the authoritative turn arrives, so no line is heard twice. It stays
// idle when application playback is running: one device must own the sound.
export const useStreamedSpeech = (enabled: boolean, volume: number) => {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const pendingRef = useRef<Map<number, QueuedItem>>(new Map());
  const nextExpectedSeqRef = useRef(0);
  const fallbackSeqRef = useRef(0);
  const playedRef = useRef<Set<string>>(new Set());
  const drainingRef = useRef(false);

  const drain = useCallback(() => {
    if (drainingRef.current) return;

    const expectedSeq = nextExpectedSeqRef.current;
    const next = pendingRef.current.get(expectedSeq);
    if (!next) return;

    pendingRef.current.delete(expectedSeq);
    drainingRef.current = true;
    const audio = new Audio(next.url);
    audio.volume = volume;
    audioRef.current = audio;

    // A clip counts as heard only once it ends. A line cut short when the turn
    // lands is then played again from its start by the chronicle rather than lost,
    // and a clip that fails to start stays available to retry.
    const advance = (heard: boolean) => {
      if (heard) playedRef.current.add(next.key);
      nextExpectedSeqRef.current++;
      drainingRef.current = false;
      drain();
    };
    audio.onended = () => advance(true);
    audio.onerror = () => advance(false);
    audio.play().catch(() => advance(false));
  }, [volume]);

  const enqueue = useCallback(
    (url: string, key: string, sequence?: number) => {
      if (!enabled || !url || !key) return;
      const seq = sequence !== undefined ? sequence : fallbackSeqRef.current++;
      pendingRef.current.set(seq, { sequence: seq, url, key });
      drain();
    },
    [enabled, drain]
  );

  // playedKeys is a snapshot, so a caller can hand it to the chronicle without
  // racing the next sentence's arrival.
  const playedKeys = useCallback(() => new Set(playedRef.current), []);

  const reset = useCallback(() => {
    audioRef.current?.pause();
    audioRef.current = null;
    pendingRef.current.clear();
    nextExpectedSeqRef.current = 0;
    fallbackSeqRef.current = 0;
    playedRef.current = new Set();
    drainingRef.current = false;
  }, []);

  const stop = useCallback(() => {
    audioRef.current?.pause();
    audioRef.current = null;
    pendingRef.current.clear();
    nextExpectedSeqRef.current = 0;
    fallbackSeqRef.current = 0;
    drainingRef.current = false;
  }, []);

  return { enqueue, playedKeys, reset, stop };
};
