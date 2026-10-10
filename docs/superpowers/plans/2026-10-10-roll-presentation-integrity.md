# Roll Presentation Integrity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the roll card authoritative: every resolved check attaches to the segment that narrates it, the model stops restating dice, remaining telegraphy is hidden, and the theater shows the card.

**Architecture:** The orchestrator reuses its `@roll` anchor to stamp `CheckRef` on the tool and pending paths; a prompt rule reduces the leak; a conservative `stripDiceTelegraphy` at the presentation boundary hides what remains; a shared `groupChecksWithSegments` gives the theater the same grouping as the chronicle.

**Tech Stack:** Go 1.27, `pkg/engine`, `pkg/harness`; React 19, TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-10-roll-presentation-integrity-design.md`
**Issue:** [#131](https://github.com/darkliquid/LocalRPG/issues/131)

## Global Constraints

- History keeps the model's raw text; the filtering is presentational.
- `stripDiceTelegraphy` removes only unambiguous roll reports; a negative test is required for every positive.
- Speech is never filtered.

## File Map

| File | Change |
| --- | --- |
| `pkg/engine/orchestrator.go` | stamp `CheckRef` on the tool and pending paths |
| `pkg/engine/mechanics_turn_test.go` | attachment tests |
| `pkg/harness/mechanics_instructions.go`, `context.go`, `turn_tools.go` | prompt rule |
| `frontend/src/lib/rollText.ts` | new: `stripDiceTelegraphy` |
| `frontend/src/lib/checks.ts` | new: `groupChecksWithSegments` |
| `frontend/src/components/TurnSegments.tsx` | use both |
| `frontend/src/lib/turnStreamProcessor.ts` | filter live narration |
| `frontend/src/components/StoryTheater.tsx` | render the card |

---

### Task 1: Attach the tool-resolved check

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/mechanics_turn_test.go`

- [ ] **Step 1: Write a failing test** that a `request_check` resolution stamps `CheckRef` on the following segment.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Append a `checkAnchor` at the tool site and stamp it after the loop.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 2: Attach the pending-resolved check

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/engine/mechanics_turn_test.go`

- [ ] **Step 1: Write a failing test** that a pending resolution stamps `CheckRef` and links the continuation.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Set `CheckRef: result.RollCheckRef` on the outcome segment.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 3: The prompt rule

**Files:**
- Modify: `pkg/harness/mechanics_instructions.go`, `pkg/harness/context.go`, `pkg/harness/turn_tools.go`
- Test: the instruction tests

- [ ] **Step 1: Write a failing test** that the "Do not restate the dice" rule is present when engagement is on.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Add the rule and strengthen the existing lines.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 4: Hide telegraphy in the frontend

**Files:**
- Create: `frontend/src/lib/rollText.ts`
- Modify: `frontend/src/components/TurnSegments.tsx`, `frontend/src/lib/turnStreamProcessor.ts`

- [ ] **Step 1: Write failing table tests** for the removals and, crucially, the non-removals ("you have 4 rations", "a 7-foot wall").
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the filter and apply it to narration and the prose fallback.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 5: The theater card

**Files:**
- Create: `frontend/src/lib/checks.ts`
- Modify: `frontend/src/components/TurnSegments.tsx`, `frontend/src/components/StoryTheater.tsx`

- [ ] **Step 1: Write failing tests** for `groupChecksWithSegments` (attached before its segment, unattached leads) and that the theater renders a `DiceCheckCard`.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Extract the helper and render the card in the theater.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**
