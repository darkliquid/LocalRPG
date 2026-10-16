# Draining the Host Bridge's Directives Design

**Date:** 2026-10-16
**Status:** Proposed
**Bead:** LocalRPG-2j4
**Scope:** `pkg/rules`, `pkg/engine`, `pkg/systemtest`. No wire or API change.

## 1. Problem

A system's script reaches the model's prompt through `injectGMDirection`, which appends to a slice on the
host bridge. The orchestrator's `drainDirectives` reads that slice before generation:

```go
directives := o.rulesEngine.HostAPI().GetDirectives()
```

`GetDirectives` returns the whole accumulated slice, and nothing clears it, so a directive injected in
turn 5 is applied in turn 6 and in every turn after it. The method the orchestrator calls is named
`drainDirectives` and its own comment says "returns every directive hooks injected this turn", so the
implementation contradicts the name, the comment and the orchestrator's expectation.

## 2. Goals

- **A drain drains.** Reading the injected directives empties them, so a hook's direction reaches the one
  turn that asked for it.
- **One method, honestly named.** `GetDirectives` is replaced by `DrainDirectives` rather than joined by
  it: a second read-and-keep accessor would be a method whose only caller is a test, and the name of the
  one that remains says what it does.
- **A regression test that fails on the bug**: two drains in a row return the directive once and then
  nothing.

## 3. Non-goals

- **The logs.** `Log`/`GetLogs` accumulate the same way and nothing reads them, so no behaviour is wrong
  today. Draining them is a decision about what a turn's diagnostics are, and it is not this bead.
- **Where a directive is applied.** `applyDirectives` prepends them to the GM directive in the order they
  were injected, which is unchanged.

## 4. Design

`rules.HostAPI` loses `GetDirectives() []string` and gains:

```go
// DrainDirectives returns the directives injected since the last drain and clears
// them: a direction belongs to the turn that asked for it.
DrainDirectives() []string
```

Both implementations return the slice and reset it. `TurnOrchestrator.drainDirectives` calls it, and
keeps its own filtering of empty strings.

## 5. Behaviour

- A directive injected during a turn is applied to that turn's prompt and to no later one.
- An empty or whitespace-only directive is still filtered out.

## 6. Testing

- The existing world-tick test still sees its directive, now through the drain.
- A new test drains twice: the first returns the directive, the second returns nothing. That test fails
  against the old implementation.
- `pkg/rules`' script test reads through the drain as well.
- `mise run test` and `mise run lint`.

## 7. Risks

- **The interface loses an inspection accessor.** Nothing in production read it; the one test that did is
  updated, and a test that wants to inspect what a script injected can drain and assert on the result.
