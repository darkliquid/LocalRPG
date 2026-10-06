# Frontend and End-to-End Testing Design

**Date:** 2026-10-06
**Status:** Proposed
**Scope:** `frontend`, new `pkg/e2e`, `.github/workflows/ci.yml`, `mise.toml`, `AGENTS.md`

---

## 1. Problem

The frontend has no test runner. The only automated checks are three hand-written
`esbuild` + `assert` scripts (`checkTreeModel.mjs`, `checkEntityScaffold.mjs`,
`checkPlayerBundle.mjs`) and `tsc --noEmit`, wired into `mise run test:frontend`.
They cover two pure modules and one build artifact; nothing else in 22,000 lines of
TypeScript is exercised. A hook, a reducer, a parser, or a component can regress and
only `tsc` notices, and `tsc` checks types, not behaviour.

End-to-end tests exist but do not gate anything. Three Go tests in `pkg/gui`
(`delete_campaign_e2e_test.go`, `export_player_e2e_test.go`,
`portrait_clipping_test.go`) drive the real SPA through chromedp, and each one:

- rebuilds the chromedp allocator, the `httptest` server, and the console listener
  from scratch, so the same forty lines are copy-pasted three times;
- calls `t.Skip` when no browser is present, and **CI installs no browser**, so all
  three always skip there and guard nothing;
- captures no artifact on failure, so a red run in CI is undebuggable without a
  local reproduction.

There is also a second, separate browser path: `pkg/driver` runs declarative YAML
scenarios (`scenarios/*.yaml`) through `localrpg debug test-run`, used for the debug
overlay and screenshot capture. It asserts almost nothing and is not a test suite.

## 2. Goals

- A real frontend unit and component test runner: Vitest, with React Testing
  Library and jsdom.
- The three `.mjs` checks become Vitest tests; one runner, not two.
- A shared Go e2e harness: one browser wrapper and one fixture, so a browser test
  is a few lines of intent.
- The three existing e2e tests migrate onto the harness and keep their value.
- New e2e coverage for the launcher and the settings studio, the two surfaces a user
  meets first and where a broken render is most visible.
- The e2e suite runs for real in CI, in its own job with a browser installed, and
  uploads a screenshot and DOM dump when it fails.
- The browser tests stay out of the fast default Go suite, so `go test ./...` never
  needs a browser.

## 3. Non-goals

- Replacing `pkg/driver` and the YAML scenarios. They serve the debug overlay and
  screenshot capture; this spec does not touch them.
- Visual regression (pixel-diff) testing. The harness captures screenshots on
  failure as an artifact, not as an assertion.
- Coverage thresholds that fail a build. Coverage is reported, not enforced, in the
  first pass.
- Testing every component. The first pass establishes the harness and covers the
  logic-heavy modules and a representative set of components; breadth follows.
- A new e2e framework. chromedp is already a dependency and already works here.

## 4. Design

### 4.1 Frontend unit and component tests

**Runner:** Vitest, `jsdom` environment, React Testing Library. Vitest is the
Vite-native runner and reuses the existing transform pipeline, so a `.tsx` test needs
no extra build step.

**Config:** a dedicated `frontend/vitest.config.ts`, not the app's `vite.config.ts`.
The app config sets `build.outDir` into `pkg/gui/dist` and loads the Tailwind plugin;
neither belongs in a unit run. The test config loads only `@vitejs/plugin-react` and
declares:

```ts
test: {
  environment: 'jsdom',
  setupFiles: ['./src/test/setup.ts'],
  include: ['src/**/*.test.{ts,tsx}'],
  css: false,
}
```

**Setup:** `frontend/src/test/setup.ts` imports `@testing-library/jest-dom/vitest`
for the DOM matchers, stubs the browser APIs jsdom does not implement
(`ResizeObserver`, `window.matchMedia`), and calls RTL's `cleanup()` in `afterEach`.
Because the tests import `describe`/`it`/`expect` explicitly rather than enabling
Vitest globals, RTL's automatic cleanup does not run and the `afterEach` is required.

**Explicit imports, no globals.** The codebase is uniform on explicit imports and
`tsconfig.json` has `noUnusedLocals`/`noUnusedParameters`. Enabling `globals: true`
would need a `types` entry and hides where a symbol comes from; the tests import
from `vitest` directly.

**Scripts:** `test` (`vitest run`), `test:watch` (`vitest`), `test:coverage`
(`vitest run --coverage`). The `mise run test:frontend` task becomes
`npx tsc --noEmit && npx vitest run`, keeping the type check that already gates
unused imports.

