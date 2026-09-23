import { useEffect, useRef, useState } from 'react';
import { APIClient } from '../api/client';
import { TTSConfig, TTSInspectResponse } from '../types';

// INSPECT_DEBOUNCE_MS lets a burst of edits settle before one inspect runs, so
// dragging a slider does not issue a request per pixel.
const INSPECT_DEBOUNCE_MS = 400;

export interface TTSInspectState {
  inspect: TTSInspectResponse | null;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

// useTTSInspect describes the configuration the editor currently holds, not the
// saved one, so a provider is described before it is applied. It debounces edits
// and ignores a response that a newer edit has superseded.
export function useTTSInspect(config: TTSConfig | null, enabled = true): TTSInspectState {
  const [inspect, setInspect] = useState<TTSInspectResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const requestRef = useRef(0);
  const refreshRef = useRef(false);
  const signature = config ? JSON.stringify(config) : '';

  useEffect(() => {
    if (!enabled || !config) {
      setInspect(null);
      setError(null);
      return;
    }

    const requestID = ++requestRef.current;
    setLoading(true);
    const timer = window.setTimeout(async () => {
      try {
        const refresh = refreshRef.current;
        refreshRef.current = false;
        const res = await APIClient.inspectTTS({ config, refresh });
        if (requestRef.current !== requestID) return;
        setInspect(res);
        setError(res.error ?? null);
      } catch (err) {
        if (requestRef.current !== requestID) return;
        setError(err instanceof Error ? err.message : 'Inspect failed');
      } finally {
        if (requestRef.current === requestID) setLoading(false);
      }
    }, INSPECT_DEBOUNCE_MS);

    return () => window.clearTimeout(timer);
    // The signature stands in for config so an equal config object does not
    // re-trigger the effect on every render.
  }, [signature, nonce, enabled]);

  return {
    inspect,
    loading,
    error,
    refresh: () => {
      refreshRef.current = true;
      setNonce((value) => value + 1);
    },
  };
}
