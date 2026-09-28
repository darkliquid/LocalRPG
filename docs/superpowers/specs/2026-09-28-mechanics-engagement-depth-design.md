# Mechanics Engagement Depth & Hook Completeness Design

**Date:** 2026-09-28
**Status:** Proposed
**Scope:** Wire the two mechanics hooks that are implemented but never called (`onWorldTick`, `onHealthZero`), deepen the resolution prompt so checks are actually invoked, make mechanical consequences visible on the turn, and remove the dead hook code
**Related:** `pkg/rules` (`js_engine.go`, `host_api.go`, `resolver.go`), `pkg/engine` (`orchestrator.go`, `advancement.go`, `history.go`), `pkg/harness` (`mechanics_instructions.go`, `context.go`, `context_plan.go`), `pkg/config` (`types.go`), `pkg/gui` (`runtime.go`, `service.go`, `types.go`), `frontend/`; implements `docs/superpowers/specs/2026-09-25-turn-memory-mechanics-followups-design.md` and builds on `2026-09-25-mechanics-engagement-and-declarative-schema-design.md`, `2026-09-26-mechanics-trigger-and-cadence-design.md`, `2026-09-26-mechanics-engagement-and-visibility-design.md`, and `2026-09-26-chronicle-inline-check-results-design.md`

## 1. Overview & Goals

Mechanics are integrated more deeply than they feel: `onAction` runs before
generation, `onTurnBegin`/`onTurnEnd` bracket the turn, `onCheck` resolvers feed
`request_check`, and `validateSubmission` enforces the engagement policy. But two
hooks are fully implemented and **never called in production**:

- `JSEngine.ExecuteWorldTick` (`pkg/rules/js_engine.go:267`) and the
  `onWorldTick` registrar (`:138`) have no caller anywhere.
- `JSEngine.EvaluateHealthZero` (`:333`) and `onHealthZero` (`:166`) have no
  caller; the design to apply them exists
  (`2026-09-25-turn-memory-mechanics-followups-design.md:23-26,136-191`) but its
  own status line reads *"No implementation plan written as of 2026-09-27."*

The engagement prompt is thin: `FormatMechanicsInstructions`
(`pkg/harness/mechanics_instructions.go:15`) describes a policy but never names
the player's actual stats or values, so the model has little to test and defaults
to narration. The result is checks that fire only when the cadence floor forces
them (`pkg/engine/orchestrator.go:479-484`), not because the fiction invited
them.

**Goals:**

- Make the world move: call `onWorldTick` on a configurable cadence so systems
  can advance off-screen state.
- Apply health-zero: detect the declared health stat reaching zero and resolve
  the declared `onHealthZero`/`zero_effect`.
- Strengthen the resolution instruction using the system's own declared stats,
  so the model knows what it may test.
- Make mechanical consequences legible on the turn: checks (already inline),
  state changes, advancement, health effects, and world ticks.
- Delete the unwired `WasmEngine` or mark it explicitly experimental.

**Non-Goals:**

- Replacing `JSEngine` with a persistent VM across turns; the per-turn rebuild is
  deliberate (`pkg/gui/service.go:1286-1288`).
- Per-NPC health tracking (recorded as an adjacent follow-up in the existing
  spec; this iteration covers the player).
- Changing the check-resolution algorithm or the outcome vocabulary.
- Changing the default engagement policy (`auto`) or the cadence default (3).

**Success Criteria:**

- A system with an `onWorldTick` hook fires it every configured N turns, and its
  effects (directives, stat writes, returned text) appear on the following turn.
- A turn that drives the declared health stat to `<= 0` records the resolved
  `zero_effect` in `Turn.HealthEffects`, in `history.jsonl`, the DTO, and the
  chronicle.
- The mechanics instruction names the player's declared stats and current values
  (when the system declares them) and instructs `request_check` before
  `submit_turn` for uncertain actions under `auto`.
- The chronicle shows state changes, advancement, health effects, and world
  ticks, not only checks.
- No production code path exists that constructs `WasmEngine`.

## 2. Investigation Findings

- **Hooks and registries.** `pkg/rules/js_engine.go`: registries at `:21-25`
  (`turnEndHooks`, `turnBeginHooks`, `worldTickHooks`, `checkResolvers`,
  `healthZeroHooks`); registrars `onAction:119`, `onTurnEnd:129`,
  `onWorldTick:138`, `onTurnBegin:147`, `onCheck:156`, `onHealthZero:166`;
  executors `ExecuteTurnEnd:255`, `ExecuteWorldTick:267`, `ExecuteTurnBegin:315`,
  `EvaluateHealthZero:333`.
