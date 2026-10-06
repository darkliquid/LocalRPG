import { describe, expect, it } from 'vitest';
import { parse as parseYaml } from 'yaml';
import { buildEntityMarkdown } from './entityScaffold';
import type { EntityTypeCatalog, FrontmatterKeySchema } from '../types';

const key = (name: string, type: string, required: boolean, description: string): FrontmatterKeySchema => ({
  name,
  type,
  required,
  description,
});

const baseKeys: FrontmatterKeySchema[] = [
  key('id', 'string', true, 'The note id.'),
  key('name', 'string', true, 'The display name.'),
  key('type', 'string', true, 'What kind of thing the note is.'),
  key('tags', '[]string', false, 'Free-form labels.'),
  key('aliases', '[]string', false, 'Other names it is known by.'),
  key('location', 'string', false, 'Where the note is.'),
  key('faction', 'string', false, 'The group it belongs to.'),
  key('state', 'map<string, any>', false, 'Arbitrary per-note state.'),
];

const characterKeys: FrontmatterKeySchema[] = [
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

const locationKeys: FrontmatterKeySchema[] = [
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

const catalog: EntityTypeCatalog = {
  base_keys: baseKeys,
  types: [
    { id: 'character', label: 'Character', description: 'People.', keys: characterKeys },
    { id: 'location', label: 'Location', description: 'Places.', keys: locationKeys },
  ],
};

const frontmatterOf = (markdown: string): string => {
  const end = markdown.indexOf('\n---', 3);
  return markdown.slice(4, end);
};

describe('the entity scaffold', () => {
  it('writes required keys with their values', () => {
    const fm = frontmatterOf(buildEntityMarkdown(catalog, { id: 'lady-evelyn', name: 'Lady Evelyn Vance', type: 'character' }));
    expect(fm).toContain('id: "lady-evelyn"');
    expect(fm).toContain('name: "Lady Evelyn Vance"');
    expect(fm).toContain('type: "character"');
  });

  it('emits an empty list for tags', () => {
    const fm = frontmatterOf(buildEntityMarkdown(catalog, { id: 'a', name: 'A', type: 'character' }));
    expect(fm).toContain('tags: []');
  });

  it('emits an empty map for state', () => {
    const fm = frontmatterOf(buildEntityMarkdown(catalog, { id: 'a', name: 'A', type: 'character' }));
    expect(fm).toContain('state: {}');
  });

  it('emits an empty string for a free-text key', () => {
    const fm = frontmatterOf(buildEntityMarkdown(catalog, { id: 'a', name: 'A', type: 'character' }));
    expect(fm).toContain('appearance: ""');
  });

  it('starts the body with the note name as an H1', () => {
    const character = buildEntityMarkdown(catalog, { id: 'lady-evelyn', name: 'Lady Evelyn Vance', type: 'character' });
    expect(character.trimEnd().endsWith('# Lady Evelyn Vance')).toBe(true);
  });

  it('precedes every key with its description comment', () => {
    const fm = frontmatterOf(buildEntityMarkdown(catalog, { id: 'a', name: 'A', type: 'character' }));
    const lines = fm.split('\n');
    let undocumented = 0;
    lines.forEach((line, index) => {
      if (/^[a-z_]+:/.test(line) && !(lines[index - 1] ?? '').startsWith('# ')) undocumented++;
    });
    expect(undocumented).toBe(0);
  });

  it('still writes the documented voice block for a character without a voice', () => {
    const emptyVoice = buildEntityMarkdown(catalog, { id: 'a', name: 'A', type: 'character' });
    expect(frontmatterOf(emptyVoice)).toContain('voice:\n  voice_id: ""');
  });

  it('writes a chosen voice into the block', () => {
    const voiced = buildEntityMarkdown(catalog, {
      id: 'a',
      name: 'A',
      type: 'character',
      voice: { provider: 'elevenlabs', voice_id: '21m00', pitch: 1, speech_rate: 1.05 },
    });
    const voicedFm = frontmatterOf(voiced);
    expect(voicedFm).toContain('provider: "elevenlabs"');
    expect(voicedFm).toContain('voice_id: "21m00"');
    expect(voicedFm).toContain('speech_rate: 1.05');
  });

  it('offers appearance for a location', () => {
    const location = frontmatterOf(buildEntityMarkdown(catalog, { id: 'the-ashen-bastion', name: 'The Ashen Bastion', type: 'location' }));
    expect(location).toContain('appearance: ""');
  });

  it('does not offer voice, gender or age for a location', () => {
    const location = frontmatterOf(buildEntityMarkdown(catalog, { id: 'the-ashen-bastion', name: 'The Ashen Bastion', type: 'location' }));
    expect(location).not.toContain('voice:');
    expect(location).not.toContain('gender:');
    expect(location).not.toContain('age:');
  });

  it('falls back to the base key set for an unknown type', () => {
    const unknown = frontmatterOf(buildEntityMarkdown(catalog, { id: 'x', name: 'X', type: 'does-not-exist' }));
    expect(unknown).toContain('id: "x"');
    expect(unknown).not.toContain('voice:');
    expect(unknown).toContain('state: {}');
  });

  it('generates frontmatter that parses as YAML', () => {
    const fm = frontmatterOf(buildEntityMarkdown(catalog, { id: 'lady-evelyn', name: 'Lady Evelyn Vance', type: 'character' }));
    const parsed = parseYaml(fm) as { id?: string; type?: string };
    expect(parsed?.id).toBe('lady-evelyn');
    expect(parsed?.type).toBe('character');
  });
});
