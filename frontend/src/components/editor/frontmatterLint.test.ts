import { describe, expect, it } from 'vitest';
import { frontmatterDiagnostics, keyLineOffset, lineStartOffset } from './frontmatterLint';
import { frontmatterRange } from './frontmatter';
import type { FrontmatterSchema } from '../../types';

const schema: FrontmatterSchema = {
  allowUnknown: true,
  keys: [
    { name: 'id', type: 'string', required: true, description: 'The note id.' },
    { name: 'name', type: 'string', required: true, description: 'The display name.' },
  ],
};

describe('lineStartOffset', () => {
  it('returns zero for the first line', () => {
    expect(lineStartOffset('a\nbb\nccc', 1)).toBe(0);
  });

  it('returns the offset of a later line', () => {
    expect(lineStartOffset('a\nbb\nccc', 3)).toBe(5);
  });
});

describe('keyLineOffset', () => {
  it('finds a key line', () => {
    expect(keyLineOffset('id: a\nname: b', 'name')).toBe(6);
  });

  it('returns zero for a key that is not there', () => {
    expect(keyLineOffset('id: a', 'missing')).toBe(0);
  });
});

describe('frontmatterDiagnostics', () => {
  it('reports nothing for known keys', () => {
    const doc = '---\nid: a\nname: A\n---\nbody\n';
    const range = frontmatterRange(doc);
    expect(range).not.toBeNull();
    expect(frontmatterDiagnostics(schema, doc, range!)).toEqual([]);
  });

  it('reports an unknown key as info', () => {
    const doc = '---\nid: a\nsecret: yes\n---\nbody\n';
    const range = frontmatterRange(doc)!;
    const diagnostics = frontmatterDiagnostics(schema, doc, range);
    expect(diagnostics).toHaveLength(1);
    expect(diagnostics[0].severity).toBe('info');
    expect(diagnostics[0].message).toContain('secret');
  });

  it('reports invalid YAML as an error', () => {
    const doc = '---\nid: [unclosed\n---\nbody\n';
    const range = frontmatterRange(doc)!;
    const diagnostics = frontmatterDiagnostics(schema, doc, range);
    expect(diagnostics).toHaveLength(1);
    expect(diagnostics[0].severity).toBe('error');
    expect(diagnostics[0].message).toContain('Invalid YAML');
  });
});
