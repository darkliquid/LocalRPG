import { parse as parseYaml } from 'yaml';
import type { Diagnostic } from '@codemirror/lint';
import type { FrontmatterSchema } from '../../types';
import type { DocRange } from './frontmatter';

// lineStartOffset returns the byte offset of a 1-based line within text.
export function lineStartOffset(text: string, line: number): number {
  if (line <= 1) return 0;
  let offset = 0;
  for (let i = 1; i < line; i++) {
    const next = text.indexOf('\n', offset);
    if (next === -1) return text.length;
    offset = next + 1;
  }
  return offset;
}

// keyLineOffset finds the offset of a top-level key's line, so a diagnostic about
// that key underlines the key rather than the whole block.
export function keyLineOffset(block: string, key: string): number {
  const pattern = new RegExp(`^${key}\\s*:`, 'm');
  const match = pattern.exec(block);
  return match ? match.index : 0;
}

// frontmatterDiagnostics is pure: it takes the document and returns the problems
// the linter reports, so the rules are reviewable without an editor instance.
export function frontmatterDiagnostics(
  schema: FrontmatterSchema,
  doc: string,
  range: DocRange,
): Diagnostic[] {
  const block = doc.slice(range.from, range.to);
  const diagnostics: Diagnostic[] = [];

  let parsed: unknown;
  try {
    // The parser reports duplicate keys as an error of its own, so a repeated key
    // is caught here rather than by a second scan.
    parsed = parseYaml(block);
  } catch (err) {
    const error = err as Error & { linePos?: { line: number }[] };
    const line = error.linePos?.[0]?.line ?? 1;
    const from = range.from + lineStartOffset(block, line);
    diagnostics.push({
      from,
      to: Math.min(from + 1, range.to),
      severity: 'error',
      message: `Invalid YAML: ${error.message}`,
    });
    // A block that will not parse has no trustworthy keys to check.
    return diagnostics;
  }

  if (parsed === null || typeof parsed !== 'object') {
    return diagnostics;
  }

  const known = new Set(schema.keys.map((key) => key.name));
  for (const key of Object.keys(parsed as Record<string, unknown>)) {
    if (known.has(key)) continue;
    const from = range.from + keyLineOffset(block, key);
    diagnostics.push({
      from,
      to: Math.min(from + key.length, range.to),
      severity: 'info',
      // The engine is schema-agnostic: an unknown key is carried in ExtraMeta, so
      // this is information, never a reason to refuse a save.
      message: `"${key}" is not a known key. The engine keeps it as extra metadata.`,
    });
  }

  return diagnostics;
}
