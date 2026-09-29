# Health-Zero Hook Ordering Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-28.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** See a health change made by an `onTurnEnd` hook on the same turn, while keeping one recorded effect and the hook context's `health_effects`.

**Architecture:** Two-pass evaluation around the hook. `runTurnEndHooks` moves before the turn is recorded and receives the pass-1 effect so a hook can react to it; a second pass after the hook catches a hook-driven change; the first non-empty effect is recorded once.

**Tech Stack:** Go 1.27 stdlib tests.

**Spec:** `docs/superpowers/specs/2026-09-28-health-zero-hook-ordering-design.md`

## Global Constraints

- Use `interface{}`, never `any`; `go vet` clean.
- Standard library only in tests.
- One recorded effect per turn, for the player.
- The turn stays the single durable record.

---

### Task 1: Extract and move the turn-end hooks

**Files:** `pkg/engine/orchestrator.go`, `pkg/engine/mechanics_engagement.go`.

- [x] **Step 1:** Move the `onTurnEnd` block out of the post-record tail into
  `runTurnEndHooks(turnNum, turn, pendingEffect)`, which builds the same context
  and seeds `health_effects` from `pendingEffect`.
- [x] **Step 2:** Call it in `ProcessActionStream` after `applyAdvancement` and
  before the record, so hook-driven state is part of the same turn.

### Task 2: Two-pass health resolution

**Files:** `pkg/engine/orchestrator.go`, `pkg/engine/mechanics_engagement.go`.

- [x] **Step 1:** `pendingEffect := o.healthOutcome()` before the hook;
  `finalEffect := o.healthOutcome()` after it.
- [x] **Step 2:** Record `firstNonEmpty(pendingEffect, finalEffect)` exactly once.
- [x] **Step 3:** Set `turn.WorldTick` before the hook so its context still
  carries the tick directive.

### Task 3: Tests

**Files:** `pkg/engine/turn_end_health_test.go`, `pkg/engine/mechanics_engagement_test.go`.

- [x] **Step 1:** A hook that drives health to zero records the resolved effect on
  the same turn.
- [x] **Step 2:** A turn already at zero, healed by the hook, records exactly one
  effect.
- [x] **Step 3:** The hook context receives the pass-1 effect.
- [x] **Step 4:** `go test ./pkg/engine/`.

### Task 3b: Per-NPC health (follow-up, 2026-09-28)

- [x] **Step 1:** `healthOutcomes(turn)` resolves the player and every character
  named in the turn, requiring a real numeric value so an entity without the stat
  never fires.
- [x] **Step 2:** `mergeHealthEffects` unions the two passes, deduped by entity.
- [x] **Step 3:** Tests: a downed ally records its effect; an entity without the
  stat records none; the merge dedupes and keeps pass order.

### Task 4: Full verification

- [x] **Step 1:** `go build ./...`, `go test ./...`, `go vet ./...`.
