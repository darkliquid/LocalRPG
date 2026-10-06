# Frontend and End-to-End Testing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the frontend a real unit and component test runner (Vitest + React Testing Library + jsdom), replace the three hand-rolled `.mjs` checks with it, and give the browser suite a shared Go harness that runs for real in CI.

**Architecture:** A dedicated `frontend/vitest.config.ts` runs tests in jsdom with explicit imports and no globals. A new `pkg/e2e` package holds one chromedp browser wrapper (`harness.go`) and one app fixture (`fixture.go`); the browser tests carry a `//go:build e2e` tag so the default `go test ./...` never needs a browser. A new CI job installs Chrome, runs `mise run test:e2e`, and uploads failure artifacts.

**Tech Stack:** Vitest 5, jsdom 30, React Testing Library 16, `@testing-library/jest-dom` 7, Go 1.27, chromedp 0.16 (already a dependency), `pkg/driver` for the browser capability probe.

**Spec:** `docs/superpowers/specs/2026-10-06-frontend-and-e2e-testing-design.md`

## Global Constraints

- Go tests use the standard library only (`testing`, `t.TempDir()`); no testify.
- Use `any`, not interface{}`, in Go.
- TypeScript is `strict` with `noUnusedLocals` and `noUnusedParameters`; `tsc --noEmit` must stay clean.
- Vitest tests import `describe`/`it`/`expect` explicitly; no `globals: true`.
- Browser test files start with `//go:build e2e`; `harness.go` and `fixture.go` do not.
- Browser tests skip, never fail, when no usable browser is present.
- `pkg/e2e` imports only exported `pkg/gui` symbols and must not import the Wails Go package.
- No new runtime dependencies; the new packages are dev dependencies only.
- Commits are Conventional Commits with a scope (`feat(frontend):`, `test(e2e):`, `ci:`, `docs:`).

## File Map

**Create**

- `frontend/vitest.config.ts` - the test runner config.
- `frontend/src/test/setup.ts` - jsdom matchers, API stubs, RTL cleanup.
- `frontend/src/test/harness.test.ts` - a smoke test proving the runner is wired.
- `frontend/src/components/treeModel.test.ts` - migrated from `checkTreeModel.mjs`.
- `frontend/src/lib/entityScaffold.test.ts` - migrated from `checkEntityScaffold.mjs`.
- `frontend/src/lib/playerBundle.test.ts` - migrated from `checkPlayerBundle.mjs`.
- `frontend/src/lib/slug.test.ts`, `generationError.test.ts`, `voiceProfiles.test.ts`, `project.test.ts`
- `frontend/src/utils/security.test.ts`
- `frontend/src/lib/turnStreamProcessor.test.ts`
- `frontend/src/components/editor/frontmatter.test.ts`, `frontmatterLint.test.ts`, `frontmatterSchema.test.ts`
- `frontend/src/hooks/useMountTransition.test.tsx`
- `frontend/src/components/SegmentAudioControls.test.tsx`, `LimitChip.test.tsx`, `MarkdownProse.test.tsx`
- `pkg/e2e/harness.go`, `pkg/e2e/fixture.go`
- `pkg/e2e/smoke_test.go`, `launcher_test.go`, `settings_test.go`, `export_test.go`, `portrait_test.go`

**Modify**

- `frontend/package.json` - dev dependencies and scripts.
- `mise.toml` - `test:frontend`, new `test:e2e`, `test`, `lint`.
- `.github/workflows/ci.yml` - a new `e2e` job.
- `.gitignore` - ignore `test-results/`.
- `AGENTS.md` - document the runner, the package, the tag, and the tasks.

**Delete**

- `frontend/scripts/checkTreeModel.mjs`, `checkEntityScaffold.mjs`, `checkPlayerBundle.mjs`
- `pkg/gui/delete_campaign_e2e_test.go`, `pkg/gui/export_player_e2e_test.go`, `pkg/gui/portrait_clipping_test.go`

---

# Part A - Frontend unit and component tests

### Task 1: Bootstrap the Vitest harness

**Files:**

- Create: `frontend/vitest.config.ts`
- Create: `frontend/src/test/setup.ts`
- Create: `frontend/src/test/harness.test.ts`
- Modify: `frontend/package.json`
- Modify: `mise.toml`

**Interfaces:**

- Produces: `mise run test:frontend` runs `npx tsc --noEmit && npx vitest run`; a `.test.ts`/`.test.tsx` under `frontend/src` is discovered and runs in jsdom.

- [ ] **Step 1: Add the dev dependencies**

Run:

```bash
cd frontend && npm install --save-dev \
  vitest@^5.0.3 \
  jsdom@^30.1.2 \
  @vitest/coverage-v8@^5.0.3 \
  @testing-library/react@^16.3.3 \
  @testing-library/dom@^10.4.2 \
  @testing-library/user-event@^14.6.7 \
  @testing-library/jest-dom@^7.0.1
```

- [ ] **Step 2: Add the test scripts**

In `frontend/package.json`, add to `scripts` (keep `dev`, `build`, `preview`, `lint:docs`):

```json
"test": "vitest run",
"test:watch": "vitest",
"test:coverage": "vitest run --coverage",
```

- [ ] **Step 3: Write the config**

Create `frontend/vitest.config.ts`:

```ts
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// The app's own vite.config.ts sets an outDir inside pkg/gui and loads the
// Tailwind plugin; neither belongs in a unit run, so the test config is separate
// and loads only what a jsdom render needs.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    css: false,
  },
});
```

- [ ] **Step 4: Write the setup file**

Create `frontend/src/test/setup.ts`:

```ts
import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

// jsdom implements neither of these, and the drawers and the tree construct one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver =
  ResizeObserverStub as unknown as typeof ResizeObserver;

if (!window.matchMedia) {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
    dispatchEvent() {
      return false;
    },
  })) as unknown as typeof window.matchMedia;
}

// Tests import describe/it/expect explicitly, so RTL's automatic cleanup does not
// run; without this a second render in a file sees the first one's DOM.
afterEach(() => {
  cleanup();
});
```

- [ ] **Step 5: Write the smoke test**

Create `frontend/src/test/harness.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';

describe('the test harness', () => {
  it('runs in a DOM with the jest-dom matchers installed', () => {
    document.body.innerHTML = '<p>ready</p>';
    expect(screen.getByText('ready')).toBeInTheDocument();
  });

  it('renders a component', () => {
    render(<span>hello</span>);
    expect(screen.getByText('hello')).toBeVisible();
  });
});
```

Note: the second test uses JSX, so name the file `harness.test.tsx` instead, or drop the JSX test. Name it `frontend/src/test/harness.test.tsx` and keep both tests.

- [ ] **Step 6: Run it and verify it passes**

Run: `cd frontend && npx vitest run src/test/harness.test.tsx`
Expected: 2 passing tests.

- [ ] **Step 7: Update the mise task**

In `mise.toml`, change the `test:frontend` task body to:

```toml
[tasks."test:frontend"]
description = "Run TypeScript type checks and the frontend test suite"
dir = "frontend"
depends = ["build:frontend"]
run = """
npx tsc --noEmit
npx vitest run
"""
```

- [ ] **Step 8: Run the task**

Run: `mise run test:frontend`
Expected: `tsc` is clean and the harness test passes.

- [ ] **Step 9: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/vitest.config.ts frontend/src/test mise.toml
git commit -m "test(frontend): add a Vitest harness for unit and component tests"
```

---

### Task 2: Migrate the tree model check

**Files:**

- Create: `frontend/src/components/treeModel.test.ts`
- Delete: `frontend/scripts/checkTreeModel.mjs`
- Modify: `frontend/package.json`

**Interfaces:**

- Consumes: `buildIndex`, `childrenOf`, `countNotesUnder`, `folderItemId`, `leafOf`, `movesForChildren`, `noteItemId`, `parentOf` from `./treeModel`.
- Produces: the tree model assertions as a Vitest test.

- [ ] **Step 1: Write the test**

Create `frontend/src/components/treeModel.test.ts`, porting every `check(...)` from `checkTreeModel.mjs`. Replace the `folders(paths)` helper's inline types with the same shape, and import the model directly:

```ts
import { describe, expect, it } from "vitest";
import {
  buildIndex,
  childrenOf,
  countNotesUnder,
  folderItemId,
  leafOf,
  movesForChildren,
  noteItemId,
  parentOf,
} from "./treeModel";

// Mirror the shape the server sends: a list of top-level folders, nested below.
const folders = (paths: string[]): TreeFolder[] => {
  const root: TreeFolder[] = [];
  for (const path of paths) {
    let level = root;
    let prefix = "";
    for (const segment of path.split("/")) {
      prefix = prefix ? `${prefix}/${segment}` : segment;
      let node = level.find((candidate) => candidate.path === prefix);
      if (!node) {
        node = { path: prefix, name: segment, children: [] };
        level.push(node);
      }
      level = node.children;
    }
  }
  return root;
};

