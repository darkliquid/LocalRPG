// @vitest-environment node
import { describe, expect, it } from 'vitest';
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';

// The player is built on its own and inlined into a single page, so it must stay
// small and offline. The app editor must never be reachable from it. The bundle
// is a build artifact, so the test skips when the frontend has not been built.
const playerDir = fileURLToPath(new URL('../../../pkg/gui/dist/player', import.meta.url));

const walk = (dir: string): string[] =>
  readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    return statSync(full).isDirectory() ? walk(full) : [full];
  });

describe('the player bundle', () => {
  it.skipIf(!existsSync(playerDir))('does not contain the editor', () => {
    const offenders = walk(playerDir).filter((file) => /codemirror/i.test(readFileSync(file, 'utf8')));
    expect(offenders).toEqual([]);
  });
});
