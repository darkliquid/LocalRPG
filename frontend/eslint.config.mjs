// ESLint, flat config.
//
// The aim matches the Go side's: catch what a type checker cannot. tsc already
// enforces types, unused locals and unused parameters, so the value here is the
// rules it has no opinion on: hook correctness, accessibility, and the
// correctness rules in the recommended sets.
//
// Rules are either errors, which fail CI, or warnings, which print on every run
// but do not. That split is deliberate and is the shape `mise run lint:prose`
// already uses: the error level gates a build, and the warning level keeps known,
// tracked work visible without freezing unrelated changes. Each warning group
// below names the proposal that fixes it and, with it, promotes it to an error.
// Nothing is suppressed per line.

import js from '@eslint/js';
import prettier from 'eslint-config-prettier';
import jsxA11y from 'eslint-plugin-jsx-a11y';
import reactHooks from 'eslint-plugin-react-hooks';
import globals from 'globals';
import tseslint from 'typescript-eslint';

// react-hooks 7 ships its flat configs under `configs.flat`; the bare
// `configs.recommended` is the eslintrc shape, which flat config rejects.
const hooks = reactHooks.configs.flat['recommended-latest'];

// Rules-of-hooks and exhaustive-deps predate the React Compiler and catch real
// bugs, so they stay errors and are fixed here. The rest of recommended-latest is
// compiler readiness: purity, ref access during render, setState inside an
// effect. Those describe restructuring FE-3 does anyway (the state layer and the
// App.tsx split) and FE-2 does for the modals, so they warn until it lands rather
// than blocking every other frontend change.
const hooksErrors = new Set(['react-hooks/rules-of-hooks', 'react-hooks/exhaustive-deps']);
const hooksConfig = {
  ...hooks,
  rules: Object.fromEntries(
    Object.entries(hooks.rules).map(([rule]) => [rule, hooksErrors.has(rule) ? 'error' : 'warn']),
  ),
};

// Every recommended accessibility rule stays on, at the warning level: the 85
// unassociated labels and the 41 clickable divs without a keyboard handler are
// real gaps, they are recorded as finding F-8, and FE-6 is the proposal that
// fixes them and turns these back into errors.
//
// The rules the recommended set leaves `off` stay off. Mapping every key to
// `warn` turns those on as well, and two of them (`label-has-for`, superseded by
// `label-has-associated-control`, and `control-has-associated-label`, meant for
// custom controls) then contributed 354 findings of their own, which is the sort
// of noise that makes a warning list worth ignoring.
const a11yConfig = {
  ...jsxA11y.flatConfigs.recommended,
  rules: Object.fromEntries(
    Object.entries(jsxA11y.flatConfigs.recommended.rules).map(([rule, level]) => [
      rule,
      level === 'off' ? 'off' : 'warn',
    ]),
  ),
};

export default tseslint.config(
  {
    ignores: ['dist/**', 'coverage/**', 'node_modules/**', '../pkg/gui/dist/**'],
  },

  js.configs.recommended,
  ...tseslint.configs.recommended,
  hooksConfig,
  a11yConfig,
  // Last, so the formatting rules it disables cannot be re-enabled by anything
  // above it. Prettier owns formatting; ESLint owns correctness.
  prettier,

  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      globals: { ...globals.browser },
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      // FE-8 cleared this: the frontend has no explicit any left, so it fails a
      // build rather than joining a list. The test block below still turns it
      // off, where a fixture may deliberately hold a shape it does not use.
      '@typescript-eslint/no-explicit-any': 'error',
      // This reports the same unlabelled form controls as
      // label-has-associated-control, from the control's side instead of the
      // label's: the two together gave 258 findings for one class of defect
      // (85 + 173). label-has-associated-control is the standard rule for real
      // form elements and names the site the reader can fix, so the control-side
      // duplicate is off. FE-6 revisits it when it works the accessibility
      // findings, since the rule is meant for custom controls rather than for
      // input and textarea.
      'jsx-a11y/control-has-associated-label': 'off',
    },
  },

  {
    files: ['**/*.test.{ts,tsx}', 'src/test/**/*.{ts,tsx}'],
    languageOptions: { globals: { ...globals.browser, ...globals.node } },
    rules: {
      // A test may assert on a value it deliberately does not use, and the
      // harness stubs browser APIs that jsdom lacks.
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },

  {
    files: ['*.mjs', '*.ts'],
    languageOptions: { globals: { ...globals.node } },
  },
);