const entities = [
  { id: "a", name: "A", folder: "" },
  { id: "b", name: "B", folder: "factions" },
  { id: "c", name: "C", folder: "factions/orders" },
];
```

Port each assertion as an `it`. For example:

```ts
describe("the tree model", () => {
  it("lists top-level folders as children of the root", () => {
    const tree = folders(["factions", "factions/orders"]);
    expect(childrenOf(tree, entities, "")).toEqual([
      folderItemId("factions"),
      noteItemId("a"),
    ]);
  });

  it("counts descendants", () => {
    const tree = folders(["factions", "factions/orders"]);
    expect(countNotesUnder(tree, entities, "factions")).toBe(2);
  });

  it("reports path helpers", () => {
    expect([leafOf("factions/orders"), parentOf("factions/orders")]).toEqual([
      "orders",
      "factions",
    ]);
  });
});
```

Port the remaining assertions (new top-level folder appears; new folder is in the index; a new folder starts empty; a new subfolder appears under its parent; a note dropped into a folder moves there; a note drop moves no folders; a drop that changes nothing moves nothing; a folder dropped into a folder moves there; a folder already at the root needs no move; a note dropped on the root comes back out) one `it` each, copying the inputs and expectations verbatim from the script.

Add the `TreeFolder` import at the top if the model exports it; otherwise inline the object type as `{ path: string; name: string; children: TreeFolder[] }` with a local `type TreeFolder = { path: string; name: string; children: TreeFolder[] }`.

- [ ] **Step 2: Run the new test**

Run: `cd frontend && npx vitest run src/components/treeModel.test.ts`
Expected: every ported assertion passes (this is a translation of a passing check).

- [ ] **Step 3: Delete the old script and its package script**

Run:

```bash
rm frontend/scripts/checkTreeModel.mjs
```

Remove `"check:tree-model": "node scripts/checkTreeModel.mjs"` from `frontend/package.json` `scripts`.

- [ ] **Step 4: Run the whole frontend task**

Run: `mise run test:frontend`
Expected: `tsc` clean, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/treeModel.test.ts frontend/scripts frontend/package.json
git commit -m "test(frontend): move the tree model check into Vitest"
```

---

### Task 3: Migrate the entity scaffold check

**Files:**

- Create: `frontend/src/lib/entityScaffold.test.ts`
- Delete: `frontend/scripts/checkEntityScaffold.mjs`
- Modify: `frontend/package.json`

**Interfaces:**

- Consumes: `buildEntityMarkdown` from `./entityScaffold`; `parse` from `yaml`.

- [ ] **Step 1: Write the test**

Create `frontend/src/lib/entityScaffold.test.ts`. Copy the `key`, `baseKeys`, `characterKeys`, `locationKeys`, and `catalog` fixtures verbatim from `checkEntityScaffold.mjs`, import `buildEntityMarkdown` directly, and port each `check(...)` as an `it`:

```ts
import { describe, expect, it } from "vitest";
import { parse as parseYaml } from "yaml";
import { buildEntityMarkdown } from "./entityScaffold";

// key(), baseKeys, characterKeys, locationKeys, catalog copied from the old script.

const frontmatterOf = (markdown: string): string => {
  const end = markdown.indexOf("\n---", 3);
  return markdown.slice(4, end);
};

describe("the entity scaffold", () => {
  it("writes required keys with their values", () => {
    const fm = frontmatterOf(
      buildEntityMarkdown(catalog, {
        id: "lady-evelyn",
        name: "Lady Evelyn Vance",
        type: "character",
      }),
    );
    expect(fm).toContain('id: "lady-evelyn"');
    expect(fm).toContain('name: "Lady Evelyn Vance"');
    expect(fm).toContain('type: "character"');
  });

  it("generates frontmatter that parses as YAML", () => {
    const fm = frontmatterOf(
      buildEntityMarkdown(catalog, {
        id: "lady-evelyn",
        name: "Lady Evelyn Vance",
        type: "character",
      }),
    );
    expect(parseYaml(fm)).toMatchObject({
      id: "lady-evelyn",
      type: "character",
    });
  });
});
```

Port the remaining assertions (empty list for tags; empty map for state; empty string for a free-text key; body starts with the H1; every key is preceded by its comment; a character without a voice still gets the documented block; a chosen voice is written into the block; a location offers appearance; a location does not offer voice/gender/age; an unknown type falls back to the base key set).

- [ ] **Step 2: Run the new test**

Run: `cd frontend && npx vitest run src/lib/entityScaffold.test.ts`
Expected: all assertions pass.

- [ ] **Step 3: Delete the old script**

Run:

```bash
rm frontend/scripts/checkEntityScaffold.mjs
```

Remove `"check:entity-scaffold": "node scripts/checkEntityScaffold.mjs"` from `frontend/package.json` `scripts`.

- [ ] **Step 4: Run the frontend task**

Run: `mise run test:frontend`
Expected: `tsc` clean, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/entityScaffold.test.ts frontend/scripts frontend/package.json
git commit -m "test(frontend): move the entity scaffold check into Vitest"
```

---

### Task 4: Migrate the player bundle check

**Files:**

- Create: `frontend/src/lib/playerBundle.test.ts`
- Delete: `frontend/scripts/checkPlayerBundle.mjs`
- Modify: `frontend/package.json`

**Interfaces:**

- Produces: a node-environment test that fails when the built player bundle contains CodeMirror, and skips when the bundle is absent.

- [ ] **Step 1: Write the test**

Create `frontend/src/lib/playerBundle.test.ts`:

```ts
// @vitest-environment node
import { describe, expect, it } from "vitest";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

// The player is built on its own and inlined into a single page, so it must stay
// small and offline. The app editor must never be reachable from it. The bundle
// is a build artifact, so the test skips when the frontend has not been built.
const playerDir = fileURLToPath(
  new URL("../../../pkg/gui/dist/player", import.meta.url),
);

const walk = (dir: string): string[] =>
  readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    return statSync(full).isDirectory() ? walk(full) : [full];
  });

describe("the player bundle", () => {
  it.skipIf(!existsSync(playerDir))("does not contain the editor", () => {
    const offenders = walk(playerDir).filter((file) =>
      /codemirror/i.test(readFileSync(file, "utf8")),
    );
    expect(offenders).toEqual([]);
  });
});
```

- [ ] **Step 2: Run the new test**

Run: `cd frontend && npx vitest run src/lib/playerBundle.test.ts`
Expected: passes when the player is built (run `mise run build:frontend` first), skips when it is not.

- [ ] **Step 3: Delete the old script**

Run:

```bash
rm frontend/scripts/checkPlayerBundle.mjs
```

Remove `"check:player-bundle": "node scripts/checkPlayerBundle.mjs"` from `frontend/package.json` `scripts`.

- [ ] **Step 4: Run the frontend task**

Run: `mise run test:frontend`
Expected: `tsc` clean, all tests pass (the player bundle is built by the task's `build:frontend` dependency).

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/playerBundle.test.ts frontend/scripts frontend/package.json
git commit -m "test(frontend): move the player bundle check into Vitest"
```

---

### Task 5: Unit tests for the pure modules

**Files:**

- Create: `frontend/src/lib/slug.test.ts`
- Create: `frontend/src/lib/generationError.test.ts`
- Create: `frontend/src/utils/security.test.ts`
- Create: `frontend/src/lib/voiceProfiles.test.ts`
- Create: `frontend/src/lib/project.test.ts`

**Interfaces:**

- Consumes: `slugify` from `../lib/slug`; `formatGenerationError`, `generationAttemptLines` from `../lib/generationError`; `safeImagePreview` from `../utils/security`; the exported helpers of `../lib/voiceProfiles`; `PROJECT_NAME`, `PROJECT_REPO_URL`, `PROJECT_ISSUES_URL` from `../lib/project`.

- [ ] **Step 1: Write the slug tests**

Create `frontend/src/lib/slug.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { slugify } from "./slug";

describe("slugify", () => {
  it("lowercases and hyphenates", () => {
    expect(slugify("Lady Evelyn Vance")).toBe("lady-evelyn-vance");
  });
  it("collapses runs of separators", () => {
    expect(slugify("the   quay--district")).toBe("the-quay-district");
  });
  it("drops leading and trailing separators", () => {
    expect(slugify("  _The Quay_ ")).toBe("the-quay");
  });
  it("strips punctuation", () => {
    expect(slugify("Sean O'Malley!")).toBe("sean-omalley");
  });
  it("keeps digits", () => {
    expect(slugify("Sector 7-G")).toBe("sector-7-g");
  });
});
```

- [ ] **Step 2: Run it**

