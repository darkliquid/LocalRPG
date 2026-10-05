# Editing Content Outside the App Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#77 PKG-6](https://github.com/darkliquid/LocalRPG/issues/77)
**Epic:** [#24 Content packages and registries](https://github.com/darkliquid/LocalRPG/issues/24)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §6 (PKG-6)
**Depends on:** [#72 PKG-1](https://github.com/darkliquid/LocalRPG/issues/72)
**Scope:** `pkg/gui/docs`

---

## 1. Problem

LocalRPG is local-first and file-based: a system is a directory of YAML, JavaScript, and Markdown, and
a world is a directory of Markdown. The app reads them directly and reindexes on open. So content is
**already** editable in any editor, and that is a core property, not a side effect.

But nothing documents it as a contract. The existing articles describe the studios, not the on-disk
format as an interchange. A user who wants to edit in VS Code, keep content in git, or generate it
with a script has to reverse-engineer the layout from the source, and the package format (PKG-1) has
no user-facing description at all.

## 2. Goals

- One article that documents the on-disk formats as the contract: the three tiers, a system's shape,
  a world's shape, and the entity frontmatter.
- The package format (`.lrpgpack`) documented, with the CLI verbs to pack, unpack, sign, and verify.
- The "edit outside, reload inside" workflow stated plainly, including what triggers a reindex.
- The article renders on the showcase site and passes the docs lint, like every other guide.

## 3. Non-goals

- New behaviour. This is documentation of what exists.
- A schema reference for every field; link to the existing codex/frontmatter article instead.
- A tutorial; this is a reference.

## 4. Design

### 4.1 The article

A new `pkg/gui/docs/20-editing-content.md`, added to the site generator's content list
(`tools/sitegen/content.go`) and to the Vale scope (`scripts/lint-prose.sh`), covering:

1. **The three tiers.** `systems/`, `worlds/`, `games/`, and what each holds, with the
   `core.PathResolver` defaults and the XDG paths (link to the storage-paths article).
2. **A system.** The tree: `system.yaml` (identity plus the `mechanics` block), `mechanics.js`
   (the host API and hooks), `prompts/rules.md`, and `tests/*.yaml` (SYS-7's scenarios). A minimal
   complete example.
3. **A world.** The tree: `world.yaml` (identity, `genre`, `art_style`, `default_system`,
   `requires`), `prompts/lore.md`, `entities/**/*.md`, and `system_overrides/<system>/hooks.js`.
4. **Entities.** The frontmatter contract: `id`, `type`, `name`, `tags`, `location`, `faction`,
   `state`, `voice`, `portrait`, `history`, and the `[[wikilink]]` edges, linking to the codex
   article for the full field list.
5. **The package.** The `.lrpgpack` layout (`package.yaml`, the content tree, `package.sig`), what
   the manifest carries, and the CLI: `localrpg content export|import`, `content sign|verify`, and
   `publisher add`.
6. **The workflow.** Edit files in any editor; the app reindexes a game's `entities/` on open
   (`localrpg play` reindexes at startup) and rebuilds `cache/index.db` from `history.jsonl` when
   they disagree. Content is the source of truth; the cache is disposable.
7. **What not to touch.** `cache/index.db` (disposable), `history.jsonl` (append-only, rewritten only
   by `/undo`), and the fact that a malformed frontmatter file is skipped by the syncer.

### 4.2 Consistency

The article must match the code. Where it states a field or a path, it is the one in `pkg/core` and
`pkg/rules`; a mismatch is a docs bug. The site generator renders it from the same source the app
embeds, so the site cannot drift from the app.

### 4.3 Linting

The article is Vale-linted (`mise run lint:docs` / `lint:prose`). It is added to `scripts/lint-prose.sh`
and `.vale.ini`'s scope implicitly (the scope is the `pkg/gui/docs/*.md` set), and to
`tools/sitegen/content.go` so it appears on the site.

## 5. Behaviour

Not applicable; this is documentation. The observable outcome: a user can read one page and know how
to author or edit content outside the app and how to share it.

## 6. Testing

- `mise run lint:docs` and `mise run lint:prose` pass for the new article.
- `mise run site:build` renders it.
- A review check: every path and field named in the article exists in the code (spot-checked against
  `pkg/core/types.go`, `pkg/core/mechanics.go`, and `pkg/rules/loader.go`).

## 7. Rollout

A new docs article and two list entries. No behaviour change.

## 8. Risks

- **Documentation drift.** The format can change; the article is part of the same repo and review
  process, and the codex article already cross-references the frontmatter fields, so a change should
  update both.
- **Over-promising stability.** The on-disk format is the contract, but it can still evolve; the
  article should say which parts are stable (the tier layout, the manifest) and which are the app's
  internal cache (never to be edited).
