import axe, { type AxeResults } from 'axe-core';
import { expect } from 'vitest';

// formatViolations writes the sentence a failing test needs: which rule broke, how
// badly, and the element that broke it. axe's own report is built for a browser
// console, and a test failure is read in a terminal.
function formatViolations(violations: AxeResults['violations']): string {
  if (violations.length === 0) return '';
  const lines = violations.map((violation) => {
    const targets = violation.nodes.map((node) => node.target.join(' ')).join(', ');
    return `${violation.id} (${violation.impact ?? 'unknown'}): ${violation.help} -> ${targets}`;
  });
  return `Accessibility violations:\n${lines.join('\n')}`;
}

// expectNoViolations runs axe over what a test rendered and fails with the rules it
// broke. It works in jsdom, which has no layout: axe skips the rules that need one
// (colour contrast above all) and reports the rest.
export async function expectNoViolations(container: HTMLElement): Promise<void> {
  const results = await axe.run(container, {
    rules: {
      // jsdom has neither layout nor a canvas, so axe cannot measure contrast
      // here: the rule reports nothing and only costs a log line per render.
      // The icon-ligature check inside the name rules still asks for a canvas
      // and jsdom logs it, which is harmless.
      'color-contrast': { enabled: false },
    },
  });
  expect(results.violations, formatViolations(results.violations)).toEqual([]);
}
