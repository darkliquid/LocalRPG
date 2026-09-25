import { useCallback, useEffect, useRef, useState } from 'react';
import { TurnSegment } from '../types';

// useSegmentPlayback plays a turn's segments in order, skipping any without a
// clip. It is the one playback implementation, shared by the chronicle and the
// story theater so pacing and controls cannot drift between them.
//
// Browsers refuse to start audio without a user gesture, so an autoplay attempt
// that is rejected is reported as `blocked` rather than swallowed: the caller can
// then offer a Play control, and that click is the gesture that starts playback.
export const useSegmentPlayback = (
  segments: TurnSegment[] | undefined,
  autoPlay: boolean,
  volume: number
) => {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [playing, setPlaying] = useState(false);
  const [blocked, setBlocked] = useState(false);

  const stop = useCallback(() => {
    audioRef.current?.pause();
    audioRef.current = null;
    setPlaying(false);
  }, []);

  const playFrom = useCallback(
    (index: number) => {
      const urls = (segments ?? []).map((segment) => segment.audio_url);
      const next = urls.findIndex((url, i) => i >= index && !!url);
      if (next === -1) {
        stop();
        return;
      }

      audioRef.current?.pause();

      const audio = new Audio(urls[next] as string);
      audio.volume = volume;
      audio.onended = () => playFrom(next + 1);
      audioRef.current = audio;
      setPlaying(true);

      // Preload subsequent segment audio so playback flows continuously without delays
      const following = urls.findIndex((url, i) => i > next && !!url);
      if (following !== -1) {
        const prefetch = new Audio(urls[following] as string);
        prefetch.preload = 'auto';
      }

      audio
        .play()
        .then(() => setBlocked(false))
        .catch(() => {
          setPlaying(false);
          setBlocked(true);
        });
    },
    [segments, stop, volume]
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

  return { playing, blocked, play, playFrom, stop };
};
