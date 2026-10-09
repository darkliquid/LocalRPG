import { useCallback, useEffect, useRef, useState } from 'react';
import { PlaybackEntry, TurnSegment } from '../types';
import { clipKeyFromURL, segmentClipURLs, segmentIsGroupLeader } from '../lib/audio';
import { APIClient } from '../api/client';

// Clip names one unit of a segment's audio and the segment it belongs to, so
// playback can walk a flattened running order while the UI still speaks in beats.
interface Clip {
  segmentIndex: number;
  url: string;
}

interface SegmentPlaybackOptions {
  autoPlay: boolean;
  volume?: number;
  // Clips already heard while the turn streamed, so the finalise pass plays only
  // the rest of the turn: the played set is exact because a clip is one unit.
  skipKeys?: ReadonlySet<string>;
  // The playback ledger: complete clips are skipped, and partially heard clips
  // are resumed from their recorded offset.
  ledger?: Record<string, PlaybackEntry>;
  // Needed only to regenerate a beat, which is a request rather than playback.
  gameId?: string;
  turnNumber?: number;
}

// useSegmentPlayback plays a turn's segments in order, skipping any without a
// clip. It is the one playback implementation, shared by the chronicle and the
// story theater so pacing and controls cannot drift between them.
//
// Browsers refuse to start audio without a user gesture, so an autoplay attempt
// that is rejected is reported as `blocked` rather than swallowed: the caller can
// then offer a Play control, and that click is the gesture that starts playback.
export const useSegmentPlayback = (
  segments: TurnSegment[] | undefined,
  options: SegmentPlaybackOptions
) => {
  const { autoPlay, volume = 1, skipKeys, ledger, gameId, turnNumber } = options;
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const prefetchRef = useRef<HTMLAudioElement | null>(null);
  const prefetchedUrlRef = useRef<string | null>(null);
  const [playing, setPlaying] = useState(false);
  const [blocked, setBlocked] = useState(false);
  const [playingIndex, setPlayingIndex] = useState<number | null>(null);

  // The turn's clips in play order, one entry per unit of every segment, minus
  // whatever has already been heard. A group's clip is listed once, under its
  // first segment, so a shared clip is not played once per segment it covers.
  const clipsFor = useCallback((): Clip[] => {
    const clips: Clip[] = [];
    (segments ?? []).forEach((segment, segmentIndex) => {
      if (!segmentIsGroupLeader(segments, segmentIndex)) return;
      segmentClipURLs(segment).forEach((url) => {
        const key = clipKeyFromURL(url);
        if (skipKeys?.has(key)) return;
        if (ledger?.[key]?.complete) return;
        clips.push({ segmentIndex, url });
      });
    });
    return clips;
  }, [segments, skipKeys, ledger]);

  const stop = useCallback(() => {
    audioRef.current?.pause();
    audioRef.current = null;
    setPlaying(false);
    setPlayingIndex(null);
  }, []);

  const playUrl = useCallback(
    (url: string, index: number, onEnded?: () => void) => {
      audioRef.current?.pause();
      const audio = new Audio(url);
      audio.volume = volume;
      const key = clipKeyFromURL(url);
      const entry = ledger?.[key];
      if (entry && !entry.complete && entry.played_ms > 0) {
        audio.currentTime = entry.played_ms / 1000;
      }
      audio.onended = () => {
        setPlayingIndex(null);
        onEnded?.();
      };
      audioRef.current = audio;
      setPlaying(true);
      setPlayingIndex(index);
      audio
        .play()
        .then(() => setBlocked(false))
        .catch(() => {
          setPlaying(false);
          setPlayingIndex(null);
          setBlocked(true);
        });
    },
    [volume, ledger]
  );

  const playFrom = useCallback(
    (index: number) => {
      const clips = clipsFor();
      const next = clips.findIndex((clip) => clip.segmentIndex >= index);
      if (next === -1) {
        stop();
        return;
      }

      // Preload the next clip and keep it, so the browser has it ready when the
      // current one ends. Discarding the element let it be collected unplayed.
      const following = clips[next + 1];
      if (following && prefetchedUrlRef.current !== following.url) {
        const prefetch = new Audio(following.url);
        prefetch.preload = 'auto';
        prefetchRef.current = prefetch;
        prefetchedUrlRef.current = following.url;
      }

      playUrl(clips[next].url, clips[next].segmentIndex, () => playFrom(clips[next].segmentIndex + 1));
    },
    [clipsFor, stop, playUrl]
  );

  // regenerateFrom re-synthesizes one beat and plays its refreshed clips: the
  // server bypasses the cache, so the keys, and therefore the URLs, can change.
  const regenerateFrom = useCallback(
    async (index: number) => {
      if (!gameId || turnNumber === undefined) return;
      try {
        const urls = await APIClient.regenerateSegmentAudio(gameId, turnNumber, index);
        if (urls.length === 0) return;
        playUrl(urls[0], index);
      } catch (err) {
        console.error('regenerate segment audio:', err);
      }
    },
    [gameId, turnNumber, playUrl]
  );

  useEffect(() => {
    if (!autoPlay) return;
    playFrom(0);
    return stop;
  }, [autoPlay, playFrom, stop]);

  const play = useCallback(() => {
    setBlocked(false);
    playFrom(0);
  }, [playFrom]);

  return { playing, blocked, playingIndex, play, playFrom, regenerateFrom, stop };
};
