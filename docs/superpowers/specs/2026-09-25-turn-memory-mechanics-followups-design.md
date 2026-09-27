# Turn, Memory & Mechanics Follow-ups Design

> **Status:** No implementation plan written as of 2026-09-27.

**Date:** 2026-09-25
**Status:** Proposed
**Scope:** Close the residual gaps left by the structured turn, memory, and mechanics work: an end-to-end GUI rules-loading test, the engine-side health-zero effect, and automated coverage for the codex memory timeline
**Related:** Structured Turn Protocol Design (2026-09-25), Entity Memories & Memory Tools Design (2026-09-25), Mechanics Engagement & Declarative Schema Design (2026-09-25), and their implementation plans

## 1. Overview & Goals

Three accepted specifications were implemented and verified, but each left one
gap that is known, bounded, and untested end to end. This specification defines
the work to close them so the feature set is complete rather than "implemented
except for".

The three gaps:

1. **The GUI rules-loading fix has no test.** The GUI now builds a `RuleLoader`
   and calls `LoadRules` once per campaign, but nothing asserts that a web
   campaign actually runs `mechanics.js`. The regression this fixed (a web
   campaign with an empty mechanics VM) could silently return.
2. **Health-zero is resolved but never applied.** `JSEngine.EvaluateHealthZero`
   exists and is tested, and `HealthSpec` declares `stat`, `max_stat`, and
   `zero_effect`, but the engine never reads the declared health stat, detects
   zero, or records the effect on the turn.
3. **The codex memory timeline has no automated test.** The read-only timeline is
   verified by `tsc --noEmit` and a manual glance only.

**Goals:**

- An automated test proves a campaign whose system ships `mechanics.js` runs a
  scripted hook during a turn through the GUI service path.
- The engine detects a declared health stat reaching zero after state changes and
  records the resolved `zero_effect` on the turn, which reaches the client and
  the `onTurnEnd` hook context.
- The codex memory timeline has automated coverage, either by introducing a
  minimal frontend test runner or by a backend contract test plus an explicit
  documented manual gate, decided in §4.

**Non-Goals:**

- Re-opening the settled protocol, memory, or mechanics contracts.
- New gameplay features beyond detecting and reporting a declared health-zero.
- A general frontend test framework migration; only the minimum needed for this
  timeline (and future small component tests) is in scope.
- Changing how memory ranking selects `currentTurn` or making mechanical-memory
  importance configurable (recorded as adjacent follow-ups, not fixed here).

**Success Criteria:**

- `TestGUILoadsRulesForCampaign` exists and fails if `LoadRules` is not called,
  then passes with the current wiring.
- A turn whose `state_changes` drive a declared health stat to zero records a
  `HealthEffects` entry equal to the system's `zero_effect` (or the hook's
  return), and the value appears in the `onTurnEnd` hook context and the GUI turn
  DTO.
- A health stat that does not reach zero leaves `HealthEffects` empty.
- The codex timeline is covered by the decided mechanism and the decision is
  recorded in this spec.

## 2. Residual 1: GUI Rules-Loading Test

### 2.1 What exists

`pkg/gui/service.go` builds the `JSEngine`, and on first use per campaign loads
rules:

```go
jsEngine := rules.NewJSEngine(rules.NewHostBridge(store, timeline, playerID))
s.rulesMu.Lock()
loaded := s.rulesLoaded[manifest.ID]
s.rulesMu.Unlock()
if !loaded {
    loader := rules.NewRuleLoader(s.resolver, jsEngine)
    if err := loader.LoadRules(manifest.SystemID, manifest.WorldID); err != nil {
        logger.Event("rules.load_error", ...)
    }
    s.rulesMu.Lock()
    s.rulesLoaded[manifest.ID] = true
    s.rulesMu.Unlock()
}
```

Nothing exercises this through the GUI service.

### 2.2 Design

Add `pkg/gui/rules_loading_test.go`:

- Build a temp root with `systems/<id>/system.yaml`, a `mechanics.js` that
  registers an observable side effect, a `worlds/<id>/world.yaml`, and a
  `games/<id>/game.yaml` referencing them. Reuse the existing GUI test rig
  (`setupTestGame`/temp-dir helpers) where possible; a hand-built tree is fine
  when the rig pins a different shape.
