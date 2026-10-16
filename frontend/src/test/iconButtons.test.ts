import fs from 'node:fs';
import path from 'node:path';
import ts from 'typescript';
import { describe, expect, it } from 'vitest';

// This is a source scan rather than a render, because the defect it looks for is
// spread over 80-odd components and only some of them are rendered by a test.
// axe reports the same thing on a rendered surface (button-name), but it accepts a
// title as a name, which is exactly the habit this guards.

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full, out);
    else if (entry.name.endsWith('.tsx') && !entry.name.endsWith('.test.tsx')) out.push(full);
  }
  return out;
}

// rendersText reports whether an element subtree can render any text: a text node
// with content, or an expression, whose value is not knowable here. An icon is a
// self-closing element and renders none.
function rendersText(node: ts.JsxElement, source: ts.SourceFile): boolean {
  let found = false;
  const visit = (child: ts.Node) => {
    if (found) return;
    if (ts.isJsxText(child)) {
      if (child.getText(source).trim() !== '') found = true;
      return;
    }
    if (ts.isJsxExpression(child)) {
      found = true;
      return;
    }
    if (ts.isJsxElement(child) || ts.isJsxFragment(child)) {
      child.children.forEach(visit);
    }
  };
  node.children.forEach(visit);
  return found;
}

function unnamedIconButtons(): string[] {
  const root = path.resolve(__dirname, '..');
  const found: string[] = [];

  for (const file of walk(root)) {
    const source = ts.createSourceFile(
      file,
      fs.readFileSync(file, 'utf8'),
      ts.ScriptTarget.ESNext,
      true,
      ts.ScriptKind.TSX,
    );
    const visit = (node: ts.Node) => {
      if (ts.isJsxElement(node) && node.openingElement.tagName.getText(source) === 'button') {
        const attributes = node.openingElement.attributes.properties;
        const has = (name: string) => attributes.some((a) => a.name?.getText(source) === name);
        if (!rendersText(node, source) && !has('aria-label') && !has('aria-labelledby')) {
          const line = source.getLineAndCharacterOfPosition(node.openingElement.getStart(source)).line + 1;
          found.push(`${path.relative(root, file)}:${line}`);
        }
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
  }

  return found;
}

describe('icon-only buttons', () => {
  it('all carry their own accessible name', () => {
    expect(unnamedIconButtons()).toEqual([]);
  });
});