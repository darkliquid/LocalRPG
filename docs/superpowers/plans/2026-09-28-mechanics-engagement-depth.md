# Mechanics Engagement Depth Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-28.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Call the two implemented-but-unwired mechanics hooks (`onWorldTick`, `onHealthZero`), make injected directives actually reach the prompt, and put the player's own stats in the resolution instruction so checks are invited rather than forced.

**Architecture:** A new `pkg/engine/mechanics_engagement.go` holds the world tick, health outcome, directive drain, and prompt builders. The orchestrator gains `SetHealthSpec`, `SetWorldTickTurns`, and `SetMechanicsSchema`, runs the world tick before context assembly, drains host directives after hook execution, and resolves health-zero before the turn is recorded. `FormatMechanicsInstructions` gains a stat list.

**Tech Stack:** Go 1.27 stdlib tests.

**Spec:** `docs/superpowers/specs/2026-09-28-mechanics-engagement-depth-design.md`

## Global Constraints

- Use `interface{}`, never `any`; `go vet` clean.
- Standard library only in tests.
- Health zero is evaluated once, for the player, after state changes and before the turn is recorded. A health change made by an `onTurnEnd` hook is therefore seen on the next turn; this is documented, not silently wrong.
- `onWorldTick` defaults to disabled so existing campaigns do not change behaviour.
- No go.mod change: the unwired `WasmEngine` removal is deferred to the documentation/hygiene spec (it would drop the wazero dependency and is unrelated to engagement).

---

### Task 1: Config world-tick cadence

- [x] **Step 1:** `MechanicsConfig` gains `WorldTickTurns int` and
  `MechanicsWorldTickTurns() int` (0 or unset disables; a negative value
  disables explicitly). Default `0`.
- [x] **Step 2:** `go test ./pkg/config/`.

---

### Task 2: Health-zero

**Files:** `pkg/engine/history.go` (`Turn`), new `pkg/engine/mechanics_engagement.go`, `pkg/engine/orchestrator.go`.

- [x] **Step 1:** `Turn` gains `HealthEffects []HealthEffect` and
  `WorldTick string`; `HealthEffect{Entity, Effect}` with JSON tags.
- [x] **Step 2:** `SetHealthSpec(*core.HealthSpec)` stores the spec.
- [x] **Step 3:** `healthOutcome(ctx) string` reads `bridge.GetStat(playerID,
  health.Stat)`, and when the numeric value is `<= 0` and `ZeroEffect` is set,
  resolves it via `EvaluateHealthZero`.
- [x] **Step 4:** Call it after `ApplyStateChanges`/`applyAdvancement` and before
  `RecordTurnContextStructured`; append the player's effect to `turn.HealthEffects`.
- [x] **Step 5:** Include `health_effects` in the `onTurnEnd` hook context.
- [x] **Step 6:** Test: a state change to zero records the effect; a positive
  stat records none; no `mechanics.Health` is a no-op.

---

### Task 3: World tick and directive drain

- [x] **Step 1:** `SetWorldTickTurns(int)`; `worldTickDue(turnNum, cadence)` is
  true when the cadence is positive and `(turnNum-1)%cadence == 0`.
- [x] **Step 2:** In `ProcessActionStream`, before context assembly, run
  `ExecuteWorldTick({turn, location, player, entities})` when due; log errors and
  never fail the turn.
- [x] **Step 3:** `drainDirectives()` reads `HostAPI().GetDirectives()` and
  prepends them to `gmDirective`; call it after `ExecuteTurnBegin`, before
  generation, so world-tick and hook injections reach the prompt. (`GetDirectives`
  had no caller, so `injectGMDirection` was a dead end.)
- [x] **Step 4:** Record the tick's own returned text isn't available from
  `ExecuteWorldTick` (hooks return nothing); the injected directive is the
  observable effect. Set `Turn.WorldTick` to the drained directive text for this
  turn when a tick ran.
- [x] **Step 5:** Test: a world-tick hook injects a directive that appears in the
  next generation request; the cadence fires on the configured turns only.

---

### Task 4: Stat-aware resolution prompt

- [x] **Step 1:** `harness.StatValue{ID, Label string; Value int}` and
  `FormatMechanicsInstructions(spec, engagement, stats []StatValue)`; render a
  `Player stats: ...` line and a `request_check`-before-`submit_turn` rule under
  `auto`.
- [x] **Step 2:** Orchestrator `SetMechanicsSchema(spec, engagement)` marks the
  prompt for per-turn rebuild; `statValues()` reads the player's declared stats
  through the bridge; `mechanicsInstruction()` builds the string.
- [x] **Step 3:** Use `mechanicsInstruction()` at assembly; `LoadPrompts` adopts
  the schema path.
- [x] **Step 4:** Update `mechanics_instructions_test.go` for the new signature
  and assert the stats line and the sequencing rule.
- [x] **Step 5:** `go test ./pkg/harness/ ./pkg/engine/`.

---

### Task 5: Visibility and GUI wiring

- [x] **Step 1:** `TurnDTO` gains `HealthEffects` and `WorldTick`; `turnDTO`
  maps them.
- [x] **Step 2:** `frontend/src/types.ts` `Turn` gains `health_effects` and
  `world_tick`; the chronicle renders a compact line for each.
- [x] **Step 3:** `pkg/gui/runtime.go` / `service.go` wire `SetMechanicsSchema`,
  `SetHealthSpec`, and `SetWorldTickTurns`.
- [x] **Step 4:** `go test ./pkg/gui/` and `cd frontend && npx tsc --noEmit`.

---

### Task 6: Full verification

- [x] **Step 1:** `go build ./...`, `go test ./...`, `go vet ./...`.
- [x] **Step 2:** `cd frontend && npx tsc --noEmit`.
