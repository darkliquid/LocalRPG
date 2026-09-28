import React from 'react';

interface CinematicOverlayProps {
  enabled: boolean;
}

/**
 * CinematicOverlay is the atmospheric backdrop layer behind
 * `preferences.cinematic_effects`: a soft vignette plus the existing film-noise
 * texture. It is inert, so it never intercepts a click, and it redraws as a
 * static texture when the user prefers reduced motion.
 */
export const CinematicOverlay: React.FC<CinematicOverlayProps> = ({ enabled }) => {
  if (!enabled) return null;
  return (
    <div
      aria-hidden="true"
      className="pointer-events-none fixed inset-0 z-30"
      style={{
        background:
          'radial-gradient(ellipse at center, rgba(0,0,0,0) 45%, rgba(0,0,0,0.45) 100%)',
      }}
    >
      <div className="absolute inset-0 bg-noise opacity-40 motion-reduce:opacity-25" />
    </div>
  );
};