- The observable side effect must be readable from the test without a live
  provider. Two options, in order of preference:
  1. `mechanics.js` calls `onAction("do", function(){ log("rules-loaded"); })`;
     the test plays a `Do` turn through the service's turn path, or calls the
     service method that builds the orchestrator, and asserts the bridge log
     contains `rules-loaded`. The bridge is internal to the built orchestrator,
     so the test needs an accessor or must assert via a trace event.
  2. Emit a trace event from `LoadRules` (`rules.loaded`) and assert it appears
     in the service's trace sink. This requires `RuleLoader` to accept the
     logger; `LoadRules` currently has no logger.

Recommendation: option 2, because it is deterministic, needs no provider, and
gives operators a visible "rules loaded for campaign X" event that is useful
beyond the test. Concretely:

- `RuleLoader` gains a `SetLogger(trace.Logger)` (or `NewRuleLoaderWithLogger`)
  and emits `rules.loaded` with `system_id` and `world_id`.
- `pkg/gui` passes its logger when constructing the loader.
- The test injects a `trace.Memory` sink, builds the service, prepares a
  campaign, and asserts exactly one `rules.loaded` event.

A second assertion covers the once-per-campaign guard: preparing the same
campaign twice emits `rules.loaded` once.

### 2.3 Alternatives considered

- Asserting the hook's effect on state (write a stat from `onTurnEnd`) is a
  stronger test but couples the test to a provider and a full turn; the trace
  event is the smaller, deterministic seam.

## 3. Residual 2: Engine Health-Zero Effect

### 3.1 What exists

- `core.HealthSpec{Stat, MaxStat, ZeroEffect}` is parsed from `mechanics`.
- `JSEngine.EvaluateHealthZero(effect string) (string, error)` runs an
  `onHealthZero` hook or returns the declared text.
- `rules.ApplyStateChanges` writes `state_changes` through the host bridge.
- `Turn` has no health field; the orchestrator never reads the health stat.

### 3.2 Design

**Schema access.** The orchestrator already receives declared stats
(`SetDeclaredStats`). Add the health spec alongside:

```go
func (o *TurnOrchestrator) SetHealthSpec(health *core.HealthSpec)
```

Prioritise `HealthSpec.Stat`. If a system declares a health stat that is absent
from `Stats`, the stat is still tracked (the health block is authoritative for
what "health" is).

**Detection point.** After `rules.ApplyStateChanges` succeeds for a structured
turn, and again after any `onTurnEnd` hook that may mutate state, the engine
evaluates health:

```go
// healthOutcome reads the declared health stat for the player (and, later, any
// entity with a declared health stat) and resolves the zero effect.
func (o *TurnOrchestrator) healthOutcome(ctx context.Context) string
```

Scope for this iteration: the player entity only. A system that wants per-NPC
health can be a follow-up; the spec records it as an adjacent gap.

- Read `bridge.GetStat(playerID, health.Stat)`.
- If the numeric value is <= 0 and `ZeroEffect` is non-empty, call
  `JSEngine.EvaluateHealthZero(ZeroEffect)`; the resolved text is the outcome.
- Idempotence: only evaluate once per turn, and only when the value is zero at
  the end of the turn. A stat that was already zero and stays zero does not
  re-fire within the same turn.
- Record a list so a future per-entity health needs no breaking change:

```go
type HealthEffect struct {
    Entity string `json:"entity"`
    Effect string `json:"effect"`
}

// Turn
HealthEffects []HealthEffect `json:"health_effects,omitempty"`
```

This iteration populates at most one entry, for the player.

**Surfacing.** `Turn.HealthEffects`:

- persists in `history.jsonl` automatically (whole `Turn` marshalled);
- is added to `TurnDTO` and mapped in `turnDTO`;
- is included in the `onTurnEnd` hook context as `"health_effects"`;
- is rendered in the chronicle as a small status line when non-empty (a
  follow-up may style it as a banner).

**Non-declared behaviour.** With no `mechanics.Health`, nothing is evaluated and
`Turn.HealthEffects` stays empty, preserving today's behaviour.

### 3.3 Testing

- `healthOutcome` returns the declared text when the stat is zero, and empty when
  it is positive or the spec is absent.
