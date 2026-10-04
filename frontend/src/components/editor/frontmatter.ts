import type { CompletionSource } from '@codemirror/autocomplete';
import type { FrontmatterSchema } from '../../types';

export interface DocRange {
  from: number;
  to: number;
}

// frontmatterRange returns the span of the YAML block a note starts with, or null
// when the document has no closed frontmatter. It is pure, so the block boundary
// rules are reviewable without a browser.
export function frontmatterRange(doc: string): DocRange | null {
  if (!doc.startsWith('---\n')) return null;
  const end = doc.indexOf('\n---\n', 4);
  if (end === -1) return null;
  return { from: 4, to: end + 1 };
}

// frontmatterKeysPresent lists the top-level keys the block already sets, so
// completion never offers a key that is already there.
export function frontmatterKeysPresent(doc: string, range: DocRange): Set<string> {
  const keys = new Set<string>();
  for (const line of doc.slice(range.from, range.to).split('\n')) {
    const match = /^([A-Za-z_][A-Za-z0-9_-]*)\s*:/.exec(line);
    if (match) keys.add(match[1]);
  }
  return keys;
}

// isKeyPosition reports whether the cursor sits where a key may be typed: inside
// the block, before any colon on its line, and not in a list item or a comment.
export function isKeyPosition(doc: string, pos: number, range: DocRange): boolean {
  if (pos < range.from || pos > range.to) return false;
  const lineStart = doc.lastIndexOf('\n', pos - 1) + 1;
  const before = doc.slice(lineStart, pos);
  if (before.includes(':')) return false;
  return !/^\s*[-#]/.test(before);
}

// frontmatterKeyCompletion offers the accepted keys at a key position, with the
// type and the description the schema carries.
export function frontmatterKeyCompletion(schema: FrontmatterSchema): CompletionSource {
  return (context) => {
    const doc = context.state.doc.toString();
    const range = frontmatterRange(doc);
    if (!range) return null;
    if (!isKeyPosition(doc, context.pos, range)) return null;

    const word = context.matchBefore(/[A-Za-z_][A-Za-z0-9_-]*/);
    if (!word && !context.explicit) return null;
    const from = word ? word.from : context.pos;

    const present = frontmatterKeysPresent(doc, range);
    const options = schema.keys
      .filter((key) => !present.has(key.name))
      .map((key) => ({
        label: key.name,
        type: 'property',
        detail: key.required ? `${key.type} (required)` : key.type,
        info: key.description,
        apply: `${key.name}: `,
      }));

    if (options.length === 0) return null;
    return { from, options, validFor: /^[A-Za-z_][A-Za-z0-9_-]*$/ };
  };
}
