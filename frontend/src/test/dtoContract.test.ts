import fs from 'node:fs';
import path from 'node:path';
import ts from 'typescript';
import { describe, expect, it } from 'vitest';

// The Go side owns this manifest: `go test ./pkg/gui -update-dto` rewrites it from
// the DTO structs. This test compares it against frontend/src/types.ts, so a
// renamed tag, a renamed property, an added field or a changed omitempty fails
// naming the type and the field, on either side.

interface ManifestField {
  json: string;
  go: string;
  optional: boolean;
}

interface ManifestType {
  fields: ManifestField[];
}

interface Manifest {
  types: Record<string, ManifestType>;
}

interface Declaration {
  name: string;
  members: Map<string, boolean>;
}

interface Exceptions {
  /** A Go DTO the client does not mirror, keyed by the Go name, with the reason. */
  notMirrored: Record<string, string>;
  /** A TypeScript property the server never sends, keyed `Type.prop`, with the reason. */
  tsOnly: Record<string, string>;
  /** A field TypeScript declares optional that Go always sends, keyed `GoType.json`, with the reason. */
  wireRequired: Record<string, string>;
}

const manifestPath = path.resolve(__dirname, '../../../pkg/gui/testdata/dto.json');
const declarationsPath = path.resolve(__dirname, '../types.ts');

// The decisions, each with its reason. Every one is checked against the manifest
// and the declarations below, so a reason cannot outlive what it explains.
const exceptions: Exceptions = {
  notMirrored: {},
  tsOnly: {
    'Turn.negotiations':
      'the client holds the counter-proposals it has made to a pending check and appends them to the turn it is showing; the server never sends the field back',
  },
  wireRequired: {
    'SegmentDTO.duration':
      'the client assembles segments as the reply streams, before the server has reported a duration; the server always sends one, and the client may not have it yet',
  },
};

function readManifest(): Manifest {
  return JSON.parse(fs.readFileSync(manifestPath, 'utf8')) as Manifest;
}

// readDeclarations parses types.ts with the TypeScript compiler, because that file
// is a hand-written mirror rather than a generated one: its unions, its view models
// and its comments are the reason this test exists instead of a generator.
function readDeclarations(): Map<string, Declaration> {
  const source = ts.createSourceFile(
    declarationsPath,
    fs.readFileSync(declarationsPath, 'utf8'),
    ts.ScriptTarget.ESNext,
    true,
    ts.ScriptKind.TS,
  );

  const declarations = new Map<string, Declaration>();
  for (const statement of source.statements) {
    if (!ts.isInterfaceDeclaration(statement)) continue;
    if (!statement.modifiers?.some((m) => m.kind === ts.SyntaxKind.ExportKeyword)) continue;

    const members = new Map<string, boolean>();
    for (const member of statement.members) {
      const name = member.name;
      if (!name) continue;
      if (ts.isIdentifier(name) || ts.isStringLiteral(name) || ts.isNumericLiteral(name)) {
        members.set(name.text, !!member.questionToken);
      }
    }
    declarations.set(statement.name.text, { name: statement.name.text, members });
  }
  return declarations;
}

function setEquals(a: Set<string>, b: Set<string>): boolean {
  if (a.size !== b.size) return false;
  for (const value of a) if (!b.has(value)) return false;
  return true;
}

// findMirror resolves a DTO's TypeScript counterpart by its property set, which is
// the proof of the pairing: a rename on either side breaks the match, so no
// hand-written table can fall behind or lie. An allowed extra property is removed
// before the comparison, which is what a recorded `tsOnly` exception buys.
function findMirror(
  jsonNames: Set<string>,
  declarations: Map<string, Declaration>,
  allowExtra: Set<string>,
): Declaration | undefined {
  const ordered = [...declarations.values()].sort((a, b) => a.name.localeCompare(b.name));
  for (const declaration of ordered) {
    const own = new Set([...declaration.members.keys()].filter((key) => !allowExtra.has(`${declaration.name}.${key}`)));
    if (setEquals(own, jsonNames)) return declaration;
  }
  return undefined;
}

// closestMirror is the diagnostic: the interface that shares the most properties,
// so a failure names the type the author probably meant.
function closestMirror(
  jsonNames: Set<string>,
  declarations: Map<string, Declaration>,
): { name: string; missing: string[]; extra: string[] } | undefined {
  let best: { name: string; missing: string[]; extra: string[]; overlap: number } | undefined;
  for (const declaration of declarations.values()) {
    const props = new Set(declaration.members.keys());
    const overlap = [...jsonNames].filter((name) => props.has(name)).length;
    if (!best || overlap > best.overlap) {
      best = {
        name: declaration.name,
        overlap,
        missing: [...jsonNames].filter((name) => !props.has(name)),
        extra: [...props].filter((name) => !jsonNames.has(name)),
      };
    }
  }
  return best;
}

