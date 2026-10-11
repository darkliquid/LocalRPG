import React, { useState } from 'react';

// A character with no art is still a character: the palette keeps a name
// recognisable when there is no portrait to show.
const PALETTE = ['#7c3aed', '#0ea5e9', '#059669', '#d97706', '#db2777', '#4f46e5', '#0d9488', '#b45309'];

// avatarColor returns a stable colour for a name, so one character keeps one
// colour across a session and across screens. The hash is FNV-1a, the same family
// the backend uses to assign a voice, so colour and voice agree.
export function avatarColor(name: string): string {
  let hash = 0x811c9dc5;
  for (let i = 0; i < name.length; i++) {
    hash ^= name.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return PALETTE[hash % PALETTE.length];
}

function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return '?';
  const first = words[0][0] ?? '';
  const second = words.length > 1 ? (words[1][0] ?? '') : '';
  return (first + second).toUpperCase() || '?';
}

interface EntityAvatarProps {
  src?: string;
  name: string;
  className?: string;
  alt?: string;
}

// EntityAvatar is the one portrait element in play mode. It renders the image when
// it is given one, and a deterministic initials avatar when it is not, or when the
// image fails, so a missing portrait is never a broken-image glyph.
export const EntityAvatar: React.FC<EntityAvatarProps> = ({ src, name, className, alt }) => {
  const [failed, setFailed] = useState(false);

  if (src && !failed) {
    return <img src={src} alt={alt ?? name} onError={() => setFailed(true)} className={className} loading="lazy" />;
  }

  return (
    <div
      aria-label={name}
      className={`flex items-center justify-center font-sans font-bold text-white select-none ${className ?? ''}`}
      style={{ backgroundColor: avatarColor(name) }}
    >
      {initials(name)}
    </div>
  );
};