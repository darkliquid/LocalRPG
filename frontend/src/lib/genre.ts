// The genre palette and copy source. The launcher, the app background, the empty
// and loading states, the showcase site, and the export all read these values, so
// a campaign looks the same everywhere. The table is duplicated in
// pkg/scene/genre.go, and a parity test over the shared fixture
// (pkg/scene/testdata/genre-palettes.json) fails if the two drift.

export interface GenrePalette {
  id: string;
  label: string;
  /** The gradient's centre colour. */
  from: string;
  /** The gradient's edge colour. */
  to: string;
  accent: string;
  /** A lucide icon name, resolved by the component that renders it. */
  icon: string;
}

export interface GenreCopy {
  empty: string;
  loading: string;
}

export const GENRE_PALETTES: GenrePalette[] = [
  { id: 'neutral', label: 'Neutral', from: '#261e1b', to: '#0c0a09', accent: '#a855f7', icon: 'Dices' },
  { id: 'fantasy', label: 'Fantasy', from: '#1e1b4b', to: '#090a0f', accent: '#a855f7', icon: 'Sword' },
  { id: 'cyberpunk', label: 'Cyberpunk', from: '#0e3b4a', to: '#050811', accent: '#22d3ee', icon: 'Cpu' },
  { id: 'horror', label: 'Horror', from: '#3b1111', to: '#050505', accent: '#ef4444', icon: 'Skull' },
  { id: 'scifi', label: 'Science Fiction', from: '#123055', to: '#050914', accent: '#38bdf8', icon: 'Rocket' },
  { id: 'western', label: 'Western', from: '#4a2a12', to: '#140b05', accent: '#f59e0b', icon: 'Flame' },
  { id: 'modern', label: 'Modern', from: '#2b3a45', to: '#0c1013', accent: '#94a3b8', icon: 'Building2' },
  { id: 'historical', label: 'Historical', from: '#3a3220', to: '#0f0d08', accent: '#d6b25e', icon: 'Shield' },
];

export const GENRE_COPY: Record<string, GenreCopy> = {
  neutral: { empty: 'Nothing here yet.', loading: 'Loading...' },
  fantasy: { empty: 'The chronicle is still blank.', loading: 'Consulting the oracle...' },
  cyberpunk: { empty: 'No records on this grid.', loading: 'Jacking in...' },
  horror: { empty: 'The pages are empty. For now.', loading: 'Listening for something...' },
  scifi: { empty: 'No logs recorded.', loading: 'Scanning the sector...' },
  western: { empty: 'Nothing to report, partner.', loading: 'Saddling up...' },
  modern: { empty: 'Nothing filed yet.', loading: 'Pulling the records...' },
  historical: { empty: 'The archive is bare.', loading: 'Consulting the archive...' },
};

// The names a world may use for a genre, so "sci-fi" and "cyber" resolve without a
// new palette.
const GENRE_ALIASES: Record<string, string> = {
  fantasy: 'fantasy', 'high-fantasy': 'fantasy', 'dark-fantasy': 'fantasy', medieval: 'fantasy',
  cyberpunk: 'cyberpunk', cyber: 'cyberpunk', neon: 'cyberpunk', dystopian: 'cyberpunk',
  horror: 'horror', gothic: 'horror', grimdark: 'horror', lovecraftian: 'horror',
  scifi: 'scifi', 'sci-fi': 'scifi', space: 'scifi', 'science-fiction': 'scifi', 'space-opera': 'scifi',
  western: 'western', wildwest: 'western', 'wild-west': 'western',
  modern: 'modern', contemporary: 'modern', urban: 'modern', noir: 'modern',
  historical: 'historical', history: 'historical', steampunk: 'historical', victorian: 'historical',
};

function genreID(genre?: string): string {
  const key = (genre ?? '').trim().toLowerCase();
  if (key === '') return 'neutral';
  return GENRE_ALIASES[key] ?? key;
}

/** genrePalette resolves a genre, or a world's tags, to its palette. */
export function genrePalette(genre?: string): GenrePalette {
  const id = genreID(genre);
  return GENRE_PALETTES.find((palette) => palette.id === id) ?? GENRE_PALETTES[0];
}

/** genreCopy resolves a genre to its empty and loading copy. */
export function genreCopy(genre?: string): GenreCopy {
  return GENRE_COPY[genreID(genre)] ?? GENRE_COPY.neutral;
}

/** genreGradient is the app background for a genre. */
export function genreGradient(genre?: string): string {
  const palette = genrePalette(genre);
  return `radial-gradient(ellipse at center, ${palette.from} 0%, ${palette.to} 100%)`;
}

function channelLuminance(value: number): number {
  const c = value / 255;
  return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

function relativeLuminance(hex: string): number {
  const clean = hex.replace('#', '');
  const r = parseInt(clean.slice(0, 2), 16);
  const g = parseInt(clean.slice(2, 4), 16);
  const b = parseInt(clean.slice(4, 6), 16);
  return 0.2126 * channelLuminance(r) + 0.7152 * channelLuminance(g) + 0.0722 * channelLuminance(b);
}

/** contrastRatio is the WCAG contrast ratio between two colours. */
export function contrastRatio(a: string, b: string): number {
  const la = relativeLuminance(a);
  const lb = relativeLuminance(b);
  const lighter = Math.max(la, lb);
  const darker = Math.min(la, lb);
  return (lighter + 0.05) / (darker + 0.05);
}