- **Wired call sites.** `orchestrator.go:738` (`ExecuteTurnBegin`, ctx
  `{turn, location}`), `orchestrator.go:1109` (`ExecuteTurnEnd`, ctx built
  `:1100-1108`). `onAction` at `:635-648`. `ExecuteWorldTick` and
  `EvaluateHealthZero` appear only in tests.
- **Cadence, not ticks.** The only time-based behaviour is the check cadence
  floor: `shouldForceCheck`/`forceCheckNudge` (`orchestrator.go:479-484`,
  `pkg/engine/cadence.go`), driven by `MechanicsCadenceTurns` (default 3,
  `pkg/config/types.go:326-334`). There is no notion of a world turn.
- **Health-zero gap and design.** Followups spec `:23-26` records that the
  declared health stat is never read; `:136-191` designs `SetHealthSpec`,
  `healthOutcome`, `Turn.HealthEffects []HealthEffect{Entity, Effect}`, DTO and
  chronicle surfacing, and `onTurnEnd` ctx `health_effects`. No plan file exists
  for it.
- **Prompt construction.** `FormatMechanicsInstructions(spec, engagement)`
  switches on `off`/`ask`/`auto` (`mechanics_instructions.go:15-38`) and appends
  notation, outcome vocabulary, and difficulties (`:41-60`). It never sees stat
  values. `turnRuntime.mechanicsPrompt` is built once per runtime and cached
  (`pkg/gui/runtime.go:92-94`); the JSEngine is rebuilt per turn
  (`pkg/gui/service.go:1284-1292`), so a callable-accepting prompt cannot be
  cached that way.
- **Context placement.** `buildSections` orders the `mechanics` section first
  among policy text and marks it **not droppable**
  (`pkg/harness/context.go:263-277`), so it always reaches the model. It is not a
  prefix section: `isPrefixSection` returns only rules/lore/instructions/catalogue
  (`context.go:845-847`), so mechanics rides in the delta prompt. `BuildPrefix`
  (`pkg/harness/context_plan.go:25`) is **dead code** — only its own test calls
  it — while the live prefix is built from `isPrefixSection` candidates
  (`context.go:760-781`).
- **Turn shape.** `Turn` (`pkg/engine/history.go:17-60`) already carries `Checks`
  (`harness.CheckResult`), `Verdict`, `PendingCheck`, `ToolCalls`, `Context`,
  and `Memories`; advancement is recorded as engine-owned memories
  (`pkg/engine/advancement.go:42,101,124`). There is no `HealthEffects` or
  `WorldTick` field yet.
- **Dead alternative.** `pkg/rules/wasm_engine.go` defines `WasmEngine` exporting
  only `host_roll`, with no production caller; `JSEngine` is what the loader and
  GUI use.

## 3. Design

### 3.1 Wire `onWorldTick` on a cadence

Add a world-tick cadence and run it at the start of a turn, before context
assembly, so its effects land in the turn that triggers them.

```go
// pkg/config
type MechanicsConfig struct {
    Engagement     string `yaml:"engagement,omitempty"`
    CadenceTurns   int    `yaml:"cadence_turns,omitempty"`
    WorldTickTurns int    `yaml:"world_tick_turns,omitempty"` // 0 disables; negative disables
}

// MechanicsWorldTickTurns returns the world-tick cadence; 0 or unset disables it.
func (c *Config) MechanicsWorldTickTurns() int
```

In `ProcessActionStream`, after mechanics/action resolution and before
`o.assembler.Assemble` (`orchestrator.go:712`), when
`worldTickDue(turnNumber, cadence)`:

- Call `o.rulesEngine.ExecuteWorldTick(ctx)` with
  `{turn, location, player, entities}`.
- A hook may mutate stats through the host bridge (persisted to entity Markdown),
  return a summary string, and/or call `injectGMDirection`. The instruction
  `onWorldTick` is deliberately lower-frequency than a turn: it is for off-screen
  drift (faction moves, clocks, rumour, healing), not per-turn bookkeeping.
- Prepend any returned summary/directive to the turn's GM directive so it is part
  of the prompt that generates this turn.
- Record the tick on the turn:

