import { existsSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { GENRE_PALETTES, contrastRatio, genreCopy, genreGradient, genrePalette } from './genre';

interface GenreFixture {
  genres: Array<{ id: string; label: string; from: string; to: string; accent: string; icon: string }>;
  copy: Record<string, { empty: string; loading: string }>;
}

// The fixture is the shared table the Go side also reads, so it is found by
// walking up from the test's working directory rather than by a fixed path.
function fixturePath(): string {
  let dir = process.cwd();
  for (let i = 0; i < 6; i++) {
    const candidate = join(dir, 'pkg', 'scene', 'testdata', 'genre-palettes.json');
    if (existsSync(candidate)) return candidate;
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  throw new Error('the shared genre fixture was not found');
}

function loadFixture(): GenreFixture {
  return JSON.parse(readFileSync(fixturePath(), 'utf8')) as GenreFixture;
}

describe('genrePalette', () => {
  it('resolves known genres and defaults', () => {
    expect(genrePalette('cyberpunk').id).toBe('cyberpunk');
    expect(genrePalette('SCI-FI').id).toBe('scifi');
    expect(genrePalette('nonsense').id).toBe('neutral');
    expect(genrePalette(undefined).id).toBe('neutral');
  });

  it('matches the Go table the export reads', () => {
    expect(GENRE_PALETTES).toEqual(loadFixture().genres);
  });
});

describe('genreCopy', () => {
  it('has a neutral fallback', () => {
    expect(genreCopy('horror').loading).not.toBe('');
    expect(genreCopy('nonsense').loading).toBe(genreCopy(undefined).loading);
    expect(genreCopy('fantasy').empty).not.toBe(genreCopy(undefined).empty);
  });

  it('matches the shared fixture', () => {
    const fixture = loadFixture();
    for (const palette of GENRE_PALETTES) {
      expect(genreCopy(palette.id)).toEqual(fixture.copy[palette.id]);
    }
  });
});

describe('genreGradient', () => {
  it('uses the palette colours', () => {
    const gradient = genreGradient('cyberpunk');
    expect(gradient).toContain(genrePalette('cyberpunk').from);
    expect(gradient).toContain(genrePalette('cyberpunk').to);
  });
});

describe('contrast', () => {
  it('keeps the app text readable on every palette', () => {
    for (const palette of GENRE_PALETTES) {
      expect(contrastRatio(palette.from, '#f5f5f4')).toBeGreaterThan(4.5);
    }
  });
});
