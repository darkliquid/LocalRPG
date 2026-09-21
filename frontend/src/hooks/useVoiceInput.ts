import { useState, useRef, useCallback } from 'react';
import { APIClient } from '../api/client';

interface UseVoiceInputOptions {
  onTranscribed: (text: string) => void;
  sttType?: string;
}

interface SpeechRecognitionErrorEvent extends Event {
  error: string;
}

interface SpeechRecognitionEvent extends Event {
  results: {
    [index: number]: {
      [index: number]: {
        transcript: string;
      };
    };
  };
}

interface BrowserSpeechRecognition extends EventTarget {
  continuous: boolean;
  interimResults: boolean;
  lang: string;
  start: () => void;
  stop: () => void;
  onresult: ((event: SpeechRecognitionEvent) => void) | null;
  onerror: ((event: SpeechRecognitionErrorEvent) => void) | null;
  onend: (() => void) | null;
}

export const useVoiceInput = ({ onTranscribed, sttType }: UseVoiceInputOptions) => {
  const [isRecording, setIsRecording] = useState(false);
  const [isTranscribing, setIsTranscribing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const audioChunksRef = useRef<Blob[]>([]);
  const speechRecognitionRef = useRef<BrowserSpeechRecognition | null>(null);

  const startRecording = useCallback(async () => {
    setError(null);

    // 1. Web Speech Mode
    const SpeechRecognition =
      (window as unknown as { SpeechRecognition?: new () => BrowserSpeechRecognition; webkitSpeechRecognition?: new () => BrowserSpeechRecognition }).SpeechRecognition ||
      (window as unknown as { webkitSpeechRecognition?: new () => BrowserSpeechRecognition }).webkitSpeechRecognition;

    if (sttType === 'web-speech' && SpeechRecognition) {
      try {
        const recognition = new SpeechRecognition();
        recognition.continuous = false;
        recognition.interimResults = false;
        recognition.lang = navigator.language || 'en-US';

        recognition.onresult = (event: SpeechRecognitionEvent) => {
          const transcript = event.results[0]?.[0]?.transcript;
          if (transcript) {
            onTranscribed(transcript);
          }
        };

        recognition.onerror = (event: SpeechRecognitionErrorEvent) => {
          console.error('Speech recognition error:', event.error);
          setError(`Speech recognition: ${event.error}`);
          setIsRecording(false);
        };

        recognition.onend = () => {
          setIsRecording(false);
        };

        recognition.start();
        speechRecognitionRef.current = recognition;
        setIsRecording(true);
        return;
      } catch (err: unknown) {
        console.warn('Web speech failed to start, falling back to MediaRecorder:', err);
      }
    }

    // 2. MediaRecorder Mode
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      audioChunksRef.current = [];

      let mimeType = 'audio/webm;codecs=opus';
      if (!MediaRecorder.isTypeSupported(mimeType)) {
        mimeType = MediaRecorder.isTypeSupported('audio/webm')
          ? 'audio/webm'
          : MediaRecorder.isTypeSupported('audio/ogg')
          ? 'audio/ogg'
          : '';
      }

      const recorder = mimeType ? new MediaRecorder(stream, { mimeType }) : new MediaRecorder(stream);
      recorder.ondataavailable = (e) => {
        if (e.data && e.data.size > 0) {
          audioChunksRef.current.push(e.data);
        }
      };

      recorder.onstop = async () => {
        stream.getTracks().forEach((track) => track.stop());
        const audioBlob = new Blob(audioChunksRef.current, {
          type: mimeType || 'audio/webm',
        });

        if (audioBlob.size === 0) {
          return;
        }

        setIsTranscribing(true);
        try {
          const res = await APIClient.transcribeAudio(audioBlob);
          if (res.text) {
            onTranscribed(res.text);
          }
        } catch (err: unknown) {
          console.error('Transcription failed:', err);
          setError(err instanceof Error ? err.message : 'Transcription failed');
        } finally {
          setIsTranscribing(false);
        }
      };

      recorder.start();
      mediaRecorderRef.current = recorder;
      setIsRecording(true);
    } catch (err: unknown) {
      console.error('Microphone access error:', err);
      setError(err instanceof Error && err.name === 'NotAllowedError' ? 'Microphone permission denied' : (err instanceof Error ? err.message : 'Microphone access failed'));
      setIsRecording(false);
    }
  }, [onTranscribed, sttType]);

  const stopRecording = useCallback(() => {
    if (speechRecognitionRef.current) {
      try {
        speechRecognitionRef.current.stop();
      } catch {
        // ignore if already stopped
      }
      speechRecognitionRef.current = null;
    }
    if (mediaRecorderRef.current && mediaRecorderRef.current.state === 'recording') {
      try {
        mediaRecorderRef.current.stop();
      } catch {
        // ignore if already stopped
      }
      mediaRecorderRef.current = null;
    }
    setIsRecording(false);
  }, []);

  const toggleRecording = useCallback(() => {
    if (isRecording) {
      stopRecording();
    } else {
      void startRecording();
    }
  }, [isRecording, startRecording, stopRecording]);

  return {
    isRecording,
    isTranscribing,
    error,
    toggleRecording,
    startRecording,
    stopRecording,
  };
};
