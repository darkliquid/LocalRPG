// Prettier's settings, and the reasoning behind the two that are not defaults.
//
// This file is a .mjs rather than a .json purely so it can carry these comments;
// Prettier reads both.

export default {
  // The codebase already wraps near 120 and its 99th-percentile line is 154, so
  // 80 would rewrap almost everything and bury the real change in a diff. 120
  // normalises without rewriting the layout people already read.
  printWidth: 120,
  // 5499 single-quoted strings against 4293 double, so single is the existing
  // majority. JSX attribute strings stay double-quoted, which is Prettier's
  // jsxSingleQuote default and what the components already do.
  singleQuote: true,
  semi: true,
  tabWidth: 2,
  trailingComma: 'all',
  arrowParens: 'always',
  endOfLine: 'lf',
};
