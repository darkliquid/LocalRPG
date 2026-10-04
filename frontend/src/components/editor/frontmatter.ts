import type { CompletionSource } from '@codemirror/autocomplete';
import { linter } from '@codemirror/lint';
import type { Extension } from '@codemirror/state';
import { hoverTooltip } from '@codemirror/view';
import type { FrontmatterSchema } from '../../types';
import { frontmatterDiagnostics } from './frontmatterLint';

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

// frontmatterLinter reports broken YAML and unknown keys. It is an early warning:
// the save path stays enabled, because the client parser and the Go parser will
// not agree on every edge case and a false positive must not trap the author.
export function frontmatterLinter(schema: FrontmatterSchema): Extension {
  return linter((view) => {
    const doc = view.state.doc.toString();
    const range = frontmatterRange(doc);
    if (!range) return [];
    return frontmatterDiagnostics(schema, doc, range);
  });
}

// frontmatterValueCompletion suggests the values a key's domain has. The values
// are suggestions, not a closed set: only the being-like types change engine
// behaviour, and any other type string is accepted.
export function frontmatterValueCompletion(schema: FrontmatterSchema): CompletionSource {
  return (context) => {
    const doc = context.state.doc.toString();
    const range = frontmatterRange(doc);
    if (!range || context.pos < range.from || context.pos > range.to) return null;

    const lineStart = doc.lastIndexOf('\n', context.pos - 1) + 1;
    const before = doc.slice(lineStart, context.pos);
    const keyMatch = /^([A-Za-z_][A-Za-z0-9_-]*)\s*:\s*/.exec(before);
    if (!keyMatch) return null;

    // A link-bearing field is the wikilink source's job, not this one's.
    if (/\[\[[^[\]]*$/.test(before)) return null;

    const key = schema.keys.find((candidate) => candidate.name === keyMatch[1]);
    if (!key?.values || key.values.length === 0) return null;

    const word = context.matchBefore(/[A-Za-z0-9_-]*/);
    if (!word && !context.explicit) return null;

    return {
      from: word ? word.from : context.pos,
      options: key.values.map((value) => ({
        label: value,
        type: 'enum',
        detail: key.name,
        info: key.description,
      })),
      validFor: /^[A-Za-z0-9_-]*$/,
    };
  };
}

// frontmatterHover explains a key: what it holds and what the engine does with it.
export function frontmatterHover(schema: FrontmatterSchema): Extension {
  return hoverTooltip((view, pos) => {
    const doc = view.state.doc.toString();
    const range = frontmatterRange(doc);
    if (!range || pos < range.from || pos > range.to) return null;

    const lineStart = doc.lastIndexOf('\n', pos - 1) + 1;
    const lineEndIdx = doc.indexOf('\n', lineStart);
    const line = doc.slice(lineStart, lineEndIdx === -1 ? doc.length : lineEndIdx);
    const match = /^([A-Za-z_][A-Za-z0-9_-]*)\s*:/.exec(line);
    if (!match) return null;

    const key = schema.keys.find((candidate) => candidate.name === match[1]);
    if (!key) return null;

    return {
      pos: lineStart,
      end: lineStart + match[1].length,
      create: () => {
        const dom = document.createElement('div');
        dom.className = 'max-w-xs px-3 py-2 text-xs';
        const title = document.createElement('div');
        title.className = 'font-mono text-purple-300';
        title.textContent = `${key.name}: ${key.type}${key.required ? ' (required)' : ''}`;
        const body = document.createElement('div');
        body.className = 'mt-1 text-stone-300';
        body.textContent = key.description;
        dom.append(title, body);
        return { dom };
      },
    };
  });
}
