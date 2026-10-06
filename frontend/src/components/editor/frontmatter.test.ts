import { describe, expect, it } from 'vitest';
import { frontmatterKeysPresent, frontmatterRange, isKeyPosition } from './frontmatter';

const doc = '---\nid: garrick\nname: Garrick\n---\nA grim guard.\n';

describe('frontmatterRange', () => {
  it('spans the YAML block', () => {
    expect(frontmatterRange(doc)).toEqual({ from: 4, to: 30 });
  });

  it('returns null without frontmatter', () => {
    expect(frontmatterRange('Just prose.\n')).toBeNull();
  });

  it('returns null for an unclosed block', () => {
    expect(frontmatterRange('---\nid: garrick\n')).toBeNull();
  });
});

describe('frontmatterKeysPresent', () => {
  it('lists the top-level keys the block sets', () => {
    const range = frontmatterRange(doc);
    expect(range).not.toBeNull();
    expect([...frontmatterKeysPresent(doc, range!)].sort()).toEqual(['id', 'name']);
  });
});

describe('isKeyPosition', () => {
  const range = frontmatterRange(doc)!;

  it('is true at the start of a key line', () => {
    expect(isKeyPosition(doc, 4, range)).toBe(true);
  });

  it('is false once a colon has been typed on the line', () => {
    expect(isKeyPosition(doc, 8, range)).toBe(false);
  });

  it('is false inside a list item', () => {
    const listDoc = '---\ntags:\n  - a\n---\nbody\n';
    const listRange = frontmatterRange(listDoc)!;
    expect(isKeyPosition(listDoc, 14, listRange)).toBe(false);
  });

  it('is false outside the block', () => {
    expect(isKeyPosition(doc, 40, range)).toBe(false);
  });
});
