# Document the Exported Surface of the Lower Layers Design

**Date:** 2026-10-15
**Status:** Proposed
**Issue:** [#171](https://github.com/darkliquid/LocalRPG/issues/171)
**Epic:** [#151](https://github.com/darkliquid/LocalRPG/issues/151)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (RD-5, finding R-6, action A5)
**Scope:** `pkg/state`, `pkg/entity`, `pkg/storage`, `pkg/rules`, `pkg/engine`; `revive.toml`, `mise.toml`, `AGENTS.md`. Comments and tooling only, no code change.

## 1. Problem

`pkg/state/state.go` had no doc comment on any exported symbol: a reader of `Get`, `Set` and `Raw`
could not learn from the file that paths are dot-separated, that `Set` creates the intermediate
maps it needs, or that `Raw` hands back a shallow copy, so a nested map a caller mutates is still
the state's own. The rest of the lower layers were nearly as bare: `pkg/entity` (`VoiceConfig`,
`EntityFrontmatter`, `Entity`, `ParseMarkdownEntity`, `SerializeMarkdown`), `pkg/storage`
(`Store`, `NewStore`, `SaveEntity`, `GetEntity`, `Synced`...), `pkg/rules` (`JSEngine`,
`LoadScript`, `ExecuteAction`) and `pkg/engine` (`Session`, `InitGame`, `Turn`, `HistoryLogger`,
`TurnOrchestrator`).

`pkg/gui/types.go` shows the standard is attainable in this codebase: 93 doc lines for the DTOs.
The recommendation was to make the gap measurable before closing it — enable `revive`'s `exported`
rule in report-only mode first, then work the core packages down to zero, and gate CI only once
the count is near zero.

Measured, the gap is:

| Package | Findings | of which in this proposal |
|---|---|---|
| `pkg/gui` | 65 | no |
| `pkg/provider` | 51 | no |
| `pkg/config` | 34 | no |
| `pkg/harness` | 32 | no |
| `pkg/media` | 30 | no |
| `pkg/rules` | 22 | yes |
| `pkg/core` | 19 | no |
| `pkg/engine` | 18 | yes |
| `pkg/storage` | 13 | yes |
| `pkg/models` | 13 | no |
| `pkg/trace` | 12 | no |
| `pkg/entity` | 8 | yes |
| `pkg/tui`, `pkg/embeddings`, `pkg/scene` | 7, 6, 3 | no |
| `pkg/state` | 5 | yes |
| `pkg/sysgen`, `pkg/jsonrepair`, `pkg/driver`, `pkg/debugger`, `pkg/content` | 1 each | no |
| **total** | **336** | **65** |

Reproduce it with:

```bash
mise run lint:revive
```

## 2. Goals

- **Zero findings in `pkg/state`, `pkg/entity`, `pkg/storage`, `pkg/rules` and `pkg/engine`**, which
  takes the repository from 336 to 271.
- **A doc-comment standard** that is about the reader rather than about the linter: what the
  symbol is for, and the invariant a caller cannot infer from its signature.
- **`revive` wired in report-only mode**: pinned in `mise.toml` like every other tool, configured
  by a checked-in `revive.toml`, and run by `mise run lint:revive` with `STRICT=1` to gate.

## 3. Non-goals

- **The other 271 findings.** `pkg/gui`, `pkg/provider`, `pkg/config` and `pkg/harness` are the
  next four by count, and they are a follow-up bead rather than this one. The count is why CI is
  not gated yet: A5 says do not gate until it is near zero.
- **Renaming `entity.EntityFrontmatter`.** Revive's stutter check wants `entity.Frontmatter`,
  which is a name in the docs and the on-disk format alike. The check is switched off in
  `revive.toml` with that reason written down, rather than the type being renamed.
- **Commenting unexported symbols.** Revive's `exported` rule is what is being answered; the
  unexported helpers in these packages carry comments where they need them already.
- **Changing code.** Not one statement moves. A comment that is right about a function that is
  wrong is a bug report, not this proposal.
- **Running revive in CI.** The task exists and reports; nothing calls it until the count is near
  zero, and then it joins `mise run lint` with `STRICT=1`.

## 4. Design

### The standard

A doc comment here answers two questions, in the house style the well-documented parts of the
repository already use: what the symbol is for, and the invariant or the trap a caller cannot see
from the signature. It starts with the symbol's name, so `go doc` reads as prose and revive's
`exported` rule is satisfied.

The invariants that were invisible, and are the point of the exercise:

| Symbol | What the comment has to say |
|---|---|
| `state.State` | paths are dot-separated; safe for concurrent use |
| `state.NewState` | `initial` is adopted, not copied, so a later `Set` is visible through it |
| `state.Set` | creates intermediate maps; refuses to replace a component that is not a map |
| `state.Raw` | a shallow copy: nested maps are still the state's |
| `entity.Entity` | `Folder` is a location, not frontmatter, so `SerializeMarkdown` must never emit it |
| `entity.ParseMarkdownEntity` | the `id` is the frontmatter's, falling back to the slugified name |
| `entity.SerializeMarkdown` | frontmatter first, then the body; `State` is serialized through `Raw` |
| `storage.Store` | the index only: derived, disposable, rebuildable from the Markdown |
| `storage.Store.Close` | a no-op for a store a `Pool` handed out, because the pool owns it |
| `storage.Syncer.Sync` | recursive; skips malformed notes rather than failing the walk |
| `rules.JSEngine` | one goja VM behind a mutex, so one script runs at a time |
| `rules.JSEngine.ExecuteAction` | returns `(nil, nil)` when no handler is registered for the mode |
| `engine.Turn` | the canonical record; `Prose` is the narrator's text whichever field holds it |
| `engine.HistoryLogger` | append-only on disk; `RewindToTurn` is the only truncation |
| `engine.TurnOrchestrator` | the turn pipeline, and what it deliberately is not |

### Where A5's scope is widened, and why

A5 names `pkg/engine/game.go`. The whole package is done here, because a package where three of
eighteen exported symbols are documented is not a standard anyone can point at, and the remaining
fifteen are the same kind of work in the same package. This is the only widening; every other
package keeps its findings for the follow-up bead.

### The tooling

`revive` is pinned in `mise.toml` beside `staticcheck`, `gitleaks`, `actionlint`, `vale` and
`rumdl`, and configured by a checked-in `revive.toml`:

```toml
[rule.exported]
  # The stutter check asks for entity.Frontmatter ... renaming it is an API change
  # rather than a documentation one.
  arguments = ["disableStutteringCheck"]
```

Only the rules that earn their place are enabled, and the file carries the reason each choice was
made, the way `.rumdl.toml` and `.vale.ini` do. The task reports:

```bash
mise run lint:revive              # print every finding, exit 0
STRICT=1 mise run lint:revive     # exit non-zero on findings
```

Report-only is deliberate and matches `lint:prose`: a gate on 271 findings is a gate that gets
switched off, so `lint:revive` is not yet a dependency of `mise run lint` and no workflow calls
it.

## 5. Behaviour

None. Every change is a comment, plus a new task nobody depends on yet. The compiler and the suite
cannot tell the difference, which is the point: the deliverable is what a reader learns, and the
only machine-checkable part of it is revive's count.

## 6. Testing

- **Revive is the test.** 336 findings before, 271 after, and zero in the five packages:
  `mise run lint:revive`, filtered per package.
- **The suite is the proof nothing moved.** `mise run test` and `mise run lint` pass, and
  `git diff` shows no non-comment line changed in the five packages.
- **The config is tested by being wrong once.** Suppressing the stutter check must not suppress
  the doc-comment check: the before and after counts are compared with the config in place, and
  65 findings in those five packages is the number that goes to zero.

## 7. Rollout

One pull request, one commit: the spec, the plan, 65 doc comments, the pinned tool, the config and
the task. The follow-up bead takes the remaining 271 findings, `pkg/provider` and `pkg/gui` first
because they are the two largest.

## 8. Risks

- **A comment that restates the signature is worse than none.** The standard above is the
  mitigation: every comment says something the signature does not, and the review's own list of
  invisible invariants is the checklist.
- **Report-only lints rot.** The count is recorded here and in the follow-up bead, so the next
  session can see whether it moved. When it reaches zero, `lint:revive` joins `mise run lint`.
- **A pinned tool is a CI risk.** `mise.toml` pins are installed by `jdx/mise-action` in all five
  CI jobs, so a pin that does not resolve breaks every job. `revive` is pinned the same way as
  `staticcheck`, which is installed that way today, and the task is exercised locally before the
  commit.