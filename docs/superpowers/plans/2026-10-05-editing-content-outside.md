# Editing Content Outside the App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Document the on-disk and package formats as the interchange contract, so content can be edited in any editor.

**Architecture:** One new embedded docs article, registered with the site generator and the Vale scope, matching the code.

**Tech Stack:** Markdown; the docs lint (`mise run lint:docs`) and Vale (`mise run lint:prose`).

**Spec:** `docs/superpowers/specs/2026-10-05-editing-content-outside-design.md`

## Global Constraints

- Every path and field named must exist in the code (`pkg/core`, `pkg/rules`, `pkg/storage`).
- The article is Vale-linted; keep the prose clean and use the project's vocabulary.
- Register it in `tools/sitegen/content.go` and `scripts/lint-prose.sh`.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The article

**Files:**
- Create: `pkg/gui/docs/20-editing-content.md`

**Interfaces:**
- Consumes: `pkg/core/types.go`, `pkg/core/mechanics.go`, `pkg/rules/loader.go`, `pkg/storage/sync.go`, `pkg/content`.
- Produces: the article.

- [ ] **Step 1: Write the three-tiers section**

Describe `systems/`, `worlds/`, and `games/`, what each holds, and where they live by default (the
XDG paths), linking to `07-storage-paths.md`. Name the code owner: `core.PathResolver`.

- [ ] **Step 2: Write the system and world sections**

For a system: the tree (`system.yaml`, `mechanics.js`, `prompts/rules.md`, `tests/`) with a minimal
complete example whose `mechanics` block matches `core.MechanicsSpec`. For a world: `world.yaml`
(`genre`, `art_style`, `default_system`, `requires`), `prompts/lore.md`, `entities/**/*.md`, and
`system_overrides/<system>/hooks.js`.

- [ ] **Step 3: Write the entities and package sections**

The entity frontmatter contract, linking to `10-codex-yaml.md` for the full list, and the
`.lrpgpack` layout (`package.yaml`, the tree, `package.sig`) with the CLI verbs
(`content export|import|sign|verify`, `publisher add`).

- [ ] **Step 4: Write the workflow and "do not touch" sections**

State the edit-outside/reload-inside workflow and what triggers a reindex (`localrpg play` reindexes
at startup; `cache/index.db` is rebuilt from `history.jsonl`). List what not to edit
(`cache/index.db`, `history.jsonl`) and note that malformed frontmatter is skipped by the syncer.

- [ ] **Step 5: Commit**

```bash
git add pkg/gui/docs/20-editing-content.md
git commit -m "docs: describe editing content outside the app"
```

---

### Task 2: Register and lint

**Files:**
- Modify: `tools/sitegen/content.go`, `scripts/lint-prose.sh`

**Interfaces:**
- Consumes: the article (Task 1).
- Produces: the article rendered on the site and in the Vale scope.

- [ ] **Step 1: Register in the site generator**

Add the article to the content list in `tools/sitegen/content.go`, in reading order after the
existing guides.

- [ ] **Step 2: Register in the prose scope**

Add the path to `scripts/lint-prose.sh`'s file list.

- [ ] **Step 3: Lint**

Run: `mise run lint:docs && mise run lint:prose`
Expected: PASS (or only the documented warning/suggestion baseline).

- [ ] **Step 4: Build the site**

Run: `mise run site:build`
Expected: the article appears in `website/dist`.

- [ ] **Step 5: Commit**

```bash
git add tools/sitegen/content.go scripts/lint-prose.sh
git commit -m "docs: render and lint the editing-outside article"
```

---

### Task 3: Verification

- [ ] **Step 1: Accuracy check**

Spot-check every path and field named against `pkg/core/types.go`, `pkg/core/mechanics.go`, and
`pkg/rules/loader.go`; fix any mismatch.

- [ ] **Step 2: Full docs verification**

Run: `mise run lint:docs && mise run lint:prose && mise run site:build`
Expected: PASS.

- [ ] **Step 3: Confirm the acceptance criteria**

- The article documents the three tiers, a system, a world, entities, and the package.
- It states the edit-outside workflow and what not to touch.
- It renders on the site and passes the lint.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "docs: finalise the editing-outside guide"
```