```go
// Turn
WorldTick string `json:"world_tick,omitempty"`
```

Because the JSEngine is rebuilt each turn, all tick state must live through the
bridge (entity state/location); the hook cannot rely on VM globals. This is
already true for `onAction`/`onTurnEnd` and must be documented for systems
authors in the mechanics authoring docs.

### 3.2 Apply health-zero (implements the followups spec)

Implement `2026-09-25-turn-memory-mechanics-followups-design.md` §3.2 as
specified:

- `func (o *TurnOrchestrator) SetHealthSpec(health *core.HealthSpec)`.
- `healthOutcome(ctx)` reads `bridge.GetStat(playerID, health.Stat)`; when the
  numeric value is `<= 0` and `ZeroEffect` is non-empty, resolve via
  `EvaluateHealthZero`.
- Evaluate once per turn, after `rules.ApplyStateChanges` (`orchestrator.go:1042`)
  and after `ExecuteTurnEnd` (`:1109`) — whichever leaves the stat at zero first;
  idempotent within the turn.
- Add `HealthEffect{Entity, Effect}` and `Turn.HealthEffects`, persist, map in
  `turnDTO`, include in the `onTurnEnd` ctx as `"health_effects"`, and render in
  the chronicle when non-empty.

No `mechanics.Health` ⇒ no evaluation, preserving current behaviour.

### 3.3 Deepen the resolution prompt

Change `FormatMechanicsInstructions` to accept the player's declared stats and
their current values:

```go
// pkg/harness
type StatValue struct { ID, Label string; Value int }

func FormatMechanicsInstructions(spec *core.MechanicsSpec, engagement string, stats []StatValue) string
```

- Under `auto`, add an explicit sequencing rule: *for any action whose outcome
  could cost or grant something, call `request_check` before `submit_turn`, and
  reflect the returned outcome in the narration.* Keep the existing guidance
  about safe actions and NPCs not rolling.
- List the player's stats with values in a compact line, e.g.
  `Player stats: Body 3, Wits 2, Charm 1.` so the model can choose a plausible
  one. Values come from the bridge at prompt-build time, so the prompt becomes
  per-turn; rebuild it in `prepareTurn` rather than caching it in `turnRuntime`
  (the runtime already holds `declaredStats`; only the rendered string needs to
  move).
- Add a one-line worked example using the system's own notation and outcome
  vocabulary when they are declared.
- Under `ask`, keep `propose_check` then stop. Under `off`, unchanged.
- Enhance the forced-check nudge (`forceCheckNudge`) to name the last check turn,
  so a forced check reads as a nudge rather than a contradiction.

Leave mechanics in the delta section (it can vary with engagement and player
state); do not add it to the cached prefix. **Delete `BuildPrefix`** as dead code
and keep `PrefixHash` on the live prefix.

### 3.4 Make consequences visible

- Chronicle: render `HealthEffects` and `WorldTick` as small status lines,
  alongside the existing inline check results. Advancement already appears as an
  engine memory; ensure state changes are represented (a compact
  `state_changes` summary derived from the applied changes).
- `TurnDTO`: add `HealthEffects` and `WorldTick` (and a compact state-change
  list if not already present) so the GUI can show them.
- Trace: emit a per-turn mechanics summary event with counts — `checks`,
  `forced_check` (bool), `action_hook`, `turn_begin`, `turn_end`, `world_tick`,
  `health_zero` — so engagement can be measured over time rather than argued
  about.

### 3.5 Retire the unwired wasm path

Remove `pkg/rules/wasm_engine.go` (and `wazero` usage if it becomes unused) or,
if it is intended for a future sandbox, move it behind an explicit
`experimental`/`internal` package with a doc note and no ambiguity about which
engine runs. Prefer removal: `JSEngine` is the single execution path, and dead
code in the rules package is a maintenance trap for systems authors.

## 4. Interfaces

```go
// pkg/config
type MechanicsConfig struct { Engagement string; CadenceTurns, WorldTickTurns int }
func (c *Config) MechanicsWorldTickTurns() int

// pkg/rules
func (j *JSEngine) SetHealthSpec(spec *core.HealthSpec)   // used by the orchestrator

// pkg/engine
type HealthEffect struct { Entity string `json:"entity"`; Effect string `json:"effect"` }
// Turn gains: WorldTick string; HealthEffects []HealthEffect
func (o *TurnOrchestrator) SetHealthSpec(health *core.HealthSpec)
func (o *TurnOrchestrator) healthOutcome(ctx context.Context) string

// pkg/harness
type StatValue struct { ID, Label string; Value int }
func FormatMechanicsInstructions(spec *core.MechanicsSpec, engagement string, stats []StatValue) string
```

