# Editing Content Outside the App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Document the on-disk and package formats as the interchange contract, so content can be edited in any editor.

**Architecture:** One new embedded docs article, matching the code. The site generator and the Vale scope both discover it from the directory, so no registration is needed.

**Tech Stack:** Markdown; the docs lint (`mise run lint:docs`) and Vale (`mise run lint:prose`).

**Spec:** `docs/superpowers/specs/2026-10-05-editing-content-outside-design.md`

## Global Constraints

- Every path and field named must exist in the code (`pkg/core`, `pkg/rules`, `pkg/storage`).
- The article is Vale-linted; keep the prose clean and use the project's vocabulary.
- Conventional Commits, subject under 72 chars.

---

### Task 1: The article

**Files:**
- Create: `pkg/gui/docs/22-editing-content.md`

The spec named `20-editing-content.md`, but `20-local-embeddings.md` already holds order 20, so the
article takes the next free number, 22.

**Interfaces:**
- Consumes: `pkg/core/types.go`, `pkg/core/mechanics.go`, `pkg/rules/loader.go`, `pkg/storage/sync.go`, `pkg/content`.
- Produces: the article.

- [x] **Step 1: Write the three-tiers section**

Describe `systems/`, `worlds/`, and `games/`, what each holds, and where they live by default (the
XDG paths), linking to `07-storage-paths.md`. Name the code owner: `core.PathResolver`.

- [x] **Step 2: Write the system and world sections**

For a system: the tree (`system.yaml`, `mechanics.js`, `prompts/rules.md`, `tests/`) with a minimal
complete example whose `mechanics` block matches `core.MechanicsSpec`. For a world: `world.yaml`
(`genre`, `art_style`, `default_system`, `requires`), `prompts/lore.md`, `entities/**/*.md`, and
`system_overrides/<system>/hooks.js`.

- [x] **Step 3: Write the entities and package sections**

The entity frontmatter contract, linking to `10-codex-yaml.md` for the full list, and the
`.lrpgpack` layout (`package.yaml`, the tree, `package.sig`) with the CLI verbs
(`content export|import|sign|verify`, `publisher add`).

- [x] **Step 4: Write the workflow and "do not touch" sections**

State the edit-outside/reload-inside workflow and what triggers a reload (`localrpg play` rebuilds
the entity index at startup; `cache/index.db` is derived from the Markdown and `history.jsonl`). List
what not to edit (`cache/index.db`, `history.jsonl`, `content.lock.yaml`, `assets/`) and note that
malformed frontmatter is skipped by the indexer.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/docs/22-editing-content.md
git commit -m "docs: describe editing content outside the app"
```

---

### Task 2: Register and lint

**Files:**
- No change needed: `tools/sitegen/content.go`, `scripts/lint-prose.sh`

Both discover the article from the directory, so neither needed an edit. `loadDocs` in
`tools/sitegen/content.go` walks `pkg/gui/docs` and loads every `*.md`, and `scripts/lint-prose.sh`
asks `git ls-files` for `pkg/gui/docs/*.md`, `README.md`, and `docs/debugging.md`.

- [x] **Step 1: Register in the site generator**

Verified instead of edited: `loadDocs` reads every `*.md` under `pkg/gui/docs`, and the frontmatter
`order: 22` places the article after `21-local-first`.

- [x] **Step 2: Register in the prose scope**

Verified instead of edited: the script's file list is a glob over `pkg/gui/docs/*.md`, so the
article is linted once it is tracked.

- [x] **Step 3: Lint**

Run: `mise run lint:docs && mise run lint:prose`
Result: `rumdl` reports no issues in 22 files, and Vale reports 0 errors.

- [x] **Step 4: Build the site**

Run: `mise run site:build`
Result: `website/dist/docs/22-editing-content.html` is written and appears in the navigation.

- [x] **Step 5: Commit**

```bash
git add tools/sitegen/content.go scripts/lint-prose.sh
git commit -m "docs: render and lint the editing-outside article"
```

---

### Task 3: Verification

- [x] **Step 1: Accuracy check**

Every path and field named was spot-checked against `pkg/core/types.go`, `pkg/core/mechanics.go`,
`pkg/core/profile.go`, `pkg/rules/loader.go`, `pkg/storage/sync.go`, `pkg/content`, and the CLI in
`cmd/localrpg`. Two mismatches were fixed: the article described `min_app_version` as enforced, and
the entity `portrait` field as an image path, where the image is resolved from `assets/portraits/`.

- [x] **Step 2: Full docs verification**

Run: `mise run lint:docs && mise run lint:prose && mise run site:build`
Result: PASS, with the Vale count updated in `AGENTS.md` to 190 alerts across 22 files.

- [x] **Step 3: Confirm the acceptance criteria**

- The article documents the three tiers, a system, a world, entities, and the package.
- It states the edit-outside workflow and what not to edit.
- It renders on the site and passes the lint.

- [x] **Step 4: Commit**

```bash
git add -A
git commit -m "docs: finalise the editing-outside guide"
```

---

### Task 4: Correct the drift this article exposed

**Files:**
- Modify: `pkg/gui/docs/02-worlds.md`

`02-worlds.md` showed a `settings:` block in `world.yaml`, which `core.WorldManifest` has no field
for and the loader ignores. The pinned opening location is a campaign setting that `engine.InitGame`
writes to `game.yaml`, so the example and the resolution steps were corrected to match the code.

- [x] **Step 1: Fix the world manifest example**

- [x] **Step 2: Fix the starting-location resolution steps**

- [x] **Step 3: Lint and build**

Run: `mise run lint:docs && mise run site:build`
Result: PASS.
