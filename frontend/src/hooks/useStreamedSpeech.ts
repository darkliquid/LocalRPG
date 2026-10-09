import { useCallback, useEffect, useRef } from 'react';
import { PlaybackEntry } from '../types';

interface QueuedItem {
  sequence: number;
  url: string;
  key: string;
}

// useStreamedSpeech plays narration clips as the turn streams, in strict sequence order,
// and records heard offsets in a playback ledger. The chronicle and finalise pass
// can then skip complete clips and resume partial ones from their recorded offset,
// so no line is heard twice. It stays idle when application playback is running:
// one device must own the sound.
export const useStreamedSpeech = (
  enabled: boolean,
  volume: number,
  initialLedger?: Record<string, PlaybackEntry>
) => {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const currentKeyRef = useRef<string | null>(null);
  const pendingRef = useRef<Map<number, QueuedItem>>(new Map());
  const nextExpectedSeqRef = useRef(0);
  const fallbackSeqRef = useRef(0);
  const playedRef = useRef<Set<string>>(new Set());
  const ledgerRef = useRef<Map<string, PlaybackEntry>>(
    new Map(Object.entries(initialLedger ?? {}))
  );
  const drainingRef = useRef(false);

  // Sync initialLedger into ledgerRef when provided
  useEffect(() => {
    if (initialLedger) {
      for (const [key, entry] of Object.entries(initialLedger)) {
        const existing = ledgerRef.current.get(key);
        ledgerRef.current.set(key, {
          played_ms: Math.max(existing?.played_ms ?? 0, entry.played_ms),
          total_ms: Math.max(existing?.total_ms ?? 0, entry.total_ms ?? 0) || undefined,
          complete: existing?.complete || entry.complete,
        });
        if (entry.complete) {
          playedRef.current.add(key);
        }
      }
    }
  }, [initialLedger]);

  const recordCurrentOffset = useCallback((key: string, audio: HTMLAudioElement) => {
    const curTime = audio.currentTime || 0;
    const playedMs = Math.round(curTime * 1000);
    const totalMs = Number.isFinite(audio.duration) ? Math.round(audio.duration * 1000) : undefined;
    if (playedMs > 0) {
      const existing = ledgerRef.current.get(key);
      ledgerRef.current.set(key, {
        played_ms: Math.max(existing?.played_ms ?? 0, playedMs),
        total_ms: Math.max(existing?.total_ms ?? 0, totalMs ?? 0) || undefined,
        complete: existing?.complete ?? false,
      });
    }
  }, []);

  const drain = useCallback(() => {
    if (drainingRef.current) return;

    const expectedSeq = nextExpectedSeqRef.current;
    const next = pendingRef.current.get(expectedSeq);
    if (!next) return;

    pendingRef.current.delete(expectedSeq);
    drainingRef.current = true;

    const existingEntry = ledgerRef.current.get(next.key);
    if (existingEntry?.complete) {
      playedRef.current.add(next.key);
      nextExpectedSeqRef.current++;
      drainingRef.current = false;
      drain();
      return;
    }

    const audio = new Audio(next.url);
    audio.volume = volume;
    audioRef.current = audio;
    currentKeyRef.current = next.key;

    if (existingEntry && existingEntry.played_ms > 0) {
      audio.currentTime = existingEntry.played_ms / 1000;
    }

    audio.onpause = () => {
      recordCurrentOffset(next.key, audio);
    };

    const advance = (heard: boolean) => {
      if (heard) {
        playedRef.current.add(next.key);
        const totalMs = Number.isFinite(audio.duration) ? Math.round(audio.duration * 1000) : undefined;
        ledgerRef.current.set(next.key, {
          played_ms: totalMs ?? Math.round((audio.currentTime || 0) * 1000),
          total_ms: totalMs,
          complete: true,
        });
      } else {
        recordCurrentOffset(next.key, audio);
      }
      currentKeyRef.current = null;
      nextExpectedSeqRef.current++;
      drainingRef.current = false;
      drain();
    };

    audio.onended = () => advance(true);
    audio.onerror = () => advance(false);
    audio.play().catch(() => advance(false));
  }, [volume, recordCurrentOffset]);

  const enqueue = useCallback(
    (url: string, key: string, sequence?: number) => {
      if (!enabled || !url || !key) return;
      const seq = sequence !== undefined ? sequence : fallbackSeqRef.current++;
      pendingRef.current.set(seq, { sequence: seq, url, key });
      drain();
    },
    [enabled, drain]
  );

  // playedKeys is a snapshot of fully heard clips.
  const playedKeys = useCallback(() => new Set(playedRef.current), []);

  // ledger returns a snapshot of heard offsets and completion state.
  const ledger = useCallback((): Record<string, PlaybackEntry> => {
    const out: Record<string, PlaybackEntry> = {};
    ledgerRef.current.forEach((val, key) => {
      out[key] = { ...val };
    });
    return out;
  }, []);

  const reset = useCallback(() => {
    audioRef.current?.pause();
    audioRef.current = null;
    currentKeyRef.current = null;
    pendingRef.current.clear();
    nextExpectedSeqRef.current = 0;
    fallbackSeqRef.current = 0;
    playedRef.current = new Set();
    ledgerRef.current.clear();
    drainingRef.current = false;
  }, []);

  const stop = useCallback(() => {
    if (audioRef.current && currentKeyRef.current) {
      recordCurrentOffset(currentKeyRef.current, audioRef.current);
      audioRef.current.pause();
    }
    audioRef.current = null;
    currentKeyRef.current = null;
    pendingRef.current.clear();
    nextExpectedSeqRef.current = 0;
    fallbackSeqRef.current = 0;
    drainingRef.current = false;
  }, [recordCurrentOffset]);

  return { enqueue, playedKeys, ledger, reset, stop };
};
