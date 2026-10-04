// The export player is built on its own and inlined into a single page, so it
// must stay small and offline. The app editor must never be reachable from it;
// this fails the build when CodeMirror appears in the player output.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';

const playerDir = fileURLToPath(new URL('../../pkg/gui/dist/player', import.meta.url));

function walk(dir) {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    return statSync(full).isDirectory() ? walk(full) : [full];
  });
}

let files;
try {
  files = walk(playerDir);
} catch {
  console.error(`player bundle not found at ${playerDir}; run the frontend build first`);
  process.exit(1);
}

const offenders = files.filter((file) => /codemirror/i.test(readFileSync(file, 'utf8')));
if (offenders.length > 0) {
  console.error('the player bundle must not contain the editor:');
  for (const file of offenders) console.error(`  ${file}`);
  process.exit(1);
}

console.log('player bundle is editor-free');
