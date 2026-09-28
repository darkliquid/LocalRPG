import { useCallback, useEffect, useRef, useState } from 'react';
import { APIClient } from '../api/client';
import { TTSConfig, VoiceProfile } from '../types';

// DEFAULT_VOICE_PREVIEW_TEXT is the phrase a preview synthesises. Mirrors
// defaultTTSPreviewText on the server so a preview sounds like the real thing.
export const DEFAULT_VOICE_PREVIEW_TEXT =
  'Local RPG can use a wide range of voices to bring life to your characters, NPCs, and story narration.';

export interface VoicePreview {
  // previewingId is the voice or profile currently being synthesised, so a
  // caller can spin only that row.
  previewingId: string | null;
  error: string | null;
  previewProfile: (profile: VoiceProfile, ttsConfig: TTSConfig, text?: string) => Promise<void>;
  previewConfig: (id: string, ttsConfig: TTSConfig, text?: string) => Promise<void>;
  stop: () => void;
  clearError: () => void;
}

// useVoicePreview owns the one audio element every voice preview plays through,
// so auditioning never stacks playback and never touches the narration queue.
// One hook instance per surface keeps the spinner and the error with the picker
// that started the preview.
export function useVoicePreview(defaultText: string = DEFAULT_VOICE_PREVIEW_TEXT): VoicePreview {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [previewingId, setPreviewingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const stop = useCallback(() => {
    audioRef.current?.pause();
    audioRef.current = null;
    setPreviewingId(null);
  }, []);

  // A picker can unmount mid-preview, and the element would keep playing.
  useEffect(() => stop, [stop]);

  const play = useCallback((dataURI: string, volume: number) => {
    audioRef.current?.pause();
    const audio = new Audio(dataURI);
    audio.volume = Math.min(1, Math.max(0, volume));
    audioRef.current = audio;
    void audio.play().catch(() => {
      setError('Audio generated, but the browser blocked playback. Click the page and retry.');
    });
  }, []);

  const previewConfig = useCallback(
    async (id: string, ttsConfig: TTSConfig, text?: string) => {
      setPreviewingId(id);
      setError(null);
      try {
        const res = await APIClient.testProvider({
          category: 'tts',
          provider: ttsConfig,
          test_prompt: text ?? defaultText,
        });
        if (!res.success || !res.audio_data_uri) {
          setError(res.message || 'Voice preview failed.');
          return;
        }
        play(res.audio_data_uri, ttsConfig.master_volume ?? 1);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Voice preview failed.');
      } finally {
        setPreviewingId(null);
      }
    },
    [defaultText, play],
  );

  // previewProfile synthesises the profile's own voice, pitch, rate, and
  // tunables, so what a user hears is what the character will sound like.
  const previewProfile = useCallback(
    (profile: VoiceProfile, ttsConfig: TTSConfig, text?: string) =>
      previewConfig(
        profile.id,
        {
          ...ttsConfig,
          default_voice: profile.voice_id,
          pitch: profile.pitch,
          speech_rate: profile.speech_rate,
          options: profile.options,
        },
        text,
      ),
    [previewConfig],
  );

  const clearError = useCallback(() => setError(null), []);

  return { previewingId, error, previewProfile, previewConfig, stop, clearError };
}
