// Reading-speed pacing. These constants mirror pkg/scene/timing.go, so a beat
// paced in the theatre and the same beat paced in an export agree. Keep the two
// files in step.
export const READING_WORDS_PER_MINUTE = 180;
export const READING_CHARACTERS_PER_MINUTE = 600;
export const MINIMUM_BEAT_MS = 2000;

// readingDurationMs estimates how long a reader needs for a piece of text, never
// below the floor. Word counting drives spaced scripts; unspaced scripts fall
// back to a character rate, which is why two constants exist rather than one.
export function readingDurationMs(
  text: string,
  wpm = READING_WORDS_PER_MINUTE,
  minMs = MINIMUM_BEAT_MS,
): number {
  const trimmed = text.trim();
  if (!trimmed) return minMs;
  const words = trimmed.split(/\s+/).filter(Boolean);
  const ms =
    words.length >= 3
      ? (words.length / wpm) * 60_000
      : (trimmed.length / READING_CHARACTERS_PER_MINUTE) * 60_000;
  return Math.max(minMs, Math.round(ms));
}