No new HTTP routes. `TurnDTO` gains two fields; the chronicle renders them.

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| `onWorldTick` hook errors | Logged; the turn proceeds without the tick (hooks never fail a turn, matching `ExecuteTurnEnd`) |
| World-tick hook writes an undeclared stat | Existing freeform/declared rules apply via the bridge; an error is logged, not fatal |
| Health stat unreadable | Skip health evaluation; log once |
| `onHealthZero` hook errors | Fall back to the declared `zero_effect` text |
| Health already zero before the turn | No re-fire (idempotent within the turn) |
| No `mechanics.Health` | No evaluation, no field |

## 6. Testing & Verification

Go (stdlib `testing`, `t.TempDir()`):

- `pkg/engine`: a system with an `onWorldTick` hook fires it on the configured
  cadence and not between; its injected directive/summary reaches the next
  prompt and `Turn.WorldTick`.
- `pkg/engine`: a world-tick bridge mutation persists to entity Markdown and
  survives the next turn's JSEngine rebuild.
- `pkg/engine`: a turn whose `state_changes` drive the health stat to zero
  produces `HealthEffects` with the player's `zero_effect`; a positive stat
  produces none; a turn that leaves it already zero does not re-fire.
- `pkg/engine`: the `onTurnEnd` ctx contains `health_effects`.
- `pkg/harness`: `FormatMechanicsInstructions` includes stat names/values when
  supplied, the `request_check`-before-`submit_turn` rule under `auto`, the
  worked example, and is unchanged when no stats are supplied.
- `pkg/gui`: `TurnDTO` carries `HealthEffects` and `WorldTick`; the mechanics
  prompt is rebuilt per turn when stats change.
- `pkg/rules`: no test references `WasmEngine` after removal.

Frontend: `tsc` only; verify the chronicle renders the new lines against a
fixture DTO.

## 7. Compatibility & Rollout

- `WorldTickTurns` defaults to disabled (0), so existing campaigns are
  unaffected until a system opts in.
- Health evaluation defaults to no-op when `mechanics.Health` is absent.
- `FormatMechanicsInstructions` gains a parameter; all call sites
  (`pkg/gui/runtime.go`, orchestrator prompt wiring) update together.
- Moving the mechanics prompt out of the cached runtime to per-turn is a small
  cost increase, offset by the model actually resolving checks.
- Removing `WasmEngine` is a hard delete of unwired code; confirm no external
  consumer before deleting (it is an unexported-package type with no callers).

## 8. Open Questions

- Should the world tick also fire on location change, independent of the cadence?
- Is a per-entity health iteration worth doing now rather than as a follow-up?
- Should the player's stat values be in the prompt every turn, or only when
  mechanics are engaged?
- Do we surface the mechanics-summary trace as a debug panel, so authors can see
  engagement without reading traces?
- Should `WorldTickTurns` be per-system in `system.yaml` rather than global
  config, so a system ships its own pacing?

## 9. References

- Code: `pkg/rules/js_engine.go:21-25,119-166,255-346`,
  `pkg/engine/orchestrator.go:479-484,635-648,712,738,1042-1050,1109`,
  `pkg/engine/history.go:17-60`, `pkg/engine/advancement.go:22-124`,
  `pkg/harness/mechanics_instructions.go:15-60`,
  `pkg/harness/context.go:263-277,760-781,845-847`,
  `pkg/harness/context_plan.go:25`, `pkg/gui/runtime.go:17-94`,
  `pkg/gui/service.go:1284-1292`, `pkg/config/types.go:326-334`,
  `pkg/rules/wasm_engine.go`.
- Specs: `2026-09-25-turn-memory-mechanics-followups-design.md`,
  `2026-09-25-mechanics-engagement-and-declarative-schema-design.md`,
  `2026-09-26-mechanics-trigger-and-cadence-design.md`,
  `2026-09-26-mechanics-engagement-and-visibility-design.md`,
  `2026-09-26-chronicle-inline-check-results-design.md`.
- Research: `docs/proposals/2026-09-26-mechanics-integration-research.md`,
  `docs/proposals/2026-09-26-mechanics-engagement-and-advancement-research.md`.
