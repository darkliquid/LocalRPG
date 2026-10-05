// Checks the entity scaffold generator, which is pure TypeScript with no React, so
// it can be bundled and executed here rather than only exercised by hand.
//
// This exists because the generator decides what a brand new note looks like: which
// keys are present, what each one is documented as, and that the result parses. A
// regression here ships a note the loader cannot read.
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { parse as parseYaml } from 'yaml';

const here = fileURLToPath(new URL('..', import.meta.url));
const out = mkdtempSync(join(tmpdir(), 'localrpg-scaffold-'));

const key = (name, type, required, description) => ({ name, type, required, description });

const baseKeys = [
  key('id', 'string', true, 'The note id.'),
  key('name', 'string', true, 'The display name.'),
  key('type', 'string', true, 'What kind of thing the note is.'),
  key('tags', '[]string', false, 'Free-form labels.'),
  key('aliases', '[]string', false, 'Other names it is known by.'),
  key('location', 'string', false, 'Where the note is.'),
  key('faction', 'string', false, 'The group it belongs to.'),
  key('state', 'map<string, any>', false, 'Arbitrary per-note state.'),
];

const characterKeys = [
  key('id', 'string', true, 'The note id.'),
  key('name', 'string', true, 'The display name.'),
  key('type', 'string', true, 'What kind of thing the note is.'),
  key('appearance', 'string', false, 'A physical description.'),
  key('gender', 'string', false, 'The gender, as free text.'),
  key('age', 'string', false, 'The age, as free text.'),
  key('voice', 'VoiceConfig', false, 'The voice this note speaks with.'),
  key('tags', '[]string', false, 'Free-form labels.'),
  key('aliases', '[]string', false, 'Other names it is known by.'),
  key('location', 'string', false, 'Where the note is.'),
  key('faction', 'string', false, 'The group it belongs to.'),
  key('state', 'map<string, any>', false, 'Arbitrary per-note state.'),
];

const locationKeys = [
  key('id', 'string', true, 'The note id.'),
  key('name', 'string', true, 'The display name.'),
  key('type', 'string', true, 'What kind of thing the note is.'),
  key('appearance', 'string', false, 'A physical description.'),
  key('tags', '[]string', false, 'Free-form labels.'),
  key('aliases', '[]string', false, 'Other names it is known by.'),
  key('location', 'string', false, 'Where the note is.'),
  key('faction', 'string', false, 'The group it belongs to.'),
  key('state', 'map<string, any>', false, 'Arbitrary per-note state.'),
];

const catalog = {
  base_keys: baseKeys,
  types: [
    { id: 'character', label: 'Character', description: 'People.', keys: characterKeys },
    { id: 'location', label: 'Location', description: 'Places.', keys: locationKeys },
  ],
};

try {
  const bundle = join(out, 'entityScaffold.mjs');
  execFileSync(
    'npx',
    ['esbuild', 'src/lib/entityScaffold.ts', '--bundle', '--format=esm', `--outfile=${bundle}`, '--log-level=error'],
    { cwd: here, stdio: 'inherit' },
  );

  const { buildEntityMarkdown } = await import(pathToFileURL(bundle).href);

  let failures = 0;
  const check = (label, condition, detail = '') => {
    if (!condition) {
      console.error(`FAIL ${label}${detail ? `\n  ${detail}` : ''}`);
      failures++;
      return;
    }
    console.log(`ok   ${label}`);
  };

  const frontmatterOf = (markdown) => {
    const end = markdown.indexOf('\n---', 3);
    return markdown.slice(4, end);
  };

  const character = buildEntityMarkdown(catalog, { id: 'lady-evelyn', name: 'Lady Evelyn Vance', type: 'character' });
  const fm = frontmatterOf(character);

  check(
    'required keys carry their values',
    fm.includes('id: "lady-evelyn"') && fm.includes('name: "Lady Evelyn Vance"') && fm.includes('type: "character"'),
  );
  check('an empty list is emitted for tags', fm.includes('tags: []'));
  check('an empty map is emitted for state', fm.includes('state: {}'));
  check('an empty string is emitted for a free-text key', fm.includes('appearance: ""'));
  check('the body starts with the note name as an H1', character.trimEnd().endsWith('# Lady Evelyn Vance'));

  const lines = fm.split('\n');
  let undocumented = 0;
  lines.forEach((line, index) => {
    if (/^[a-z_]+:/.test(line) && !(lines[index - 1] ?? '').startsWith('# ')) undocumented++;
  });
  check('every key is preceded by its description comment', undocumented === 0, `${undocumented} undocumented key(s)`);

  const emptyVoice = buildEntityMarkdown(catalog, { id: 'a', name: 'A', type: 'character' });
  check(
    'a character without a voice still gets the documented block',
    frontmatterOf(emptyVoice).includes('voice:\n  voice_id: ""'),
  );

  const voiced = buildEntityMarkdown(catalog, {
    id: 'a',
    name: 'A',
    type: 'character',
    voice: { provider: 'elevenlabs', voice_id: '21m00', pitch: 1, speech_rate: 1.05 },
  });
  const voicedFm = frontmatterOf(voiced);
  check(
    'a chosen voice is written into the block',
    voicedFm.includes('provider: "elevenlabs"') &&
      voicedFm.includes('voice_id: "21m00"') &&
      voicedFm.includes('speech_rate: 1.05'),
  );

  const location = frontmatterOf(
    buildEntityMarkdown(catalog, { id: 'the-ashen-bastion', name: 'The Ashen Bastion', type: 'location' }),
  );
  check('a location offers appearance', location.includes('appearance: ""'));
  check(
    'a location does not offer voice, gender or age',
    !location.includes('voice:') && !location.includes('gender:') && !location.includes('age:'),
  );

  const unknown = frontmatterOf(buildEntityMarkdown(catalog, { id: 'x', name: 'X', type: 'does-not-exist' }));
  check(
    'an unknown type falls back to the base key set',
    unknown.includes('id: "x"') && !unknown.includes('voice:') && unknown.includes('state: {}'),
  );

  const parsed = parseYaml(fm);
  check('the generated frontmatter parses as YAML', parsed && parsed.id === 'lady-evelyn' && parsed.type === 'character');

  if (failures > 0) {
    console.error(`\n${failures} entity scaffold check(s) failed`);
    process.exit(1);
  }
  console.log('\nentity scaffold is consistent');
} finally {
  rmSync(out, { recursive: true, force: true });
}