Run: `cd frontend && npx vitest run src/lib/slug.test.ts`
Expected: pass. If `slugify('Sector 7-G')` returns something else, read `src/lib/slug.ts` and correct the expectation to the implemented behaviour (the function is the source of truth; it mirrors `entity.Slugify`).

- [ ] **Step 3: Write the generation error tests**

Create `frontend/src/lib/generationError.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import {
  formatGenerationError,
  generationAttemptLines,
} from "./generationError";

describe("formatGenerationError", () => {
  it("leads with the message and suffixes the code", () => {
    expect(
      formatGenerationError({ code: "rate_limited", message: "Slow down" }),
    ).toBe("Slow down (rate_limited)");
  });
  it("falls back to the code when there is no message", () => {
    expect(formatGenerationError({ code: "timeout" })).toBe("timeout");
  });
});

describe("generationAttemptLines", () => {
  it("renders one line per attempt with an optional detail", () => {
    expect(
      generationAttemptLines({
        code: "failed",
        attempts: [
          { role: "gm", provider: "openai", code: "timeout" },
          { role: "gm", provider: "ollama", code: "ok", detail: "recovered" },
        ],
      }),
    ).toEqual(["gm/openai [timeout]", "gm/ollama [ok]: recovered"]);
  });
  it("returns an empty list when there are no attempts", () => {
    expect(generationAttemptLines({ code: "failed" })).toEqual([]);
  });
});
```

Read `frontend/src/types.ts` for the exact `GenerationFailure`/attempt field names before running; correct the fixture objects if a field name differs.

- [ ] **Step 4: Write the security tests**

Create `frontend/src/utils/security.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { safeImagePreview } from "./security";

describe("safeImagePreview", () => {
  it("accepts the allowed schemes", () => {
    expect(safeImagePreview("blob:http://localhost/abc")).toBe(
      "blob:http://localhost/abc",
    );
    expect(safeImagePreview("/api/game/1/asset")).toBe("/api/game/1/asset");
    expect(safeImagePreview("data:image/png;base64,AAAA")).toBe(
      "data:image/png;base64,AAAA",
    );
  });
  it("rejects anything else", () => {
    expect(safeImagePreview("javascript:alert(1)")).toBeUndefined();
    expect(safeImagePreview("https://evil.example/x.png")).toBeUndefined();
    expect(safeImagePreview("")).toBeUndefined();
    expect(safeImagePreview(null)).toBeUndefined();
  });
});
```

- [ ] **Step 5: Write the voice profile and project tests**

Run `grep -n 'export' frontend/src/lib/voiceProfiles.ts` first, then create `frontend/src/lib/voiceProfiles.test.ts` covering the exported helpers with the cases you find (at minimum: the function that resolves a profile by ID returns the match and a defined fallback for an unknown ID). Create `frontend/src/lib/project.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { PROJECT_ISSUES_URL, PROJECT_REPO_URL } from "./project";

describe("project links", () => {
  it("derives the issues URL from the repository URL", () => {
    expect(PROJECT_ISSUES_URL).toBe(`${PROJECT_REPO_URL}/issues`);
  });
});
```

- [ ] **Step 6: Run the new tests**

Run: `cd frontend && npx vitest run src/lib/slug.test.ts src/lib/generationError.test.ts src/utils/security.test.ts src/lib/voiceProfiles.test.ts src/lib/project.test.ts`
Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/slug.test.ts frontend/src/lib/generationError.test.ts frontend/src/utils/security.test.ts frontend/src/lib/voiceProfiles.test.ts frontend/src/lib/project.test.ts
git commit -m "test(frontend): cover the pure library modules"
```

---

### Task 6: Test the turn stream processor

**Files:**

- Create: `frontend/src/lib/turnStreamProcessor.test.ts`

**Interfaces:**

- Consumes: `TurnStreamProcessor` from `./turnStreamProcessor`.

- [ ] **Step 1: Write the tests**

Create `frontend/src/lib/turnStreamProcessor.test.ts`. The class's contract, from its doc comment: control lines starting with `@` never surface; `>` lines become speech segments; narration streams character by character; a canonical `feedSegment` enriches a live speech segment with a resolved speaker ID.

```ts
import { describe, expect, it } from "vitest";
import { TurnStreamProcessor } from "./turnStreamProcessor";

describe("TurnStreamProcessor", () => {
  it("filters a control line out entirely", () => {
    const p = new TurnStreamProcessor();
    p.feedChunk("@roll might\nYou shove the door.");
    const segments = p.getSegments();
    expect(segments.some((s) => s.text.includes("@roll"))).toBe(false);
    expect(segments.some((s) => s.text.includes("You shove the door."))).toBe(
      true,
    );
  });

  it("parses a blockquoted speech line into a speaker and text", () => {
    const p = new TurnStreamProcessor();
    p.feedChunk('> Garrick: "Keep moving."\n');
    expect(p.getSegments()).toContainEqual({
      kind: "speech",
      speaker: "Garrick",
      text: "Keep moving.",
    });
  });

  it("streams narration a character at a time", () => {
    const p = new TurnStreamProcessor();
    p.feedChunk("The quay");
    expect(p.getSegments()).toContainEqual({
      kind: "narration",
      text: "The quay",
    });
    p.feedChunk(" is quiet.");
    expect(p.getSegments()).toContainEqual({
      kind: "narration",
      text: "The quay is quiet.",
    });
  });

  it("enriches a live speech segment with a canonical speaker id", () => {
    const p = new TurnStreamProcessor();
    p.feedChunk('> Garrick: "Keep moving."\n');
    p.feedSegment({
      kind: "speech",
      speaker: "Garrick",
      speaker_id: "garrick",
      text: "Keep moving.",
    });
    const speech = p.getSegments().find((s) => s.kind === "speech");
    expect(speech?.speaker_id).toBe("garrick");
  });

  it("joins wrapped narration lines into one paragraph", () => {
    const p = new TurnStreamProcessor();
    p.feedChunk("The quay is quiet\nand the tide is out.\n");
    expect(p.getSegments()).toContainEqual({
      kind: "narration",
      text: "The quay is quiet\nand the tide is out.",
    });
  });
});
```

- [ ] **Step 2: Run them**

Run: `cd frontend && npx vitest run src/lib/turnStreamProcessor.test.ts`
Expected: pass. If a case disagrees, read the processor and align the expectation with its documented contract, and note the difference in the commit body.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/lib/turnStreamProcessor.test.ts
git commit -m "test(frontend): cover the streaming turn parser"
```

---

### Task 7: Test the editor frontmatter modules

**Files:**

- Create: `frontend/src/components/editor/frontmatter.test.ts`
- Create: `frontend/src/components/editor/frontmatterLint.test.ts`
- Create: `frontend/src/components/editor/frontmatterSchema.test.ts`

**Interfaces:**

- Consumes: the exported functions of `frontmatter.ts`, `frontmatterLint.ts`, `frontmatterSchema.ts`.

- [ ] **Step 1: Read the modules' exports**

Run:

```bash
grep -n 'export ' frontend/src/components/editor/frontmatter.ts frontend/src/components/editor/frontmatterLint.ts frontend/src/components/editor/frontmatterSchema.ts
```

- [ ] **Step 2: Write the frontmatter round-trip test**

Create `frontend/src/components/editor/frontmatter.test.ts`. Assert the parse/serialize round trip and the error path:

```ts
import { describe, expect, it } from "vitest";
import { parseFrontmatter, serializeFrontmatter } from "./frontmatter";

describe("frontmatter", () => {
  it("round-trips a document", () => {
    const source = "---\nid: garrick\nname: Garrick\n---\nA grim guard.\n";
    const parsed = parseFrontmatter(source);
    expect(parsed.frontmatter.id).toBe("garrick");
    expect(serializeFrontmatter(parsed)).toBe(source);
  });

  it("reports a document with no frontmatter", () => {
    const parsed = parseFrontmatter("Just prose.\n");
    expect(parsed.frontmatter).toEqual({});
  });
});
```

Correct the imported names and the parsed shape to match Step 1's actual exports.

- [ ] **Step 3: Write the lint and schema tests**

Create `frontend/src/components/editor/frontmatterLint.test.ts` and `frontmatterSchema.test.ts` covering, at minimum: a required key that is missing produces a diagnostic; a well-formed document produces none; the schema resolves a known type's keys. Use the exact function names and return shapes from Step 1.

- [ ] **Step 4: Run them**

