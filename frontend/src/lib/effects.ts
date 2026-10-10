// Theatre effects. Every effect is a pure function of a beat's progress, so the
// app and the export (pkg/scene/effects.go) compute the same values from the same
// inputs. Keep the two files in step.

export interface KenBurnsTransform {
  scale: number;
  dx: number;
  dy: number;
}

export interface MoodTint {
  color: string;
  opacity: number;
}

export type WeatherKind = 'rain' | 'snow' | 'fog';

export interface WeatherOverlay {
  kind: WeatherKind;
}

// The magnitudes are deliberately small: a Ken Burns that is noticed is too
// strong.
export const KEN_BURNS_ZOOM = 0.08;
export const KEN_BURNS_PAN = 0.035;
// PARALLAX_PAN is the extra translation the nearest layer receives at full
// progress; a layer's own depth scales it.
export const PARALLAX_PAN = 0.05;

function clamp01(value: number): number {
  if (!Number.isFinite(value)) return 0;
  if (value < 0) return 0;
  if (value > 1) return 1;
  return value;
}

// kenBurns is a slow zoom with a small pan across a still, scaled to the beat's
// progress. Progress zero is the identity, so a beat starts on its own picture.
// The pan direction alternates by seed so consecutive beats do not all drift the
// same way.
export function kenBurns(seed: number, progress: number): KenBurnsTransform {
  const t = clamp01(progress);
  if (t <= 0) return { scale: 1, dx: 0, dy: 0 };
  const direction = seed % 2 === 0 ? -1 : 1;
  const vertical = Math.floor(Math.abs(seed) / 2) % 2 === 0 ? -1 : 1;
  return {
    scale: 1 + KEN_BURNS_ZOOM * t,
    dx: direction * KEN_BURNS_PAN * t,
    dy: vertical * KEN_BURNS_PAN * t,
  };
}

// parallaxOffset is the translation a layer at depth receives for a beat's
// progress: the background (depth 0) does not move, the foreground (depth 1)
// moves most.
export function parallaxOffset(depth: number, progress: number): number {
  return clamp01(depth) * clamp01(progress) * PARALLAX_PAN;
}

// The mood tint the turn's outcome maps to. Success reads warm and brighter, a
// failure reads cool and darker, and an unknown or neutral outcome is no tint at
// all. The mapping mirrors the tone IMG-1 gives the scene prompt.
const MOOD_TINTS: Record<string, MoodTint> = {
  success: { color: '#f59e0b', opacity: 0.1 },
  strong: { color: '#f59e0b', opacity: 0.12 },
  weak: { color: '#a8a29e', opacity: 0 },
  partial: { color: '#a8a29e', opacity: 0 },
  miss: { color: '#38bdf8', opacity: 0.14 },
  fail: { color: '#38bdf8', opacity: 0.14 },
  failure: { color: '#38bdf8', opacity: 0.14 },
};

const NO_TINT: MoodTint = { color: '#a8a29e', opacity: 0 };

export function moodTint(outcome?: string): MoodTint {
  const key = (outcome ?? '').trim().toLowerCase();
  return MOOD_TINTS[key] ?? NO_TINT;
}

// weatherOverlay is the overlay a scene's weather calls for, or null when the
// weather is clear or unknown.
export function weatherOverlay(weather?: string): WeatherOverlay | null {
  const key = (weather ?? '').trim().toLowerCase();
  if (key === 'rain' || key === 'snow' || key === 'fog') return { kind: key };
  return null;
}
