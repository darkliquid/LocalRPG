// The arithmetic a manual roll entry needs: the range a notation can produce, and
// what the player typed. A local-first app trusts the player, so these are hints
// rather than gates.

export interface DiceRange {
  min: number;
  max: number;
}

export interface DiceEntry {
  dice: number[];
  total: number;
}

// diceRange is the possible total range of a simple NdM notation, or null when the
// notation is not one this can range: a pool with a threshold, an exploding die, or
// a modifier. An unknown notation shows no hint rather than a wrong one.
export function diceRange(notation?: string): DiceRange | null {
  const match = /^\s*(\d+)\s*d\s*(\d+)\s*$/i.exec(notation ?? '');
  if (!match) return null;
  const count = Number(match[1]);
  const faces = Number(match[2]);
  if (!Number.isInteger(count) || !Number.isInteger(faces) || count < 1 || faces < 1) return null;
  return { min: count, max: count * faces };
}

// parseDiceEntry reads what the player typed. "4 3" and "4,3" are two dice, and a
// single "7" is one die of seven, which is the same as a total for the arithmetic.
// It returns null when the entry is not whole numbers.
export function parseDiceEntry(entry: string): DiceEntry | null {
  const parts = entry.trim().split(/[\s,]+/).filter(Boolean);
  if (parts.length === 0) return null;

  const dice: number[] = [];
  for (const part of parts) {
    const value = Number(part);
    if (!Number.isInteger(value) || value < 0) return null;
    dice.push(value);
  }
  return { dice, total: dice.reduce((sum, die) => sum + die, 0) };
}

// isOutOfRange reports whether an entry falls outside the notation's possible
// range. An unknown notation is never out of range, because nothing is known.
export function isOutOfRange(entry: DiceEntry, range: DiceRange | null): boolean {
  if (!range) return false;
  return entry.total < range.min || entry.total > range.max;
}