- A turn whose `state_changes` subtract the health stat to zero produces a turn
  whose `HealthEffects` contains the player's `zero_effect`.
- A turn that leaves the stat positive produces an empty `HealthEffects`.
- The `onTurnEnd` hook context contains `health_effects`.
- A system with no health spec is unchanged.

## 4. Residual 3: Codex Timeline Coverage

### 4.1 The gap

`APIClient.listEntityMemories` and the `CodexDrawer` timeline have no automated
test. The repo has no frontend test runner; `tsc --noEmit` is the current gate.

### 4.2 Options

1. **Backend contract test only.** Assert `GET
     /api/game/{id}/entity/{id}/memories` returns the expected DTO shape and
     order. Leaves the component and client method to manual verification.
2. **Introduce a minimal frontend runner (recommended).** Add Vitest +
   `@testing-library/react` + `jsdom`, wired into `mise run test`, with a single
   test that renders the timeline for a stubbed client response and asserts the
   memories render newest-first. This also gives future small component tests a
   home, which the "show generation failures" work and the gallery work would
   both have benefited from.
3. **Documented manual gate only.** Keep `tsc` as the gate and add the timeline to
   a manual checklist. Cheapest, but leaves a real regression risk.

**Recommendation: option 2, preceded by option 1.** The backend endpoint test is
cheap and always worth having; the Vitest introduction is the durable fix and is
the smallest runner that fits a Vite project (`vitest` reuses the existing Vite
config). The plan must keep the runner minimal: one config, one test, no coverage
thresholds, no snapshot sprawl.

### 4.3 Design sketch (option 2)

- Add dev dependencies `vitest`, `jsdom`, `@testing-library/react`,
  `@testing-library/jest-dom`.
- `frontend/vitest.config.ts` with `environment: 'jsdom'`, `globals: true`, and
  a `setup.ts` importing `@testing-library/jest-dom`.
- `frontend/package.json` gains `"test": "vitest run"`; `mise.toml`'s
  `test:frontend` runs `tsc --noEmit && vitest run`.
- `CodexDrawer.test.tsx`: mock `APIClient.listEntityMemories` to resolve two
  memories, render with a selected `entity` and a `gameID`, assert both texts
  appear and the higher-turn memory precedes the lower.
- A loading/empty case asserts the section is absent when there are no memories.

## 5. Adjacent Gaps (recorded, not fixed here)

These were noticed while implementing the three specs and are out of scope for
this follow-up unless promoted:

- **Per-entity health.** Health-zero is scoped to the player; NPC health is not
  evaluated.
- **Mechanical memory importance is fixed at 3.** A configurable mapping from
  check stakes to importance is not implemented.
- **Memory ranking `currentTurn` is approximated.** `search_memories` ranks with
  the newest hit's turn rather than the campaign's current turn; correct for
  relative ordering, approximate for decay.
- **`search_memories` exposes only `query`.** The raw FTS `match` escape hatch
  that `search_entities`/`search_timeline` offer is not exposed.
- **`/gm` override.** The directorial directive is carried in context but does
  not explicitly set `verdict: automatic`; the behaviour is correct in practice
  but not encoded.

## 6. Testing Strategy Summary

- Backend: the GUI rules-loading test (§2.2), health-outcome tests (§3.3), and
  the memories endpoint contract test (§4.2 option 1).
- Frontend: Vitest component test for the timeline (§4.3), if option 2 is
  chosen.
- All existing suites must stay green; `mise run test` and `mise run lint` are the
  gates.

## 7. Compatibility & Migration

- `Turn.HealthEffect` is an additive optional field; old records are unaffected.
- The new `rules.loaded` trace event is additive.
- Introducing a frontend test runner changes `mise run test:frontend` to also run
  Vitest; it adds dev dependencies only, and changes no runtime bundle.

## 8. Resolved Decisions and Open Questions

Resolved in this spec: the health field is a list (`HealthEffects`) so per-entity
health can extend it without a breaking change.

Open:

- Frontend testing: adopt Vitest (option 2) now, or stay manual and keep only
  the backend contract test (option 1)? The recommendation is Vitest, but it is
  the one decision with ecosystem cost.
- Health-zero scope: player-only for this iteration (recommended), or evaluate
  every entity that declares the health stat?
