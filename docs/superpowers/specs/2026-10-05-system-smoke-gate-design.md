# System Smoke Gate Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#86 SG-3](https://github.com/darkliquid/LocalRPG/issues/86)
**Epic:** [#26 AI system generation](https://github.com/darkliquid/LocalRPG/issues/26)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §9 (SG-3)
**Depends on:** [#84 SG-1](https://github.com/darkliquid/LocalRPG/issues/84), [#43 SYS-7](https://github.com/darkliquid/LocalRPG/issues/43)
**Scope:** `pkg/sysgen`, `pkg/systemtest`, `pkg/gui`, `frontend`

---

## 1. Problem

SG-1 verifies a generated system before offering it, and SYS-7 provides the harness. Neither applies
to a **hand-authored** system: a user can save a system whose `mechanics.js` throws on load, or whose
profile never resolves, and only discover it in play.

There is also no standard "minimum a system must do". SG-1 invents a smoke scenario per generation;
nothing defines it once, so two callers could check different things.

## 2. Goals

- One **smoke scenario** definition every system must satisfy: load, roll a check, apply a state
  change.
- A **gate** at the save path: a system that fails the smoke test is not saved silently.
- The gate is **enforced** for a generated system and a **warning** for a hand-authored one (so a
  work in progress can be saved), with an explicit override.
- The same scenario runs in CI for the reference corpus.

## 3. Non-goals

- The harness itself (SYS-7) or the full scenario format (SYS-7).
- Testing a system's balance or quality; only that it works.
- Changing the save format.

## 4. Design

### 4.1 The smoke scenario

One definition, in `pkg/systemtest`:

```go
// SmokeScenario returns the minimal scenario every system must pass: the system
// loads, a check resolves through its declared profile (or conventions), and a
// state change is applied.
func SmokeScenario(mechanics *core.MechanicsSpec) Scenario
```

It is generated from the system's own mechanics, so it tests what the system declares:

- if the system has a profile, the step names it and expects an outcome from its vocabulary;
- otherwise the step expects the conventions outcome;
- if the system declares a stat, the step sets it and expects the change.

A system with no mechanics passes trivially (it is schema-agnostic by design), which is correct: the
gate is about a declared system working, not about requiring declarations.

### 4.2 The gate

`pkg/sysgen` exposes:

```go
// Gate runs the smoke scenario and reports whether a system may be saved.
type GateResult struct {
	OK      bool
	Failures []systemtest.Failure
	Script  bool // whether the system ships a mechanics.js
}

func Gate(sys System) GateResult
```

The GUI's save path calls `Gate`:

- **Generated system** (SG-1's accept): a failure **blocks** the save. The draft is shown with the
  failure and a regenerate action.
- **Hand-authored system** (`SaveSystem`): a failure is a **warning** in the response (the existing
  `Warnings` channel), and the save proceeds unless the caller requests strict mode. A user who is
  mid-edit can save a broken system and fix it.

A `strict` flag on the save request turns the warning into a block for a user who wants it.

### 4.3 What the gate catches

- a `mechanics.js` syntax error or an undefined host call (the load fails);
- a profile that never resolves (the check yields no outcome);
- a declared stat the script cannot write (the state change fails).

It does not catch a system that is valid but unpleasant; that is the user's judgement.

### 4.4 The corpus

SYS-6's systems already ship scenarios; the smoke scenario is run against them in CI as the minimal
bar, in addition to their own scenarios. This keeps the smoke definition honest: if it cannot run the
corpus, it is wrong.

### 4.5 Surfaces

- **API**: `POST /api/system/test` (SYS-7) already runs scenarios; the save path calls `Gate`
  internally, so no new endpoint is required. The save response carries the gate result.
- **Studio**: a failed gate shows the failure inline with a regenerate action (generated) or a
  dismissible warning (hand-authored).

## 5. Behaviour

| System | Gate | Save |
| --- | --- | --- |
| generated, passes | OK | saved |
| generated, fails | blocked | shown with the failure; regenerate |
| hand-authored, passes | OK | saved silently |
| hand-authored, fails | warning | saved with the warning, unless strict |
| no mechanics declared | trivially OK | saved |
| a broken `mechanics.js` | a load failure | per the above |

## 6. Testing

- `pkg/systemtest`: `SmokeScenario` names the declared profile and expects its vocabulary; a system
  with no mechanics yields a passing scenario.
- `pkg/sysgen`: `Gate` passes a good system, fails a broken script, and fails a profile that does not
  resolve.
- `pkg/gui`: a generated system that fails cannot be saved; a hand-authored one saves with a warning;
  strict mode blocks.
- `pkg/refsystems`: the smoke scenario passes every corpus system.

## 7. Rollout

Additive: a scenario definition, a gate, and save-path wiring. A hand-authored broken system now
warns where it previously saved silently; strict mode is opt-in.

## 8. Risks

- **A gate that blocks a legitimate save.** The warning default for hand-authored systems prevents it;
  strict is a choice. A generated system is never saved broken, which is the point.
- **A smoke scenario that is too weak.** It tests load, a roll, and a state change; a system can pass
  and still be dull. That is acceptable for a gate; SYS-6's richer scenarios are the deeper check.
- **Cost.** The gate runs in-process with no model call, so it is cheap.
