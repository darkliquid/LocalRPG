# Document the Exported Surface of the Lower Layers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Take revive's `exported` rule from 336 findings to 271, with zero in `pkg/state`, `pkg/entity`, `pkg/storage`, `pkg/rules` and `pkg/engine`, and wire the tool so the number is measurable.
**Architecture:** A doc comment per exported symbol that says what it is for and the invariant a caller cannot infer. No code changes. `revive` pinned in `mise.toml`, configured by `revive.toml`, run by a report-only `lint:revive` task.
**Tech Stack:** Go 1.27, `revive` 1.15.0, `mise`.
**Spec:** `docs/superpowers/specs/2026-10-15-exported-surface-design.md`

## File Map

| File | Why |
|---|---|
| `pkg/state/state.go` | 5 doc comments |
| `pkg/entity/entity.go`, `pkg/entity/memory.go` | 7 doc comments |
| `pkg/storage/store.go`, `pool.go`, `sync.go`, `db.go`, `embedding_worker.go` | 13 doc comments |
| `pkg/rules/host_api.go`, `js_engine.go`, `loader.go`, `dice.go` | 22 doc comments |
| `pkg/engine/game.go`, `history.go`, `orchestrator.go`, `timeline.go`, `memory.go`, `chronicler.go`, `image_trigger.go` | 18 doc comments |
| `revive.toml` | the rule set, with the reason for each choice |
| `mise.toml` | pin `revive`; add the `lint:revive` task |
| `AGENTS.md` | the task in the command list, and why it is report-only |

## Global Constraints

- **Comments and tooling only.** No statement, signature or field changes. `git diff -U0` on the five packages shows only added comment lines.
- **Every comment says something the signature does not.** The invariant, the trap, or the contract. Restating the name in a sentence is not a doc comment.
- **Start with the symbol's name**, so `go doc` reads as prose.
- **The documented set is A5's four packages plus all of `pkg/engine`**, with the widening recorded in the spec.
- **The task is report-only.** `lint:revive` is not a dependency of `lint`, and no workflow calls it.

---

### Task 1: Baseline

- [x] **Step 1: The count, with the config in place**

Run: `go run github.com/mgechev/revive@v1.15.0 -config revive.toml ./...`
Expected: 336 findings; 65 of them in `pkg/state`, `pkg/entity`, `pkg/storage`, `pkg/rules` and `pkg/engine`.

- [x] **Step 2: The suite green**

Run: `go test ./...`
Expected: pass, so the comment-only claim is checked against a green tree.

---

### Task 2: `pkg/state` (5)

- [x] **Step 1: Document the type, its constructor and its three methods**

`State`, `NewState`, `Get`, `Set`, `Raw`. Say that paths are dot-separated, that `NewState` adopts
the map it is given, that `Set` creates intermediate maps and refuses to replace a non-map, and
that `Raw` is a shallow copy.

- [x] **Step 2: Verify the package is clean**

Run: `go run github.com/mgechev/revive@v1.15.0 -config revive.toml ./pkg/state/`
Expected: no findings.

---

### Task 3: `pkg/entity` (7)

- [x] **Step 1: `entity.go`**

`VoiceConfig`, `EntityFrontmatter`, `Entity`, `InitState`, `ParseMarkdownEntity`,
`SerializeMarkdown`. Say that `Folder` is a location rather than frontmatter, that a missing `id`
falls back to the slugified name, and what `SerializeMarkdown` writes and in what order.

- [x] **Step 2: `memory.go`**

A comment on the `MemorySource` const block, because revive asks for one comment per block rather
than per constant.

- [x] **Step 3: Verify the package is clean**

Run: `go run github.com/mgechev/revive@v1.15.0 -config revive.toml ./pkg/entity/`
Expected: no findings, stutter check included because it is suppressed in the config.

---

### Task 4: `pkg/storage` (13)

- [x] **Step 1: `store.go`**

`Edge`, `Store`, `NewStore`, `SaveEntity`, `GetEntity`, `GetEdgesFrom`, `GetEdgesTo`. Say that the
store is the index rather than the durable record, that it is derived and rebuildable, and that a
shared store's `Close` is a no-op.

- [x] **Step 2: `pool.go`, `sync.go`, `db.go`, `embedding_worker.go`**

`NewPool`, `SyncResult`, `Syncer`, `NewSyncer`, `OpenDB`, `EmbeddingWorkerOptions`. Say that
`Sync` is recursive, that it skips what is not note storage, and that a malformed note is skipped
rather than failing the walk.

