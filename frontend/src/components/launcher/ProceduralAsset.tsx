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
  LucideIcon,
} from 'lucide-react';

export function hashString(str: string): number {
  let hash = 0x811c9dc5;
  for (let i = 0; i < str.length; i++) {
    hash ^= str.charCodeAt(i);
    hash += (hash << 1) + (hash << 4) + (hash << 7) + (hash << 8) + (hash << 24);
  }
  return hash >>> 0;
}

const PALETTES = [
  { name: 'Arcane', bg: '#090a0f', from: '#4c1d95', via: '#1e1b4b', to: '#312e81', accent: '#a855f7' },
  { name: 'Void', bg: '#050811', from: '#1e293b', via: '#0f172a', to: '#0284c7', accent: '#38bdf8' },
  { name: 'Deepwood', bg: '#061009', from: '#14532d', via: '#052e16', to: '#166534', accent: '#22c55e' },
  { name: 'Ember', bg: '#140804', from: '#7c2d12', via: '#431407', to: '#9a3412', accent: '#f97316' },
  { name: 'Crimson', bg: '#130508', from: '#881337', via: '#4c0519', to: '#9f1239', accent: '#f43f5e' },
  { name: 'Eldritch', bg: '#041014', from: '#134e4a', via: '#042f2e', to: '#115e59', accent: '#14b8a6' },
];

export function getPalette(id: string) {
  const hash = hashString(id || 'default');
  return PALETTES[hash % PALETTES.length];
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
];

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
  className?: string;
  style?: React.CSSProperties;
}> = ({ id, className = '', style }) => {
  const p = getPalette(id);
  const hash = hashString(id);
  const angle = hash % 360;

  return (
    <div
      className={`relative w-full h-full overflow-hidden ${className}`}
      style={{
        backgroundColor: p.bg,
        backgroundImage: `radial-gradient(ellipse at 75% 30%, ${p.from} 0%, ${p.via} 50%, ${p.bg} 100%), linear-gradient(${angle}deg, ${p.to}22, transparent)`,
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
  const p = getPalette(id);
  const IconComponent = getGenreIcon(genre) || getGenreIcon(name);
  const monogram = getMonogram(name);

  return (
    <div
      className={`relative flex items-center justify-center rounded-xl overflow-hidden font-sans font-bold select-none border border-white/15 shadow-md ${className}`}
      style={{
        width: size,
        height: size,
        background: `linear-gradient(135deg, ${p.from} 0%, ${p.via} 100%)`,
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