Run: `cd frontend && npx vitest run src/components/editor`
Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/editor/*.test.ts
git commit -m "test(frontend): cover the editor frontmatter modules"
```

---

### Task 8: Test the mount transition hook

**Files:**

- Create: `frontend/src/hooks/useMountTransition.test.tsx`

**Interfaces:**

- Consumes: `useMountTransition` from `./useMountTransition`.

- [ ] **Step 1: Write the test**

Create `frontend/src/hooks/useMountTransition.test.tsx` using `renderHook` and fake timers:

```tsx
import { describe, expect, it, vi, afterEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { useMountTransition } from "./useMountTransition";

afterEach(() => {
  vi.useRealTimers();
});

describe("useMountTransition", () => {
  it("enters synchronously when opened", () => {
    const { result } = renderHook(({ open }) => useMountTransition(open, 200), {
      initialProps: { open: false },
    });
    expect(result.current.mounted).toBe(false);

    act(() => {
      result.current; // no-op to keep the hook reference live
    });

    const { rerender } = renderHook(
      ({ open }) => useMountTransition(open, 200),
      {
        initialProps: { open: false },
      },
    );
    rerender({ open: true });
    expect(result.current).toBeDefined();
  });

  it("unmounts after the duration on close", () => {
    vi.useFakeTimers();
    const { result, rerender } = renderHook(
      ({ open }) => useMountTransition(open, 200),
      {
        initialProps: { open: true },
      },
    );
    expect(result.current.mounted).toBe(true);

    rerender({ open: false });
    expect(result.current.state).toBe("exit");
    expect(result.current.mounted).toBe(true);

    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(result.current.mounted).toBe(false);
  });
});
```

Simplify the first test to a single `renderHook` with `initialProps`, then `rerender({ open: true })`, asserting `mounted` is `true` and `state` is `'enter'`; the second test as written above is the important one.

- [ ] **Step 2: Run it**

Run: `cd frontend && npx vitest run src/hooks/useMountTransition.test.tsx`
Expected: pass.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/hooks/useMountTransition.test.tsx
git commit -m "test(frontend): cover the mount transition hook"
```

---

### Task 9: Component tests with React Testing Library

**Files:**

- Create: `frontend/src/components/SegmentAudioControls.test.tsx`
- Create: `frontend/src/components/LimitChip.test.tsx`
- Create: `frontend/src/components/MarkdownProse.test.tsx`

**Interfaces:**

- Consumes: `SegmentAudioControls`, `LimitChip`, `MarkdownProse`.

- [ ] **Step 1: Test the audio controls**

Create `frontend/src/components/SegmentAudioControls.test.tsx`. Assert that the play control is disabled while generating and calls back when idle:

```tsx
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SegmentAudioControls } from "./SegmentAudioControls";

describe("SegmentAudioControls", () => {
  it("disables play while a clip is generating", () => {
    render(
      <SegmentAudioControls
        state="generating"
        onPlay={() => {}}
        onStop={() => {}}
        onRegenerate={() => {}}
      />,
    );
    expect(screen.getByLabelText("Play this line")).toBeDisabled();
  });

  it("plays when idle", async () => {
    const onPlay = vi.fn();
    render(
      <SegmentAudioControls
        state="idle"
        onPlay={onPlay}
        onStop={() => {}}
        onRegenerate={() => {}}
      />,
    );
    await userEvent.click(screen.getByLabelText("Play this line"));
    expect(onPlay).toHaveBeenCalledOnce();
  });
});
```

- [ ] **Step 2: Test the limit chip**

Create `frontend/src/components/LimitChip.test.tsx` with fake timers. Assert the chip renders the countdown for a future `until` and renders nothing for a past one:

```tsx
import { describe, expect, it, vi, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { LimitChip } from "./LimitChip";

afterEach(() => vi.useRealTimers());

describe("LimitChip", () => {
  it("renders a countdown for a future limit", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-06T12:00:00Z"));
    render(
      <LimitChip
        block={{
          provider: "openai",
          role: "gm",
          until: "2026-10-06T12:00:30Z",
        }}
      />,
    );
    expect(screen.getByText(/openai \(gm\): 30s/)).toBeInTheDocument();
  });

  it("renders nothing once the limit has passed", () => {
    render(
      <LimitChip
        block={{
          provider: "openai",
          role: "gm",
          until: "2020-01-01T00:00:00Z",
        }}
      />,
    );
    expect(screen.queryByText(/openai/)).toBeNull();
  });
});
```

Confirm the `LimitState` field names in `frontend/src/types.ts` and correct the fixture if they differ.

- [ ] **Step 3: Test the inline prose grammar**

Create `frontend/src/components/MarkdownProse.test.tsx`. Assert that a wikilink becomes a clickable entity, emphasis is rendered, and a stage direction is handled per `displayMode`:

```tsx
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MarkdownProse } from "./MarkdownProse";

describe("MarkdownProse", () => {
  it("renders a wikilink as a clickable entity", async () => {
    const onEntityClick = vi.fn();
    render(
      <MarkdownProse
        text="The quay is quiet by [[the-quay|The Quay]]."
        onEntityClick={onEntityClick}
      />,
    );
    await userEvent.click(screen.getByText("The Quay"));
    expect(onEntityClick).toHaveBeenCalledWith("the-quay");
  });

  it("renders emphasis", () => {
    render(<MarkdownProse text="A **grim** guard." />);
    expect(screen.getByText("grim").tagName).toBe("STRONG");
  });
});
```

- [ ] **Step 4: Run them**

Run: `cd frontend && npx vitest run src/components/SegmentAudioControls.test.tsx src/components/LimitChip.test.tsx src/components/MarkdownProse.test.tsx`
Expected: pass. Correct selectors or field names to match the components as needed.

- [ ] **Step 5: Run the whole frontend task and commit**

Run: `mise run test:frontend`

```bash
git add frontend/src/components/SegmentAudioControls.test.tsx frontend/src/components/LimitChip.test.tsx frontend/src/components/MarkdownProse.test.tsx
git commit -m "test(frontend): cover representative components with RTL"
```

---

# Part B - The e2e harness

### Task 10: Build the harness, the fixture, and a smoke test

**Files:**

- Create: `pkg/e2e/harness.go`
- Create: `pkg/e2e/fixture.go`
- Create: `pkg/e2e/smoke_test.go`
- Modify: `mise.toml`
- Modify: `.gitignore`

**Interfaces:**

- Produces: `NewBrowser(t *testing.T, baseURL string) *Browser`; `Browser` methods `Navigate`, `Click`, `Type`, `Clear`, `Text`, `InputValue`, `BodyText`, `Eval`, `WaitVisible`, `WaitFor`, `WaitForGone`, `Console`, `InstrumentAudio`, `Clips`, `WaitForClips`, `Screenshot`; `NewFixture(t *testing.T, configYAML string) *Fixture`; `Fixture` fields `Service`, `Server`, `Root`; `Fixture` methods `WriteSystem`, `WriteWorld`, `InitGame`, `WriteEntities`, `WriteHistory`, `Launch`.

- [ ] **Step 1: Write the browser wrapper**

Create `pkg/e2e/harness.go`:

```go
// Package e2e holds the end-to-end browser suite and the harness it runs on.
//
// The browser tests carry the `e2e` build tag, so the default `go test ./...`
// never compiles them and a machine without Chrome stays fast. Run them with
// `mise run test:e2e` (`go test -tags e2e ./pkg/e2e/...`).
package e2e

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"

 "github.com/chromedp/cdproto/log"
 "github.com/chromedp/cdproto/page"
 "github.com/chromedp/cdproto/runtime"
 "github.com/chromedp/chromedp"

 "github.com/darkliquid/localrpg/pkg/driver"
)

const defaultTimeout = 45 * time.Second

// requireBrowser returns a usable browser path, or skips the test. Finding the
// binary is not enough, so it asks driver.Available, which launches one; the two
// share their allocator options, so a capability probe cannot disagree with a run.
func requireBrowser(t *testing.T) string {
 t.Helper()
 path := driver.ChromePath()
 if path == "" {
  t.Skip("no chrome/chromium available; skipping browser test")
 }
 ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
 defer cancel()
 if err := driver.Available(ctx); err != nil {
  t.Skipf("no usable browser in this environment; skipping browser test: %v", err)
 }
 return path
}

// Browser is a headless Chrome session driving one page.
type Browser struct {
 t       *testing.T
 ctx     context.Context
 cancel  context.CancelFunc
 baseURL string
 dir     string

 mu      sync.Mutex
 console []string
}

// NewBrowser launches a browser bound to baseURL and skips the test when the
// host has no usable one. baseURL may be empty for a page the test builds itself
// (a file:// probe); only a relative Navigate path needs it.
func NewBrowser(t *testing.T, baseURL string) *Browser {
 t.Helper()
 browserPath := requireBrowser(t)

 allocOptions := append(chromedp.DefaultExecAllocatorOptions[:],
  chromedp.ExecPath(browserPath),
  chromedp.Flag("headless", true),
  chromedp.Flag("no-sandbox", true),
  chromedp.Flag("disable-gpu", true),
  chromedp.WSURLReadTimeout(defaultTimeout),
 )
 allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocOptions...)
 taskCtx, cancelTask := chromedp.NewContext(allocCtx)
 ctx, cancelTimeout := context.WithTimeout(taskCtx, defaultTimeout)

 b := &Browser{t: t, ctx: ctx, baseURL: baseURL, dir: artifactDir(t)}
 b.cancel = func() { cancelTimeout(); cancelTask(); cancelAlloc() }

 chromedp.ListenTarget(taskCtx, func(ev interface{}) {
  switch event := ev.(type) {
  case *runtime.EventConsoleAPICalled:
   parts := make([]string, 0, len(event.Args))
   for _, arg := range event.Args {
    parts = append(parts, string(arg.Value))
   }
   b.record(event.Type.String() + ": " + strings.Join(parts, " "))
  case *runtime.EventExceptionThrown:
   b.record("exception: " + event.ExceptionDetails.Error())
  case *log.EventEntryAdded:
   b.record("log: " + event.Entry.Text)
  }
 })

 t.Cleanup(b.cancel)
 return b
}

func (b *Browser) record(entry string) {
 b.mu.Lock()
 b.console = append(b.console, entry)
 b.mu.Unlock()
}

// Console returns the page's console and exception entries so far.
func (b *Browser) Console() []string {
 b.mu.Lock()
 defer b.mu.Unlock()
 return append([]string(nil), b.console...)
}

// run executes actions under the session deadline. A failure captures artifacts
// and the console, because a CDP error alone rarely says what the page did.
func (b *Browser) run(actions ...chromedp.Action) {
 b.t.Helper()
 if err := chromedp.Run(b.ctx, actions...); err != nil {
  b.dumpArtifacts()
  b.t.Fatalf("%v\npage console:\n%s", err, strings.Join(b.Console(), "\n"))
 }
}

func (b *Browser) resolve(path string) string {
 if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "file:") {
  return path
 }
 return strings.TrimRight(b.baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

// Navigate opens a path relative to the fixture's server, or an absolute URL.
func (b *Browser) Navigate(path string) {
 b.t.Helper()
 b.run(chromedp.Navigate(b.resolve(path)))
}

// WaitVisible waits for the selector to be visible.
func (b *Browser) WaitVisible(selector string) {
 b.t.Helper()
 b.run(chromedp.WaitVisible(selector, chromedp.BySearch))
}

// Click waits for the selector, then clicks it.
func (b *Browser) Click(selector string) {
 b.t.Helper()
 b.run(chromedp.WaitVisible(selector, chromedp.BySearch), chromedp.Click(selector, chromedp.BySearch))
}

// Type waits for the selector, then sends the text.
func (b *Browser) Type(selector, text string) {
 b.t.Helper()
 b.run(chromedp.WaitVisible(selector, chromedp.BySearch), chromedp.SendKeys(selector, text, chromedp.BySearch))
}

// Clear empties an input before typing into it.
func (b *Browser) Clear(selector string) {
 b.t.Helper()
 b.run(chromedp.WaitVisible(selector, chromedp.BySearch), chromedp.Clear(selector, chromedp.BySearch))
}

// Text reads a selector's text content.
func (b *Browser) Text(selector string) string {
 b.t.Helper()
 var out string
 b.run(chromedp.WaitVisible(selector, chromedp.BySearch), chromedp.Text(selector, &out, chromedp.BySearch))
 return out
}

// InputValue reads an input's value.
func (b *Browser) InputValue(selector string) string {
 b.t.Helper()
 var out string
 b.run(chromedp.WaitVisible(selector, chromedp.BySearch), chromedp.Value(selector, &out, chromedp.BySearch))
 return out
}

// BodyText reads the whole page's inner text.
func (b *Browser) BodyText() string {
 b.t.Helper()
 return b.Eval(`document.body.innerText`)
}

// Eval runs a script and returns its string result.
func (b *Browser) Eval(script string) string {
 b.t.Helper()
 var out string
 b.run(chromedp.Evaluate(script, &out))
 return out
}

// WaitFor polls the page until every needle is present, or the deadline passes.
func (b *Browser) WaitFor(needles ...string) {
 b.t.Helper()
 b.waitFor("show", func(text string) bool {
  for _, needle := range needles {
   if !strings.Contains(strings.ToUpper(text), strings.ToUpper(needle)) {
    return false
   }
  }
  return true
 }, strings.Join(needles, ", "))
}

// WaitForGone polls the page until the needle is absent.
func (b *Browser) WaitForGone(needle string) {
 b.t.Helper()
 b.waitFor("lose", func(text string) bool {
  return !strings.Contains(text, needle)
 }, needle)
}

func (b *Browser) waitFor(verb string, ok func(string) bool, what string) {
 b.t.Helper()
 deadline := time.Now().Add(15 * time.Second)
 var last string
 for {
  last = b.BodyText()
  if ok(last) {
   return
  }
  if time.Now().After(deadline) {
   b.dumpArtifacts()
   b.t.Fatalf("the page never did %s %q:\n%s", verb, what, last)
  }
  time.Sleep(200 * time.Millisecond)
 }
}

// InstrumentAudio installs a probe that records every Audio element the page
// creates and whether its play() promise settled, so a test can tell "the page
// played the clip" from "the page created a clip it never played".
func (b *Browser) InstrumentAudio() {
 b.t.Helper()
 b.run(chromedp.ActionFunc(func(ctx context.Context) error {
  _, err := page.AddScriptToEvaluateOnNewDocument(instrumentAudioScript).Do(ctx)
  return err
 }))
}

// Clips returns the recorded audio probe as JSON.
func (b *Browser) Clips() string {
 b.t.Helper()
 return b.Eval(`JSON.stringify(window.__clips || [])`)
}

// WaitForClips polls until the recorded clips have settled, or the deadline passes.
func (b *Browser) WaitForClips() string {
 b.t.Helper()
 deadline := time.Now().Add(10 * time.Second)
 for {
  clips := b.Clips()
  settled := clips != "[]" &&
   (!strings.Contains(clips, `"played":false`) || strings.Contains(clips, `"error":"`))
  if settled || time.Now().After(deadline) {
   return clips
  }
  time.Sleep(100 * time.Millisecond)
 }
}

// Screenshot writes a PNG of the viewport to path.
func (b *Browser) Screenshot(path string) {
 b.t.Helper()
 var buf []byte
 b.run(chromedp.CaptureScreenshot(&buf))
 if err := os.WriteFile(path, buf, 0o644); err != nil {
  b.t.Fatalf("write screenshot: %v", err)
 }
}

const instrumentAudioScript = `(function () {
  window.__clips = [];
  var Real = window.Audio;
  window.Audio = function (src) {
    var audio = new Real(src);
    var entry = { src: String(src).slice(0, 22), played: false, error: null };
    window.__clips.push(entry);
    audio.addEventListener('error', function () { entry.error = 'load'; });
    var play = audio.play.bind(audio);
    audio.play = function () {
      var result = play();
      if (result && result.then) {
        result.then(function () { entry.played = true; }).catch(function (err) { entry.error = String(err && err.name ? err.name : err); });
      }
      return result;
    };
    return audio;
  };
  window.Audio.prototype = Real.prototype;
})();`

// artifactDir resolves where a test's failure artifacts go. CI sets
// E2E_ARTIFACT_DIR and uploads it; locally it defaults to test-results/.
func artifactDir(t *testing.T) string {
 base := os.Getenv("E2E_ARTIFACT_DIR")
 if base == "" {
  base = "test-results"
 }
 return filepath.Join(base, strings.NewReplacer("/", "_", " ", "_", "\\", "_").Replace(t.Name()))
}

// dumpArtifacts writes the page's text, a DOM probe, and a screenshot. It runs
// while a test is already failing, so every step ignores its own error.
func (b *Browser) dumpArtifacts() {
 if b.dir == "" {
  return
 }
 if err := os.MkdirAll(b.dir, 0o755); err != nil {
  return
 }
 var text string
 if err := chromedp.Run(b.ctx, chromedp.Evaluate(`document.body.innerText`, &text)); err == nil {
  _ = os.WriteFile(filepath.Join(b.dir, "body.txt"), []byte(text), 0o644)
 }
 var probe string
 if err := chromedp.Run(b.ctx, chromedp.Evaluate(domProbeScript, &probe)); err == nil {
  _ = os.WriteFile(filepath.Join(b.dir, "dom.json"), []byte(probe), 0o644)
 }
 var buf []byte
 if err := chromedp.Run(b.ctx, chromedp.CaptureScreenshot(&buf)); err == nil {
  _ = os.WriteFile(filepath.Join(b.dir, "failure.png"), buf, 0o644)
 }
}

const domProbeScript = `JSON.stringify({
  url: location.href,
  title: document.title,
  mains: document.querySelectorAll('main').length,
  buttons: document.querySelectorAll('button').length,
  dialogs: document.querySelectorAll('[role="dialog"]').length,
  bodyStart: document.body.innerText.slice(0, 400),
})`
```

- [ ] **Step 2: Write the fixture**

Create `pkg/e2e/fixture.go`:

```go
package e2e

import (
 "fmt"
 "net/http/httptest"
 "os"
 "path/filepath"
 "testing"

 "github.com/darkliquid/localrpg/pkg/engine"
 "github.com/darkliquid/localrpg/pkg/gui"
)

// Fixture is the application under test: a real gui.Service on a temp root,
// served over an httptest server with the embedded SPA. Every browser test
// starts here, so no test re-implements the server wiring.
type Fixture struct {
 Service *gui.Service
 Server  *httptest.Server
 Root    string
}

// NewFixture starts the app on a fresh temp root with the given config.yaml.
// An empty configYAML leaves the app on its defaults.
func NewFixture(t *testing.T, configYAML string) *Fixture {
 t.Helper()
 root := t.TempDir()
 if configYAML != "" {
  if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(configYAML), 0o644); err != nil {
   t.Fatal(err)
  }
 }
 svc := gui.NewService(root)
 t.Cleanup(svc.Close)

 server := httptest.NewServer(gui.NewServer(svc, gui.AssetHandler()))
 t.Cleanup(server.Close)

 return &Fixture{Service: svc, Server: server, Root: root}
}

// Launch opens a browser on the fixture's server.
func (f *Fixture) Launch(t *testing.T) *Browser {
 t.Helper()
 return NewBrowser(t, f.Server.URL)
}

// WriteSystem writes a minimal system manifest.
func (f *Fixture) WriteSystem(t *testing.T, id, name string) {
 t.Helper()
 writeFile(t, filepath.Join(f.Service.GetResolver().SystemDir(id), "system.yaml"),
  fmt.Sprintf("id: %s\nname: %s\nversion: \"1.0\"\n", id, name))
}

// WriteWorld writes a minimal world manifest with an entities directory.
func (f *Fixture) WriteWorld(t *testing.T, id, name string, compatible []string) {
 t.Helper()
 dir := f.Service.GetResolver().WorldDir(id)
 body := fmt.Sprintf("id: %s\nname: %s\n", id, name)
 if len(compatible) > 0 {
  body += "compatible_systems:\n"
  for _, system := range compatible {
   body += fmt.Sprintf("  - %s\n", system)
  }
 }
 writeFile(t, filepath.Join(dir, "world.yaml"), body)
 if err := os.MkdirAll(filepath.Join(dir, "entities"), 0o755); err != nil {
  t.Fatal(err)
 }
}

// InitGame creates a campaign and closes the init session.
func (f *Fixture) InitGame(t *testing.T, opts engine.InitOptions) {
 t.Helper()
 session, err := engine.InitGame(f.Service.GetResolver(), opts)
 if err != nil {
  t.Fatalf("InitGame: %v", err)
 }
 _ = session.Close()
}

// WriteEntities writes entity notes into a campaign.
func (f *Fixture) WriteEntities(t *testing.T, gameID string, files map[string]string) {
 t.Helper()
 dir := filepath.Join(f.Service.GetResolver().GameDir(gameID), "entities")
 for name, body := range files {
  writeFile(t, filepath.Join(dir, name), body)
 }
}

// WriteHistory replaces a campaign's history log with the given JSONL.
func (f *Fixture) WriteHistory(t *testing.T, gameID, lines string) {
 t.Helper()
 writeFile(t, filepath.Join(f.Service.GetResolver().GameDir(gameID), "history.jsonl"), lines)
}

func writeFile(t *testing.T, path, body string) {
 t.Helper()
 if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
  t.Fatal(err)
 }
 if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
  t.Fatal(err)
 }
}
```

- [ ] **Step 3: Write the smoke test**

Create `pkg/e2e/smoke_test.go`:

```go
//go:build e2e

package e2e

import (
 "strings"
 "testing"
)

func TestLauncherHubRenders(t *testing.T) {
 f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")

 b := f.Launch(t)
 b.Navigate("/")
 b.WaitVisible(`//button[@aria-label="New Campaign"]`)
 if text := b.BodyText(); !strings.Contains(text, "No Campaigns Yet") {
  t.Fatalf("expected the empty-state launcher, got:\n%s", text)
 }
}
```

- [ ] **Step 4: Add the e2e task and ignore artifacts**

In `mise.toml`, add:

```toml
[tasks."test:e2e"]
description = "Run the end-to-end browser suite (skips without a browser)"
run = "go test -v -count=1 -tags e2e ./pkg/e2e/..."
```

In `.gitignore`, add:

```
# End-to-end failure artifacts
test-results/
pkg/e2e/test-results/
```

- [ ] **Step 5: Run the smoke test**

Run: `mise run test:e2e`
Expected: with Chrome installed, `TestLauncherHubRenders` passes; without one it skips with a reason.

- [ ] **Step 6: Confirm the default suite is untouched**

Run: `go test ./pkg/e2e/...`
Expected: `ok` with no test files run (the tagged file is excluded), and `go build ./...` succeeds.

- [ ] **Step 7: Commit**

```bash
git add pkg/e2e/harness.go pkg/e2e/fixture.go pkg/e2e/smoke_test.go mise.toml .gitignore
git commit -m "test(e2e): add a shared browser harness, fixture, and smoke test"
```

---

### Task 11: Migrate the delete-campaign test

**Files:**

- Create: `pkg/e2e/launcher_test.go`
- Delete: `pkg/gui/delete_campaign_e2e_test.go`

**Interfaces:**

- Consumes: `NewFixture`, `Fixture.WriteSystem`, `Fixture.WriteWorld`, `Fixture.InitGame`, `Fixture.WriteHistory`, `Fixture.Launch`, `Browser.WaitForGone`, `Browser.BodyText`.

- [ ] **Step 1: Write the migrated test**

Create `pkg/e2e/launcher_test.go`:

```go
//go:build e2e

package e2e

import (
 "strings"
 "testing"

 "github.com/darkliquid/localrpg/pkg/engine"
)

const threeTurns = `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"You look around."}` + "\n" +
 `{"number":2,"timestamp":"2026-09-21T10:05:00Z","mode":"Do","input":"wait","narration":"Time passes."}` + "\n" +
 `{"number":3,"timestamp":"2026-09-21T10:10:00Z","mode":"Do","input":"sleep","narration":"You rest."}` + "\n"

// TestDeletingTheLastCampaignResetsLauncherStats is the feedback loop for the
// report "when deleting a campaign, the stats in the top right of the launcher
// should reset".
func TestDeletingTheLastCampaignResetsLauncherStats(t *testing.T) {
 f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")
 f.WriteSystem(t, "freeform", "Freeform")
 f.WriteWorld(t, "harbour-realm", "Harbour Realm", []string{"freeform"})
 f.InitGame(t, engine.InitOptions{GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour-realm", PlayerName: "Sean"})
 f.WriteHistory(t, "campaign-01", threeTurns)

 b := f.Launch(t)
 b.Navigate("/")
 b.WaitVisible(`//button[@aria-label="Campaign Settings"]`)

 if text := b.BodyText(); !strings.Contains(text, "3 turns") {
  t.Fatalf("stats badge does not show the campaign's turn count before deletion:\n%s", text)
 }

 b.Click(`//button[@aria-label="Campaign Settings"]`)
 b.Click(`//button[normalize-space()='Delete']`)
 b.Click(`//button[normalize-space()='Confirm Delete']`)
 b.WaitForGone("3 turns")
}
```

- [ ] **Step 2: Run it**

Run: `go test -tags e2e -run TestDeletingTheLastCampaignResetsLauncherStats -v ./pkg/e2e/`
Expected: pass with a browser, skip without one.

- [ ] **Step 3: Delete the old test**

Run:

```bash
rm pkg/gui/delete_campaign_e2e_test.go
```

- [ ] **Step 4: Verify the package still builds and tests**

Run: `go vet ./pkg/gui/... && go test ./pkg/gui/...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/e2e/launcher_test.go pkg/gui/delete_campaign_e2e_test.go
git commit -m "test(e2e): migrate the campaign deletion test onto the harness"
```

---

### Task 12: Migrate the export-player test

**Files:**

- Create: `pkg/e2e/export_test.go`
- Delete: `pkg/gui/export_player_e2e_test.go`

**Interfaces:**

- Consumes: `Fixture.WriteSystem`, `Fixture.WriteWorld`, `Fixture.InitGame`, `Fixture.WriteEntities`, `Fixture.WriteHistory`, `Browser.InstrumentAudio`, `Browser.WaitForClips`, `Browser.Clips`, `Browser.WaitFor`, `gui.AssetFS`, `gui.ExportRequestDTO`, `Service.SubscribeExportEvents`, `Service.StartExport`.

- [ ] **Step 1: Write the migrated test**

Create `pkg/e2e/export_test.go`. Port `exportedBundleFixture` to use the fixture builders and keep `TestExportedBundlePlaysTheTheatre`'s assertions. The fixture becomes:

```go
//go:build e2e

package e2e

import (
 "bytes"
 "compress/gzip"
 "context"
 "encoding/base64"
 "io"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/darkliquid/localrpg/pkg/engine"
 "github.com/darkliquid/localrpg/pkg/gui"
)

// exportedBundleFixture builds a played campaign and exports it as a web bundle,
// returning the bundle's page path and the export's own messages. It skips when
// the player has not been built, because a bundle ships that build.
func exportedBundleFixture(t *testing.T) (string, []string) {
 t.Helper()

 f := NewFixture(t, "media:\n  tts:\n    type: builtin\n    auto_play: false\n  image:\n    type: disabled\n")
 f.WriteSystem(t, "freeform", "Freeform")
 f.WriteWorld(t, "harbour", "Harbour Realm", nil)
 f.InitGame(t, engine.InitOptions{GameID: "campaign-01", SystemID: "freeform", WorldID: "harbour", PlayerName: "Sean"})
 f.WriteEntities(t, "campaign-01", map[string]string{
  "garrick.md":  "---\nid: garrick\nname: Garrick\ntype: character\n---\nA grim guard.\n",
  "the-quay.md": "---\nid: the-quay\nname: The Quay\ntype: location\n---\nSalt air.\n",
  "sean.md":     "---\nid: sean\nname: Sean O'Malley\ntype: character\n---\nA traveller.\n",
 })
 f.WriteHistory(t, "campaign-01", `{"number":1,"timestamp":"2026-09-21T10:00:00Z","mode":"Do","input":"look","narration":"The quay is quiet.","location":"the-quay","segments":[{"kind":"narration","text":"The quay is quiet by [[the-quay]]."},{"kind":"speech","speaker":"Garrick","speaker_id":"garrick","text":"Keep moving."},{"kind":"speech","speaker":"Sean","speaker_id":"sean","player":true,"text":"I will."}]}`+"\n")

 events := f.Service.SubscribeExportEvents()
 defer f.Service.UnsubscribeExportEvents(events)

 out := t.TempDir()
 if _, err := f.Service.StartExport(context.Background(), gui.ExportRequestDTO{
  GameID: "campaign-01", Format: "web", OutDir: out, Art: true, Audio: true,
 }); err != nil {
  t.Fatalf("StartExport: %v", err)
 }

 var messages []string
 deadline := time.After(60 * time.Second)
 for {
  select {
  case event := <-events:
   if event.Phase == "error" {
    t.Fatalf("export failed: %s", event.Error)
   }
   if event.Message != "" {
    messages = append(messages, event.Message)
   }
   if event.Phase == "done" {
    page := filepath.Join(out, "campaign-01-web.html")
    if _, err := os.Stat(page); err != nil {
     t.Fatalf("export reported done without a page: %v", err)
    }
    return page, messages
   }
  case <-deadline:
   t.Fatal("the export did not finish in time")
  }
 }
}
```

Port the body of `TestExportedBundlePlaysTheTheatre` onto the harness: replace `requireBrowser`/allocator setup with `b := NewBrowser(t, "")`, replace `chromedp.Run(ctx, ...)` calls with the harness methods, replace `waitFor`/`bundleDom`/`waitForClips`/`failedRequests` with `b.WaitFor`, `b.Eval(domProbe)`, `b.WaitForClips`, and `b.Console`. Keep `bundleStory` as a package-level helper. The instrumentation becomes `b.InstrumentAudio()`. The navigation becomes `b.Navigate((&url.URL{Scheme: "file", Path: bundlePath}).String())`. Keep every assertion.

- [ ] **Step 2: Run it**

Run: `mise run build:frontend && go test -tags e2e -run TestExportedBundlePlaysTheTheatre -v ./pkg/e2e/`
Expected: pass with a browser and a built player; skip without either.

- [ ] **Step 3: Delete the old test**

Run:

```bash
rm pkg/gui/export_player_e2e_test.go
```

- [ ] **Step 4: Verify the package**

Run: `go vet ./pkg/gui/... && go test ./pkg/gui/...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/e2e/export_test.go pkg/gui/export_player_e2e_test.go
git commit -m "test(e2e): migrate the exported bundle test onto the harness"
```

---

### Task 13: Migrate the portrait-clipping test

**Files:**

- Create: `pkg/e2e/portrait_test.go`
- Delete: `pkg/gui/portrait_clipping_test.go`

**Interfaces:**

- Consumes: `NewBrowser`, `Browser.Navigate`, `Browser.Eval`, `Browser.Screenshot`, `gui.AssetFS`.

- [ ] **Step 1: Write the migrated test**

Create `pkg/e2e/portrait_test.go`. Keep the probe HTML construction and the assertions; replace the allocator block with `b := NewBrowser(t, "")` and the `chromedp.Run(ctx, chromedp.Navigate(fileURL), chromedp.Poll(...))` with `b.Navigate(fileURL)` followed by a `b.Eval` poll helper, or add a `Poll` method to the harness. If a poll is needed, add to `harness.go`:

```go
// Poll waits for a script to return true, or the deadline passes.
func (b *Browser) Poll(script, describe string) {
 b.t.Helper()
 deadline := time.Now().Add(15 * time.Second)
 for {
  if b.Eval(script) == "true" {
   return
  }
  if time.Now().After(deadline) {
   b.dumpArtifacts()
   b.t.Fatalf("the page never became ready: %s", describe)
  }
  time.Sleep(100 * time.Millisecond)
 }
}
```

Keep `TestMirroredPortraitRendersInsideItsRoundedBox`'s image decoding and per-variant assertions exactly as they are.

- [ ] **Step 2: Run it**

Run: `go test -tags e2e -run TestMirroredPortraitRendersInsideItsRoundedBox -v ./pkg/e2e/`
Expected: pass with a browser and a built player; skip otherwise.

- [ ] **Step 3: Delete the old test**

Run:

```bash
rm pkg/gui/portrait_clipping_test.go
```

- [ ] **Step 4: Verify the package**

Run: `go vet ./pkg/gui/... && go test ./pkg/gui/...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/e2e/portrait_test.go pkg/e2e/harness.go pkg/gui/portrait_clipping_test.go
git commit -m "test(e2e): migrate the portrait clipping test onto the harness"
```

---

### Task 14: Add a launcher create-campaign test

**Files:**

- Create: `pkg/e2e/launcher_test.go` (append)

**Interfaces:**

- Consumes: `NewFixture`, `Fixture.WriteSystem`, `Fixture.WriteWorld`, `Fixture.Launch`, `Browser.Click`, `Browser.Type`, `Browser.WaitFor`.

- [ ] **Step 1: Write the test**

Append to `pkg/e2e/launcher_test.go`:

```go
// TestCreatingACampaignFromTheLauncher drives the real creation flow: the dock's
// New Campaign button opens the world flyout, choosing a world opens the modal,
// and submitting produces a campaign the hero stage renders.
func TestCreatingACampaignFromTheLauncher(t *testing.T) {
 f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")
 f.WriteSystem(t, "freeform", "Freeform")
 f.WriteWorld(t, "harbour-realm", "Harbour Realm", []string{"freeform"})

 b := f.Launch(t)
 b.Navigate("/")
 b.Click(`//button[@aria-label="New Campaign"]`)
 b.Click(`//button[@aria-label="Harbour Realm"]`)
 b.WaitVisible(`//button[normalize-space()='Create Campaign']`)
 b.Type(`//input[@placeholder='e.g. Valen Duskwarden']`, "Sean")
 b.Click(`//button[normalize-space()='Create Campaign']`)
 b.WaitFor("Chronicles of Harbour Realm")
}
```

- [ ] **Step 2: Run it**

Run: `go test -tags e2e -run TestCreatingACampaignFromTheLauncher -v ./pkg/e2e/`
Expected: pass with a browser. If the flyout needs the world to be the only one, the fixture already provides exactly one.

- [ ] **Step 3: Commit**

```bash
git add pkg/e2e/launcher_test.go
git commit -m "test(e2e): cover creating a campaign from the launcher"
```

---

### Task 15: Add a settings studio test

**Files:**

- Create: `pkg/e2e/settings_test.go`

**Interfaces:**

- Consumes: `NewFixture`, `Fixture.Launch`, `Browser.Click`, `Browser.WaitFor`, `Browser.Clear`, `Browser.Type`, `Browser.InputValue`.

- [ ] **Step 1: Write the tab sweep and the persistence round trip**

Create `pkg/e2e/settings_test.go`:

```go
//go:build e2e

package e2e

import (
 "fmt"
 "testing"
)

// TestSettingsStudioRendersEveryTab opens the studio from the dock and asserts
// each tab renders its signature content, which catches a lazy-chunk or
// tab-wiring failure that a type check cannot.
func TestSettingsStudioRendersEveryTab(t *testing.T) {
 f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")

 b := f.Launch(t)
 b.Navigate("/")
 b.Click(`//button[@aria-label="Settings"]`)
 b.WaitFor("Global Settings")

 tabs := []struct{ label, needle string }{
  {"Paths", "Storage & Discovery Paths"},
  {"Providers", "Cloud & Ecosystem Providers"},
  {"AI Agents", "AI Agents"},
  {"Media Engines", "Media Engines"},
  {"Batch Jobs", "Batch Jobs"},
  {"Preferences", "Preferences"},
  {"Usage", "Usage"},
  {"Debug", "Debug"},
 }
 for _, tab := range tabs {
  b.Click(fmt.Sprintf(`//button[.//span[normalize-space()='%s']]`, tab.label))
  b.WaitFor(tab.needle)
 }
}

// TestSettingsRoundTripAPath edits a storage path, saves, reloads the page, and
// asserts the value survives, which is what "settings persist" means to a user.
func TestSettingsRoundTripAPath(t *testing.T) {
 f := NewFixture(t, "agents:\n  roles:\n    gm:\n      type: builtin\n      builtin_name: echo\n")

 b := f.Launch(t)
 b.Navigate("/")
 b.Click(`//button[@aria-label="Settings"]`)
 b.WaitFor("Global Settings")

 const input = `//label[normalize-space()='Rule Systems Directory']/following-sibling::input`
 b.Clear(input)
 b.Type(input, "/tmp/e2e-systems")
 b.Click(`//button[.//span[normalize-space()='Save Settings']]`)

 b.Navigate("/")
 b.Click(`//button[@aria-label="Settings"]`)
 b.WaitFor("Global Settings")
 if got := b.InputValue(input); got != "/tmp/e2e-systems" {
  t.Fatalf("path did not round-trip: got %q", got)
 }
}
```

- [ ] **Step 2: Run them**

Run: `go test -tags e2e -run 'TestSettingsStudio' -v ./pkg/e2e/`
Expected: pass with a browser.

- [ ] **Step 3: Commit**

```bash
git add pkg/e2e/settings_test.go
git commit -m "test(e2e): cover the settings studio tabs and persistence"
```

---

# Part C - Wiring

### Task 16: Wire the e2e suite into CI

**Files:**

- Modify: `mise.toml`
- Modify: `.github/workflows/ci.yml`

**Interfaces:**

- Produces: `mise run test` runs backend, frontend, and e2e; `mise run lint` vets the tagged package; CI has an `e2e` job that installs Chrome and uploads `test-results/`.

- [ ] **Step 1: Fold e2e into the aggregate and the vet**

In `mise.toml`, change `test` to depend on all three:

```toml
[tasks.test]
description = "Run all tests across the project"
depends = ["test:backend", "test:frontend", "test:e2e"]
```

Change `lint`'s body to vet the tagged package too:

```toml
[tasks.lint]
description = "Run code analysis and linting"
depends = ["lint:docs", "lint:goreleaser", "lint:actions"]
run = """
go vet ./...
go vet -tags e2e ./pkg/e2e/...
"""
```

- [ ] **Step 2: Add the CI job**

In `.github/workflows/ci.yml`, add an `e2e` job after `go`:

```yaml
e2e:
  name: End-to-end browser tests
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v7

    - uses: jdx/mise-action@v5

    # The e2e package does not link GTK or WebKit, but the service it drives
    # reaches the audio pipeline, so the same headers the go job needs are
    # installed here to keep the two jobs identical.
    - name: Install native build dependencies
      run: |
        sudo apt-get update
        sudo apt-get install --no-install-recommends -y \
          libgtk-4-dev libwebkitgtk-6.0-dev libasound2-dev libx11-dev

    - name: Build the embedded frontend
      run: mise run build:frontend

    - name: Install Chrome
      uses: browser-actions/setup-chrome@v2
      id: chrome

    - name: Run the browser suite
      env:
        CHROME_EXEC: ${{ steps.chrome.outputs.chrome-path }}
        E2E_ARTIFACT_DIR: ${{ github.workspace }}/test-results
      run: mise run test:e2e

    - name: Upload failure artifacts
      if: always()
      uses: actions/upload-artifact@v7
      with:
        name: e2e-artifacts
        path: test-results
        if-no-files-found: ignore
        retention-days: 7
```

- [ ] **Step 3: Validate the workflow**

Run: `mise run lint:actions`
Expected: actionlint passes.

- [ ] **Step 4: Confirm the local aggregate**

Run: `mise run test:backend && mise run test:frontend`
Expected: both pass. (`mise run test` also runs `test:e2e`, which skips without a browser.)

- [ ] **Step 5: Commit**

```bash
git add mise.toml .github/workflows/ci.yml
git commit -m "ci: run the end-to-end browser suite in its own job"
```

---

### Task 17: Document the testing setup

**Files:**

- Modify: `AGENTS.md`

**Interfaces:**

- Produces: AGENTS.md describes the Vitest runner, the `pkg/e2e` package, the `e2e` build tag, the new tasks, and the CI job.

- [ ] **Step 1: Update the Commands section**

In `AGENTS.md`, change the `test:frontend` line and add the e2e task:

```
mise run test           # go test -v -count=1 ./...  AND  mise run test:frontend  AND  mise run test:e2e
mise run test:frontend  # npx tsc --noEmit and npx vitest run (frontend/)
mise run test:e2e       # go test -tags e2e ./pkg/e2e/... (skips without a browser)
```

- [ ] **Step 2: Add a testing section**

Add a short section (after "Conventions" or near it) covering: the frontend suite is Vitest + React Testing Library + jsdom, configured in `frontend/vitest.config.ts` with explicit imports and no globals; `frontend/scripts/*.mjs` no longer contains tests; browser tests live in `pkg/e2e`, carry the `e2e` build tag, use the `harness.go` browser wrapper and `fixture.go` app fixture, skip when no browser is present, and write `body.txt`/`dom.json`/`failure.png` to `$E2E_ARTIFACT_DIR` (default `test-results/`) on failure; the CI `e2e` job installs Chrome with `browser-actions/setup-chrome` and uploads those artifacts.

- [ ] **Step 3: Note the moved tests**

Update the Gotchas or Architecture section to say the browser e2e tests live in `pkg/e2e`, not `pkg/gui`, and why (the harness imports `pkg/gui`, so keeping them in `pkg/gui` would be an import cycle).

- [ ] **Step 4: Lint the prose if the file is in scope**

Run: `grep -c "AGENTS.md" .vale.ini || true`
Expected: `AGENTS.md` is not in the Vale scope, so no prose lint is needed. If it is, run `mise run lint:prose`.

- [ ] **Step 5: Commit**

```bash
git add AGENTS.md
git commit -m "docs: describe the frontend and end-to-end test setup"
```

---

## Self-Review

**Spec coverage:**

- 4.1 frontend runner, config, setup, scripts, coverage -> Task 1, plus the module/hook/component tests in Tasks 5-9.
- 4.2 migrating the three checks -> Tasks 2, 3, 4.
- 4.3 the harness and fixture -> Task 10.
- 4.4 migrated coverage -> Tasks 11, 12, 13; new coverage -> Tasks 14, 15.
- 4.5 build tags and tasks -> Task 10 (tag, `test:e2e`) and Task 16 (`test`, `lint`).
- 4.6 CI -> Task 16.
- 4.7 failure artifacts -> Task 10 (`dumpArtifacts`, `artifactDir`) and Task 16 (upload).
- Rollout documentation -> Task 17.

**Placeholder scan:** Tasks 5, 6, 7, 8, 9, 12, and 13 instruct reading a module's exports or porting an existing body before writing the final assertions, because the exact field names live in files the executor must read; each names the file to read and the behaviour to assert, and no step says "add tests" without saying which. No step says "TBD" or "similar to Task N".

**Type consistency:** `NewBrowser(t, baseURL)` and `NewFixture(t, configYAML)` are used consistently from Task 10 onward. `Fixture.Launch(t) *Browser` is used in Tasks 10-15. `Browser.WaitFor(needles...)` and `WaitForGone(needle)` are used as defined. `dumpArtifacts`, `artifactDir`, and `domProbeScript` are defined in Task 10 and used there. `threeTurns` is defined in Task 11 and reused only there. The `Poll` method added in Task 13 is defined in `harness.go` in the same task.
