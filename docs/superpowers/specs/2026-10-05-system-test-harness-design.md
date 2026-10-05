# System Test Harness Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#43 SYS-7](https://github.com/darkliquid/LocalRPG/issues/43)
**Epic:** [#18 Systems depth (mechanics and rolls)](https://github.com/darkliquid/LocalRPG/issues/18)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §3 (SYS-7)
**Depends on:** [#41 SYS-5](https://github.com/darkliquid/LocalRPG/issues/41), [#42 SYS-6](https://github.com/darkliquid/LocalRPG/issues/42)
**Scope:** new `pkg/systemtest`, `cmd/localrpg`, `pkg/gui`, `frontend`

---

## 1. Problem

A system is code: `mechanics.js` runs through goja, mutates entity state, and decides outcomes. The
only way to check that it behaves is to play a campaign and watch. There is no way to assert "a `do`
action on a Might 2 character yields a weak hit" or "a strong outcome spends a point of grit",
because:

- the rules engine is only ever constructed against a real store and a real provider;
- the dice are random, so a check's outcome is not reproducible in a test;
- there is no scenario format, no runner, and no surface to run one.

So a system author, and the generator in SG-1, cannot verify a system before shipping it. SYS-6
gives a corpus; nothing runs it.

## 2. Goals

- A **scenario** format: a named sequence of actions with expected outcomes, totals, and state.
- A **deterministic runner**: the same scenario always produces the same result.
- The runner works against a system with no store, no provider, and no network.
- Exposed as a CLI command and a studio action.
- The reference systems (SYS-6) ship scenarios, so the corpus is exercised in CI.

## 3. Non-goals

- Testing the GM's prose or a provider. This is mechanics only.
- A full tabletop rules test suite; the scope is the system's declared behaviour.
- Generated systems (SG-3 reuses this as a gate).

## 4. Design

### 4.1 The scenario format

A scenario is YAML, stored beside the system at `systems/<id>/tests/<name>.yaml`:

```yaml
name: a deft approach
seed: 42
setup:
  player:
    stats: { might: 2, edge: 3 }
    tags: [nimble]
steps:
  - action: do
    input: slip past the guard
    expect:
      outcome: weak
      total: { min: 7, max: 9 }
      message_contains: "past"
  - action: do
    input: spend grit to press on
    expect:
      state: { grit: 1 }
```

```go
// Scenario is one deterministic test of a system's mechanics.
type Scenario struct {
	Name  string    `yaml:"name"`
	Seed  int64     `yaml:"seed"`
	Setup SetupSpec `yaml:"setup,omitempty"`
	Steps []Step    `yaml:"steps"`
}

type SetupSpec struct {
	Player SetupEntity `yaml:"player,omitempty"`
}

type SetupEntity struct {
	Stats map[string]interface{} `yaml:"stats,omitempty"`
	Tags  []string               `yaml:"tags,omitempty"`
}

type Step struct {
	Action string       `yaml:"action"`
	Input  string       `yaml:"input,omitempty"`
	Expect Expectations `yaml:"expect,omitempty"`
}

type Expectations struct {
	Outcome         string                 `yaml:"outcome,omitempty"`
	Total           *Range                 `yaml:"total,omitempty"`
	State           map[string]interface{} `yaml:"state,omitempty"`
	MessageContains string                 `yaml:"message_contains,omitempty"`
}

type Range struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
}
```

### 4.2 The deterministic bridge

The runner supplies a `GameHostAPI` implementation (a `testBridge`) so a system runs with no store
and no provider:

- `Roll(notation)` evaluates the notation with a **seeded** `math/rand` source, so a scenario's dice
  are reproducible. It returns the same `*rules.RollResult` shape the engine uses.
- `GetStat`/`SetStat` read and write an in-memory entity whose `state` starts from the scenario's
  `setup`.
- `ListStats`/`ListSkills`/`CheckConventions` return the system's declared schema.
- `GetEntity`/`SaveEntity` operate on the in-memory entity; `SetLocation`/`GetLocation`,
  `InjectGMDirection`/`GetDirectives`, and `Log`/`GetLogs` are in-memory.

The bridge is a test double, so it lives in `pkg/systemtest` (non-test code, because the CLI and the
studio use it too).

### 4.3 The runner

```go
// Failure is one expectation a scenario did not meet.
type Failure struct {
	Scenario string
	Step     int
	Detail   string
}

// Run loads a system into a fresh engine with a deterministic bridge, executes
// every step, and returns the expectations that failed.
func Run(system System, scenario Scenario) []Failure

// RunAll runs every scenario for a system.
func RunAll(system System, scenarios []Scenario) []Failure
```

`System` is the runner's view of a system (`ID`, `Script`, `Mechanics`), so it accepts both a
`refsystems.ReferenceSystem` and a studio draft without an import cycle.

Each step:

1. calls `engine.ExecuteAction(step.Action, {"action": step.Input, "player": playerID})`;
2. resolves any check the hook requested through the same `SchemaResolver` the engine uses;
3. compares the result against `Expect` (outcome, total range, message substring) and the bridge's
   entity state against `State`.

A step with no `expect` is a setup step: it runs and asserts nothing.

### 4.4 Surfaces

- **CLI**: `localrpg debug test-system <id>` loads the system from the systems directory (or
  `--reference` for a corpus system), reads `tests/*.yaml`, runs them, and prints failures with a
  non-zero exit on any failure.
- **Studio**: a "Run tests" action POSTs the current draft (system + scenarios) to
  `POST /api/system/test` and renders failures inline. This is what makes an authored system
  verifiable without leaving the app.
- **CI**: the `pkg/refsystems` scenarios run in `mise run test`, so the corpus is a regression suite.

### 4.5 Relationship to SG-3

SG-3 (generated-system smoke test) uses this harness: a generated system must pass a minimal
scenario (load, roll a check, apply a state change) before it can be saved. This spec provides the
mechanism; SG-3 provides the policy.

## 5. Behaviour

| Scenario | Result |
| --- | --- |
| an expectation that holds | no failure |
| an outcome mismatch | a failure naming the step and both outcomes |
| a state mismatch | a failure naming the path and both values |
| a system with no scenarios | `RunAll` returns no failures; the CLI reports "no scenarios" |
| a script that fails to load | a failure naming the load error |
| the same scenario twice | identical failures (deterministic seed) |

## 6. Testing

- `pkg/systemtest`: the deterministic bridge produces the same roll for the same seed and notation;
  `Run` passes a matching scenario and fails a mismatched one; state assertions read the bridge;
  a script load error is a failure, not a panic.
- `pkg/refsystems`: each corpus system ships at least one scenario and passes it.
- `pkg/gui`: the test endpoint returns failures for a broken draft and none for a good one.
- A property test: running the same scenario twice yields byte-identical failure lists.

## 7. Rollout

Additive: a new package, a CLI command, an endpoint, and studio chrome. No migration. The corpus
scenarios are new files.

## 8. Risks

- **Determinism leaks.** Any use of a global RNG or the real clock breaks reproducibility. The
  seeded bridge is the only dice source, and the property test guards it.
- **Scope of assertions.** Prose assertions are fragile; the format asserts mechanics (outcome,
  total, state) and only a message substring, deliberately.
- **Duplicate logic with the engine.** The runner resolves checks through the same `SchemaResolver`
  the engine uses, so the two cannot diverge; it must not reimplement resolution.