// compare returns one sentence per difference. It takes the manifest and the
// declarations rather than reading them, so a test can run it over a seeded
// difference instead of only over the live files.
function compare(manifest: Manifest, declarations: Map<string, Declaration>, decided: Exceptions): string[] {
  const problems: string[] = [];
  const allowExtra = new Set(Object.keys(decided.tsOnly));
  const entries = Object.entries(manifest.types).sort(([a], [b]) => a.localeCompare(b));

  for (const [goName, definition] of entries) {
    if (decided.notMirrored[goName]) continue;

    const jsonNames = new Set(definition.fields.map((field) => field.json));
    const mirror = findMirror(jsonNames, declarations, allowExtra);
    if (!mirror) {
      const closest = closestMirror(jsonNames, declarations);
      const hint = closest
        ? ` closest is ${closest.name} (missing ${closest.missing.join(', ') || 'none'}; extra ${closest.extra.join(', ') || 'none'})`
        : ' no interface in types.ts shares any of its properties';
      problems.push(`${goName}: no interface in types.ts mirrors its fields;${hint}`);
      continue;
    }

    for (const field of definition.fields) {
      const optional = mirror.members.get(field.json);
      if (optional === undefined) {
        problems.push(`${goName}.${field.json}: ${mirror.name} does not declare it, so the client reads undefined`);
        continue;
      }
      if (field.optional && !optional) {
        problems.push(`${goName}.${field.json}: Go may omit it, but ${mirror.name} requires it`);
      }
      if (!field.optional && optional && !decided.wireRequired[`${goName}.${field.json}`]) {
        problems.push(`${goName}.${field.json}: Go always sends it, but ${mirror.name} declares it optional`);
      }
    }

    for (const property of mirror.members.keys()) {
      if (jsonNames.has(property)) continue;
      if (decided.tsOnly[`${mirror.name}.${property}`]) continue;
      problems.push(`${mirror.name}.${property}: the server never sends it and no reason is recorded`);
    }
  }

  return problems;
}

// staleExceptions reports an exception that no longer explains anything, so the
// lists cannot become a dumping ground.
function staleExceptions(manifest: Manifest, declarations: Map<string, Declaration>, decided: Exceptions): string[] {
  const stale: string[] = [];

  for (const [goName, reason] of Object.entries(decided.notMirrored)) {
    if (!manifest.types[goName]) stale.push(`notMirrored: ${goName} is not in the manifest (${reason})`);
  }

  for (const [key, reason] of Object.entries(decided.tsOnly)) {
    const [type, property] = key.split('.');
    const declaration = declarations.get(type);
    if (!declaration) {
      stale.push(`tsOnly: ${type} is not declared in types.ts (${reason})`);
    } else if (!declaration.members.has(property)) {
      stale.push(`tsOnly: ${type} has no property ${property} (${reason})`);
    }
  }

  for (const [key, reason] of Object.entries(decided.wireRequired)) {
    const [type, json] = key.split('.');
    const definition = manifest.types[type];
    if (!definition) {
      stale.push(`wireRequired: ${type} is not in the manifest (${reason})`);
    } else if (!definition.fields.some((field) => field.json === json)) {
      stale.push(`wireRequired: ${type} has no field ${json} (${reason})`);
    }
  }

  return stale;
}

describe('the Go and TypeScript contract', () => {
  const manifest = readManifest();
  const declarations = readDeclarations();

  it('agrees on every field name and its optionality', () => {
    expect(compare(manifest, declarations, exceptions)).toEqual([]);
  });

  it('holds the whole DTO surface, so a new one cannot be forgotten', () => {
    expect(Object.keys(manifest.types).length).toBeGreaterThan(100);
  });

  it('reports a seeded difference rather than only agreeing with the live files', () => {
    const seeded: Manifest = structuredClone(manifest);
    const first = Object.keys(seeded.types)[0];
    seeded.types[first].fields.push({ json: 'a_field_the_server_never_sends', go: 'Typo', optional: false });

    expect(compare(seeded, declarations, exceptions).join('\n')).toContain('a_field_the_server_never_sends');
  });

  it('reports a seeded optionality difference', () => {
    const seeded: Manifest = structuredClone(manifest);
    const [goName, definition] = Object.entries(seeded.types).find(([, d]) => d.fields.some((f) => !f.optional))!;
    const field = definition.fields.find((f) => !f.optional)!;
    field.optional = true;

    expect(compare(seeded, declarations, exceptions).join('\n')).toContain(`${goName}.${field.json}`);
  });

  it('has no exception that outlives what it explains', () => {
    expect(staleExceptions(manifest, declarations, exceptions)).toEqual([]);
  });
});
