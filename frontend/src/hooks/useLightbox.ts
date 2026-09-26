import { useCallback, useEffect, useRef, useState } from 'react';

export interface LightboxTarget {
  src: string;
  alt: string;
}

export interface UseLightboxResult {
  lightbox: LightboxTarget | null;
  isLightboxOpen: boolean;
  openLightbox: (src: string, alt: string) => void;
  closeLightbox: () => void;
}

const LIGHTBOX_EXIT_MS = 220;

export function useLightbox(): UseLightboxResult {
  const [lightbox, setLightbox] = useState<LightboxTarget | null>(null);
  const [isLightboxOpen, setIsLightboxOpen] = useState(false);
  const closeTimer = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (closeTimer.current) window.clearTimeout(closeTimer.current);
    };
  }, []);

  const openLightbox = useCallback((src: string, alt: string) => {
    if (closeTimer.current) {
      window.clearTimeout(closeTimer.current);
      closeTimer.current = null;
    }
    setLightbox({ src, alt });
    setIsLightboxOpen(true);
  }, []);

  const closeLightbox = useCallback(() => {
    setIsLightboxOpen(false);
    if (closeTimer.current) window.clearTimeout(closeTimer.current);
    closeTimer.current = window.setTimeout(() => {
      setLightbox(null);
      closeTimer.current = null;
    }, LIGHTBOX_EXIT_MS);
  }, []);

  return { lightbox, isLightboxOpen, openLightbox, closeLightbox };
}