- [x] **Step 3: Verify the package is clean**

Run: `go run github.com/mgechev/revive@v1.15.0 -config revive.toml ./pkg/storage/`
Expected: no findings.

---

### Task 5: `pkg/rules` (22)

- [x] **Step 1: `host_api.go` (11)**

`ActionResult`, `GameHostAPI`, `DefaultHostBridge`, its `Roll`, `GetStat`, `SetStat`, `GetEntity`,
`InjectGMDirection`, `GetDirectives`, `Log`, `GetLogs`. Say that the bridge is how a script writes
without a second write path beside the engine's, and that the directives and logs it collects are
drained by the turn rather than owned by it.

- [x] **Step 2: `js_engine.go` (6), `loader.go` (3), `dice.go` (2)**

`JSEngine`, `NewJSEngine`, `LoadScript`, `ExecuteAction`, `ExecuteTurnEnd`, `ExecuteWorldTick`;
`RuleLoader`, `NewRuleLoader`, `LoadRules`; `RollResult`, `EvaluateRoll`. Say that the engine is
one VM behind a mutex, that `ExecuteAction` returns `(nil, nil)` when no handler is registered,
and what `RollResult` carries that a total does not.

- [x] **Step 3: Verify the package is clean**

Run: `go run github.com/mgechev/revive@v1.15.0 -config revive.toml ./pkg/rules/`
Expected: no findings.

---

### Task 6: `pkg/engine` (18)

- [x] **Step 1: `history.go` (6) and `game.go` (3)**

`Turn`, `HistoryLogger`, `NewHistoryLogger`, `AppendTurn`, `LoadHistory`, `RewindToTurn`;
`Session`, `Close`, `InitGame`. Say that `Turn` is the canonical record and `Prose` answers
whichever field holds the narrator's text, that the log is append-only, that `RewindToTurn` is its
only truncation, and that `Close` releases the campaign's store.

- [x] **Step 2: `orchestrator.go` (4)**

`TurnOrchestrator`, `NewTurnOrchestrator`, `SetPrompts`, `LoadPrompts`. Say what the orchestrator
owns (the phases of a turn) and what it deliberately does not (durable writes, which are the
`Timeline`'s), and that the optional setters configure a turn rather than the campaign.

- [x] **Step 3: `timeline.go`, `memory.go`, `chronicler.go`, `image_trigger.go` (5)**

`NewTimeline`, the `Default*` const block, `NewChronicler`, `SetLogger`, `MajorSceneChange`.

- [x] **Step 4: Verify the package is clean**

Run: `go run github.com/mgechev/revive@v1.15.0 -config revive.toml ./pkg/engine/`
Expected: no findings.

---

### Task 7: The tooling

- [x] **Step 1: `revive.toml`**

The `exported` rule only, with the stutter check off and the reason written beside it. The file
carries the same kind of reasoning `.rumdl.toml` and `.vale.ini` do.

- [x] **Step 2: Pin `revive` and add the task**

`revive = "1.15.0"` beside `staticcheck` in `mise.toml`, and a `lint:revive` task that prints every
finding, exits zero, and exits non-zero only under `STRICT=1`, mirroring `lint:prose`'s
convention.

- [x] **Step 3: Document the task**

The `mise run` block in `AGENTS.md` gains `mise run lint:revive`, and a short paragraph says why it
is report-only and what its count is.

- [x] **Step 4: Verify the task**

Run: `mise run lint:revive`
Expected: 271 findings, exit 0.
Run: `STRICT=1 mise run lint:revive; echo $?`
Expected: non-zero, because 271 findings are left in the middle layers.

---

### Task 8: Verification and follow-up

- [x] **Step 1: The five packages are at zero**

Run: `mise run lint:revive | grep -E 'pkg/(state|entity|storage|rules|engine)/' | wc -l`
Expected: 0.

- [x] **Step 2: Only comments changed**

Run: `git diff -U0 -- pkg/ | grep -E '^[+-]' | grep -v '^[+-][+-]' | grep -vE '^[+-]\s*(//|$)'`
Expected: no output.

- [x] **Step 3: The suite and the lint**

Run: `mise run test && mise run lint`
Expected: pass.

- [x] **Step 4: The follow-up bead**

Create a bead for the remaining 271 findings, naming `pkg/provider` (51) and `pkg/gui` (65) as the
first two, with the count recorded so the next session can see it move.