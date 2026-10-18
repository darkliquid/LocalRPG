import fs from 'node:fs';
import path from 'node:path';
import { API } from 'typescript/unstable/sync';
import {
  type JsxElement,
  type Node,
  type SourceFile,
  isJsxAttribute,
  isJsxElement,
  isJsxExpression,
  isJsxFragment,
  isJsxText,
  visitEachChild,
} from 'typescript/unstable/ast';
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
function rendersText(node: JsxElement, source: SourceFile): boolean {
  let found = false;
  const visit = (child: Node): Node => {
    if (found) return child;
    if (isJsxText(child)) {
      if (child.getText(source).trim() !== '') found = true;
      return child;
    }
    if (isJsxExpression(child)) {
      found = true;
      return child;
    }
    if (isJsxElement(child) || isJsxFragment(child)) {
      visitEachChild(child, visit);
    }
    return child;
  };
  node.children.forEach(visit);
  return found;
}

function unnamedIconButtons(): string[] {
  const root = path.resolve(__dirname, '..');
  const found: string[] = [];
  const api = new API();

  try {
    const tsconfig = path.resolve(__dirname, '../../tsconfig.json');
    const snapshot = api.updateSnapshot({ openProjects: [tsconfig] });
    const proj = snapshot.getProject(tsconfig);

    for (const file of walk(root)) {
      const source = proj?.program.getSourceFile(file);
      if (!source) continue;

      const visit = (node: Node): Node => {
        if (isJsxElement(node) && node.openingElement.tagName.getText(source) === 'button') {
          const attributes = node.openingElement.attributes.properties;
          const has = (name: string) => attributes.some((a) => isJsxAttribute(a) && a.name.getText(source) === name);
          if (!rendersText(node, source) && !has('aria-label') && !has('aria-labelledby')) {
            const line = source.getLineAndCharacterOfPosition(node.openingElement.getStart(source)).line + 1;
            found.push(`${path.relative(root, file)}:${line}`);
          }
        }
        visitEachChild(node, visit);
        return node;
      };
      visit(source);
    }
  } finally {
    api.close();
  }

  return found;
}

describe('icon-only buttons', () => {
  it('all carry their own accessible name', () => {
    expect(unnamedIconButtons()).toEqual([]);
  });
});