**What the first pass covers:**

- Pure logic: `lib/slug`, `lib/generationError`, `utils/security`, `lib/project`,
  `lib/voiceProfiles`, `lib/turnStreamProcessor`, and the editor's
  `frontmatter`/`frontmatterLint`/`frontmatterSchema`. These are the modules a
  regression breaks silently.
- One hook (`hooks/useMountTransition`) to prove fake timers and effect testing
  work in the harness.
- A representative set of components with RTL (`SegmentAudioControls`, `LimitChip`,
  `MarkdownProse`) to prove a jsdom render, a user event, and an inline-grammar
  assertion work end to end.

### 4.2 Migrating the three `.mjs` checks

Each check becomes a Vitest test that imports the module directly, with no
`esbuild` subprocess and no `node:child_process`:

| Check | Becomes | Environment |
| --- | --- | --- |
| `checkTreeModel.mjs` | `src/components/treeModel.test.ts` | jsdom |
| `checkEntityScaffold.mjs` | `src/lib/entityScaffold.test.ts` | jsdom |
| `checkPlayerBundle.mjs` | `src/lib/playerBundle.test.ts` | node |

`checkPlayerBundle.mjs` asserts a property of the built artifact (the player bundle
contains no CodeMirror), not of source. It becomes a Vitest test with a
`// @vitest-environment node` directive that reads `pkg/gui/dist/player` and skips
when the bundle is absent, so `mise run test:frontend` still runs it after the build
and a source-only checkout is not a failure. The three `.mjs` files and their
`package.json` scripts are deleted. `scripts/lintDocs.mjs` stays; it is a linter, not
a test.

### 4.3 The e2e harness (`pkg/e2e`)

A new package, `pkg/e2e`, holds both the harness and the browser tests. It cannot
live in `pkg/gui`: the harness needs `gui.NewService`/`gui.NewServer`, so a `pkg/gui`
test importing it would be an import cycle. Every symbol it needs from `pkg/gui` is
already exported (`NewService`, `NewServer`, `AssetHandler`, `AssetFS`,
`ExportRequestDTO`, `SubscribeExportEvents`, `StartExport`), and `pkg/gui` does not
import the Wails Go package, so `pkg/e2e` builds without the native GTK/WebKit
headers.

The harness is split in two files, both untagged:

- **`harness.go`** - the browser wrapper. `NewBrowser(t, baseURL)` launches a
  headless Chrome, wires a console/exception listener under a mutex (the existing
  tests append from the CDP goroutine and read later without one, a latent race),
  and exposes intent-level methods: `Navigate`, `Click`, `Type`, `Text`, `BodyText`,
  `Eval`, `WaitVisible`, `WaitFor(needles...)`, `WaitForGone(needle)`, `Console`,
  `InstrumentAudio`, `Clips`, `Screenshot`. A single `run(actions...)` helper runs
  every action under the session deadline and reports a failure with the page
  console attached. `requireBrowser(t)` calls `driver.ChromePath()` and
  `driver.Available(ctx)` and skips when no usable browser exists, so the capability
  probe and a real run cannot disagree.
- **`fixture.go`** - the app under test. `NewFixture(t, configYAML)` writes a
  `config.yaml` to a `t.TempDir()`, constructs `gui.NewService`, wraps it in
  `httptest.NewServer(gui.NewServer(svc, gui.AssetHandler()))`, and registers both
  cleanups. Builder methods (`WriteSystem`, `WriteWorld`, `InitGame`, `WriteEntities`,
  `WriteHistory`) remove the fixture duplication the three tests share, and
  `Launch(t)` returns a `Browser` bound to the server URL.

Selectors use `chromedp.BySearch` throughout, which accepts CSS, XPath, and text, so
the wrapper needs no per-call selector-strategy parameter.

### 4.4 E2E coverage

Migrated (behaviour preserved, rewritten on the harness):

- `TestDeletingTheLastCampaignResetsLauncherStats`
- `TestExportedBundlePlaysTheTheatre`
- `TestMirroredPortraitRendersInsideItsRoundedBox`

New:

- **Launcher** - the hub renders the zero state; creating a campaign through the
  real modal produces a campaign in the hero stage.
- **Settings** - the studio opens from the dock and every tab (Paths, Providers, AI
  Agents, Media Engines, Batch Jobs, Preferences, Usage, Debug) renders its
  signature content, catching a lazy-chunk or tab-wiring failure; and editing a
  path, saving, and reloading the page round-trips the value.

### 4.5 Build tags and tasks

