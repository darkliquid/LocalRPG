import { useState } from 'react';

export interface LightboxTarget {
  src: string;
  alt: string;
}

export interface UseLightboxResult {
  lightbox: LightboxTarget | null;
  openLightbox: (src: string, alt: string) => void;
  closeLightbox: () => void;
}

export function useLightbox(): UseLightboxResult {
  const [lightbox, setLightbox] = useState<LightboxTarget | null>(null);
  return {
    lightbox,
    openLightbox: (src, alt) => setLightbox({ src, alt }),
    closeLightbox: () => setLightbox(null),
  };
}
