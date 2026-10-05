# Mechanics Housekeeping Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#44 SYS-8](https://github.com/darkliquid/LocalRPG/issues/44)
**Epic:** [#18 Systems depth (mechanics and rolls)](https://github.com/darkliquid/LocalRPG/issues/18)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §3 (SYS-8)
**Scope:** `pkg/engine`, `pkg/rules`, `pkg/harness`, `pkg/gui`, `frontend`, `pkg/gui/docs`

---

## 1. Problem

Three small debts mislead anyone reading the mechanics code:

1. **A dead verdict path.** `Turn.Verdict` (`pkg/engine/history.go:56`) is never assigned by the
   engine. It is read in two places: mapped to the DTO (`pkg/gui/service.go:1525`) and handed to a
   `mechanics.js` hook context (`pkg/engine/mechanics_engagement.go:227-228`). The vocabulary
   behind it, `harness.ActionVerdict`/`ActionFeasibility` (`pkg/harness/turn.go:5-19`), was written
   for the terminal `submit_turn` payload that was **superseded** by the progressive `@`-record
   stream (`docs/superpowers/specs/2026-10-03-progressive-turn-stream-design.md`). The types and
   DTO field suggest enforcement ("an uncertain action must resolve a check") that does not exist.
2. **A comment for a field that does not exist.** `harness.ProposedCheck`
   (`pkg/harness/turn.go:111-118`) documents that the GM references its `Ref` "in
   `dismissed_checks`". No `dismissed_checks` exists anywhere.
3. **A duplicated helper.** `newCheckID` is defined identically in
   `pkg/engine/check_resolver.go:38` and `pkg/rules/resolver.go:121`, and a third call site is in
   `pkg/rules/js_engine.go:294`.
4. **Stale docs.** `pkg/gui/docs/09-systems-studio.md` shows a `system.yaml` with an `action_modes`
   field and a JavaScript global `rollDice(notation)`. Neither exists: `SystemManifest`
   (`pkg/core/types.go:12-23`) has no `action_modes`, and the host API binds `roll`, not
   `rollDice` (`pkg/rules/js_engine.go:47`).

None of this changes runtime behaviour, but each item costs a reader time and one of them (the
verdict) implies a guarantee the engine does not make.

## 2. Goals

- Remove the dead verdict vocabulary and its DTO field, so the code stops implying enforcement
  that is not there.
- Keep `ProposedCheck` (it is live) but make its comment true.
- One `newCheckID`, one definition.
- The Systems Studio doc matches the code.

## 3. Non-goals

- Implementing verdict enforcement. If that is wanted, it is a separate spec (the trigger and
  cadence work), not this cleanup.
- Any change to how rolls or hooks behave.

## 4. Design

### 4.1 Delete the verdict vocabulary

Delete:

- `Turn.Verdict` (`pkg/engine/history.go:56`) and its JSON tag.
- `harness.ActionVerdict`, `harness.ActionFeasibility`, and the three feasibility constants
  (`pkg/harness/turn.go:5-19`).
- `TurnDTO.Verdict` (`pkg/gui/types.go:214`) and its mapping (`pkg/gui/service.go:1525`).
- The `verdict` hook-context block (`pkg/engine/mechanics_engagement.go:227-228`).
- Any frontend `verdict` field on the turn type (`frontend/src/types.ts`), and any renderer that
  reads it.

Because `Turn.Verdict` is always nil, deleting it changes no observable behaviour. A
`mechanics.js` that reads `ctx.verdict` would today always see it absent, and will continue to.

### 4.2 Keep `ProposedCheck`, fix the comment

`ProposedCheck` is live: Roll mode builds one (`pkg/engine/orchestrator.go:716`) and it drives the
`[PROPOSED CHECK]` directive passed into `runGenerationLoop` (`pkg/engine/orchestrator.go:1731`).
Only the comment changes, from a reference to a non-existent `dismissed_checks` to a description of
what actually happens (the directive is advisory; enforcement is a future concern).

### 4.3 One `newCheckID`

Add `harness.NewCheckID() string` in a small `pkg/harness/check_id.go` (harness is already imported
by both `pkg/engine` and `pkg/rules`, so no new edge is created):

```go
// NewCheckID returns a process-unique check identifier. It is the single
// definition shared by the engine's default resolver and the rules engine.
func NewCheckID() string {
	return "chk_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
```

Delete `newCheckID` from `pkg/engine/check_resolver.go` and `pkg/rules/resolver.go`, and update the
three call sites (`pkg/engine/check_resolver.go:32`, `pkg/rules/resolver.go:53`,
`pkg/rules/js_engine.go:294`) to `harness.NewCheckID()`.

### 4.4 Fix the Systems Studio doc

Rewrite the `system.yaml` example in `pkg/gui/docs/09-systems-studio.md` to match
`SystemManifest` and the declarative `mechanics` block (`pkg/core/mechanics.go`), and change the
`rollDice(notation)` example to `roll(notation)` with the real host-API surface
(`getStat`/`setStat`, `getLocation`/`setLocation`, `injectGMDirection`, `log`, `grantXP`, and the
`on*` hooks). Keep the article's structure and tone; only the inaccurate fragments change. The
embedded docs are Vale-linted, so the edit must keep the prose clean.

## 5. Testing

- `go build ./...` and `go vet ./...` after the deletions (the compiler is the test for dead code).
- `go test ./pkg/engine/ ./pkg/rules/ ./pkg/gui/` for the `newCheckID` move.
- `npx tsc --noEmit` in `frontend/` for the removed TS field.
- `mise run lint:docs` for the documentation change.
- A grep assertion in review: no remaining reference to `ActionVerdict`, `ActionFeasibility`,
  `Turn.Verdict`, `dismissed_checks`, or `rollDice`.

## 6. Rollout

No migration. `Turn.Verdict` was never written to history, so no stored record contains it. The DTO
field disappearing is a frontend-visible change, but only if a client read it (none does).

## 7. Risks

- **A hidden reader of `Verdict`.** Mitigated by `rg` across `pkg/` and `frontend/` before deleting,
  and by the compiler.
- **A `mechanics.js` in the wild reading `ctx.verdict`.** It would already be undefined; the
  cleanup does not change that. The doc fix makes the available context explicit.
- **Doc rewrite drift.** Keep the change minimal and verify with `mise run lint:docs`.
