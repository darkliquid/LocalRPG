export type Tone = 'best' | 'neutral' | 'worst';

// outcomeTone derives a check's tone from the system's own outcome vocabulary:
// the first declared outcome is the best, the last is the worst, and anything
// between is neutral (the partial or weak case). An unknown outcome is neutral,
// and a system with no vocabulary falls back to a two-tone pass/fail.
export function outcomeTone(outcome: string, vocabulary: string[]): Tone {
  const vocab = vocabulary.length > 0 ? vocabulary : ['pass', 'fail'];
  const i = vocab.indexOf(outcome);
  if (i === 0) return 'best';
  if (i === vocab.length - 1) return 'worst';
  return 'neutral';
}
