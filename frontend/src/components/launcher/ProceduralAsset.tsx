import React from 'react';
import {
  Shield,
  Sword,
  Rocket,
  Skull,
  Ghost,
  Cpu,
  Flame,
  TreePine,
  Dices,
  Building2,
  LucideIcon,
} from 'lucide-react';
import { GENRE_PALETTES, genrePalette } from '../../lib/genre';

export function hashString(str: string): number {
  let hash = 0x811c9dc5;
  for (let i = 0; i < str.length; i++) {
    hash ^= str.charCodeAt(i);
    hash += (hash << 1) + (hash << 4) + (hash << 7) + (hash << 8) + (hash << 24);
  }
  return hash >>> 0;
}

// The palette table lives in lib/genre.ts, so the launcher, the app background,
// the empty states, the site, and the export all read one source. A banner with
// no genre still varies by its id, as it did.
export function getPalette(id: string) {
  const hash = hashString(id || 'default');
  return GENRE_PALETTES[hash % GENRE_PALETTES.length];
}

const GENRE_ICONS: Array<{ match: RegExp; icon: LucideIcon }> = [
  { match: /fantasy|magic|sorcery|dragon|dnd|rpg/i, icon: Sword },
  { match: /sci-?fi|space|cyber|futur/i, icon: Rocket },
  { match: /cyberpunk|neon|tech|hack/i, icon: Cpu },
  { match: /horror|dark|grim|void|death|undead/i, icon: Skull },
  { match: /nature|wild|forest|beast/i, icon: TreePine },
  { match: /mystery|stealth|shadow|ghost/i, icon: Ghost },
  { match: /war|battle|shield|iron/i, icon: Shield },
  { match: /apocalypse|wasteland|flame|fire/i, icon: Flame },
  { match: /western|frontier|cowboy|saloon/i, icon: Flame },
  { match: /modern|contemporary|urban|city/i, icon: Building2 },
  { match: /histor|victorian|steam|medieval|ancient/i, icon: Shield },
];

// GENRE_ICONS_BY_NAME resolves a palette's icon name, so lib/genre.ts can name an
// icon without importing React components.
const ICONS_BY_NAME: Record<string, LucideIcon> = {
  Sword,
  Rocket,
  Cpu,
  Skull,
  Flame,
  TreePine,
  Ghost,
  Shield,
  Dices,
  Building2,
};

export function genreIconByName(name?: string): LucideIcon | null {
  if (!name) return null;
  return ICONS_BY_NAME[name] ?? null;
}

export function getGenreIcon(genreOrTag?: string): LucideIcon | null {
  if (!genreOrTag) return null;
  for (const entry of GENRE_ICONS) {
    if (entry.match.test(genreOrTag)) return entry.icon;
  }
  return null;
}

export function getMonogram(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

export const ProceduralBanner: React.FC<{
  id: string;
  name?: string;
  genre?: string;
  className?: string;
  style?: React.CSSProperties;
}> = ({ id, genre, className = '', style }) => {
  const p = genre ? genrePalette(genre) : getPalette(id);
  const hash = hashString(id);
  const angle = hash % 360;

  return (
    <div
      className={`relative w-full h-full overflow-hidden ${className}`}
      style={{
        backgroundColor: p.to,
        backgroundImage: `radial-gradient(ellipse at 75% 30%, ${p.from} 0%, ${p.to} 55%, ${p.to} 100%), linear-gradient(${angle}deg, ${p.accent}22, transparent)`,
        ...style,
      }}
    >
      {/* Cinematic tabletop ambient texture */}
      <div className="absolute inset-0 bg-gradient-to-t from-black/85 via-black/20 to-black/60 pointer-events-none" />
      <div className="absolute inset-0 bg-radial-[circle_at_center] from-transparent via-transparent to-black/60 pointer-events-none" />
    </div>
  );
};

export const ProceduralIcon: React.FC<{
  id: string;
  name: string;
  genre?: string;
  size?: number;
  className?: string;
}> = ({ id, name, genre, size = 48, className = '' }) => {
  const p = genre ? genrePalette(genre) : getPalette(id);
  const IconComponent =
    (genre ? genreIconByName(genrePalette(genre).icon) : null) || getGenreIcon(genre) || getGenreIcon(name);
  const monogram = getMonogram(name);

  return (
    <div
      className={`relative flex items-center justify-center rounded-xl overflow-hidden font-sans font-bold select-none border border-white/15 shadow-md ${className}`}
      style={{
        width: size,
        height: size,
        background: `linear-gradient(135deg, ${p.from} 0%, ${p.to} 100%)`,
        color: '#ffffff',
      }}
    >
      <div className="absolute inset-0 bg-white/5 pointer-events-none" />
      {IconComponent ? (
        <IconComponent style={{ width: size * 0.48, height: size * 0.48, color: p.accent }} />
      ) : (
        <span style={{ fontSize: size * 0.38, letterSpacing: '0.05em' }}>{monogram}</span>
      )}
    </div>
  );
};