Browser test files carry `//go:build e2e`; `harness.go` and `fixture.go` do not
(a package whose every file is excluded by a build constraint fails `go build ./...`
with "build constraints exclude all Go files"). The default Go suite therefore never
compiles a browser test, and a machine without Chrome runs `go test ./...` fast.

New and changed mise tasks:

- `test:e2e` - `go test -v -count=1 -tags e2e ./pkg/e2e/...`.
- `test` - depends on `test:backend`, `test:frontend`, and `test:e2e`. The e2e task
  skips without a browser, so this stays runnable everywhere.
- `lint` - adds `go vet -tags e2e ./pkg/e2e/...`, since `go vet ./...` does not see
  tagged files.

### 4.6 CI

`ci.yml`'s `go` job keeps running `mise run test:backend` (fast, no browser). A new
`e2e` job:

1. checks out, activates mise, installs the native build dependencies (the same set
   the `go` job installs, so a transitive audio link is satisfied);
2. builds the embedded frontend, because the SPA and the player bundle are what the
   browser loads;
3. installs Chrome with `browser-actions/setup-chrome@v2` and exports the path;
4. runs `mise run test:e2e`;
5. uploads `test-results/` as an artifact, always, so a failure ships its screenshot
   and DOM dump.

### 4.7 Failure artifacts

On any harness failure, before the test aborts, the browser wrapper writes to
`$E2E_ARTIFACT_DIR/<test name>/` (default `test-results/`): `body.txt` (the page's
inner text), `dom.json` (a probe of the mounted structure), and `failure.png` (a
screenshot). The capture ignores its own errors, because it runs while a test is
already failing. This is what makes a CI failure actionable.

## 5. Behaviour

| Situation | Result |
| --- | --- |
| `mise run test:frontend` | type check passes, every Vitest test passes |
| a pure module regresses | its Vitest test fails with a diff |
| `go test ./...` on a machine with no browser | e2e tests are not compiled; the suite passes |
| `mise run test:e2e` with no browser | each browser test skips with a reason |
| `mise run test:e2e` with a browser | the suite runs for real |
| an e2e assertion fails | the failure names the step, prints the page console, and writes `body.txt`, `dom.json`, `failure.png` |
| a CI e2e failure | the `test-results/` artifact carries the screenshot and DOM dump |
| the player bundle is absent | the player-bundle Vitest test skips |

## 6. Testing

This spec's deliverable is test infrastructure, so its tests are the suite itself:

- The Vitest suite: migrated checks plus the new module, hook, and component tests.
- `pkg/e2e`: the migrated and new browser tests, which is the suite the harness
  exists to run.
- `go vet -tags e2e ./pkg/e2e/...` stays clean, and `npx tsc --noEmit` covers the
  new TypeScript.

A meta-check: the harness's own `requireBrowser` and `driver.Available` share the
allocator options, so a host that can probe a browser can run one.

## 7. Rollout

Additive, in three parts that can land separately:

1. Frontend Vitest harness and the migrated checks.
2. `pkg/e2e` harness and the migrated browser tests.
3. The new launcher and settings e2e tests, then the CI job and task wiring.

No production code changes. The three `.mjs` files and their scripts are removed; the
three `pkg/gui` e2e test files move to `pkg/e2e`. `AGENTS.md` is updated to describe
the new runner, the new package, the `e2e` build tag, and the new tasks.

## 8. Risks

- **Flaky browser tests.** CDP timing is the usual source. The harness polls
  (`WaitFor`, `WaitForGone`) rather than sleeping, and the existing tests already
  learned to wait for audio clips to settle instead of snapshotting. Any new flake is
  a test to fix, not to retry.
- **CI time.** A browser install plus a Chrome launch per test is slow. The e2e job
  is separate from the fast `go` job, so a slow e2e failure does not delay the unit
  signal, and the suite is small.
- **jsdom fidelity.** jsdom is not a browser: no layout, no audio. The unit tests
  assert logic and DOM structure, not pixels or sound; anything that needs real
  rendering belongs in the e2e suite.
- **Selector brittleness.** The e2e tests key on `aria-label` and visible text. When
  a label changes the test fails loudly, which is the intent, but it does couple the
  tests to copy. Prefer `aria-label` and stable roles over class names.
- **Artifact size.** PNGs on every failure could grow. The upload is per-run and
  bounded by the number of failing tests, and the retention is short.
- **A second runner.** Migrating the `.mjs` checks into Vitest is what keeps this
  from being two mechanisms; if a check cannot be expressed in Vitest, it belongs in
  a build task, not a parallel test script.
