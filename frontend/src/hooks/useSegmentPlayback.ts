import { useCallback, useEffect, useRef, useState } from 'react';
import { TurnSegment } from '../types';

// useSegmentPlayback plays a turn's segments in order, skipping any without a
// clip. It is the one playback implementation, shared by the chronicle and the
// story theater so pacing and controls cannot drift between them.
export const useSegmentPlayback = (
  segments: TurnSegment[] | undefined,
  autoPlay: boolean,
  volume: number
) => {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [playing, setPlaying] = useState(false);

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

      const audio = new Audio(urls[next] as string);
      audio.volume = volume;
      audio.onended = () => playFrom(next + 1);
      audioRef.current = audio;
      setPlaying(true);
      void audio.play();
    },
    [segments, stop, volume]
  );

  useEffect(() => {
    if (!autoPlay) return;
    playFrom(0);
    return stop;
  }, [autoPlay, playFrom, stop]);

  return { playing, play: () => playFrom(0), playFrom, stop };
};
