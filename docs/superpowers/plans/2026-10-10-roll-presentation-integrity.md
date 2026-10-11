# Roll Presentation Integrity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Stop roll mechanics appearing as narrative text: the model stops restating dice, a resolved roll attaches to the line it produced, any remaining telegraphy is hidden, and the theater shows the card.

**Architecture:** A prompt rule reduces the leak; the orchestrator anchors a pending-resolved check to its continuation's first segment (reusing the existing `@roll` anchor); a conservative `stripDiceTelegraphy` hides what still arrives, applied at the presentation boundary; and the theater renders the active beat's check card.

**Tech Stack:** Go 1.27, `pkg/engine`, `pkg/harness`; React 19, TypeScript (strict), Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-10-roll-presentation-integrity-design.md`
**Issue:** [#131](https://github.com/darkliquid/LocalRPG/issues/131)

## Global Constraints

- History keeps the model's raw text; the filtering is presentational.
- `stripDiceTelegraphy` removes only unambiguous roll reports; every positive has a negative test, and it leaves a number in ordinary prose alone.
- Speech is never filtered.
- **Scope note:** the `request_check` tool path resolves mid-generation, before the turn's segment stream exists, so there is no anchor index to stamp. That path keeps the existing behaviour (an unattached card leads the turn) and is a documented follow-up; every other resolution path is attached.

## File Map

| File | Change |
| --- | --- |
| `pkg/harness/mechanics_instructions.go` | the "do not restate the dice" rule |
| `pkg/harness/context.go`, `pkg/harness/turn_tools.go` | strengthen the existing lines |
| `pkg/engine/orchestrator.go` | anchor a pending-resolved check to the continuation |
| `frontend/src/lib/rollText.ts` | new: `stripDiceTelegraphy` |
| `frontend/src/components/TurnSegments.tsx` | filter narration and the prose fallback |
| `frontend/src/lib/turnStreamProcessor.ts` | filter live narration |
| `frontend/src/components/StoryTheater.tsx` | render the active beat's check card |

---

### Task 1: The prompt rule

**Files:**
- Modify: `pkg/harness/mechanics_instructions.go`, `pkg/harness/context.go`, `pkg/harness/turn_tools.go`
- Test: the instruction tests

- [x] **Step 1: Write the failing test** that the enabled-mechanics instructions contain "Do not restate the dice".
- [x] **Step 2: Run it to verify it fails.** `go test -run TestMechanicsInstructions ./pkg/harness/`
- [x] **Step 3: Add the rule** to the `default` branch, strengthen `context.go`'s "Never invent dice results" to "Never invent or restate dice results", and add a sentence to the `request_check` and `@roll` tool descriptions that the result is rendered for the player.
- [x] **Step 4: Run it to verify it passes.**
- [x] **Step 5: Commit.**

### Task 2: An anchor for a pending-resolved check

**Files:**
- Modify: `pkg/engine/orchestrator.go`
- Test: `pkg/gui/mechanics_turn_test.go` or a new engine test

- [x] **Step 1: Write the failing test** that a resolved pending check appears as `check_ref` on a segment of its continuation turn.
- [x] **Step 2: Run it to verify it fails.**
- [x] **Step 3: Implement:** after `rollAnchors` is declared, when `resolvedPending != nil`, append `rollAnchor{segmentIndex: 0, checkID: resolvedPending.CheckID}`. The existing stamping loop attaches it to the first segment with no `CheckRef`, which is the continuation's opening narration.
- [x] **Step 4: Run it to verify it passes.**
- [x] **Step 5: Commit.**

### Task 3: Hide remaining telegraphy

**Files:**
- Create: `frontend/src/lib/rollText.ts`, `frontend/src/lib/rollText.test.ts`
- Modify: `frontend/src/components/TurnSegments.tsx`, `frontend/src/lib/turnStreamProcessor.ts`

- [x] **Step 1: Write the failing tests.**

```ts
describe('stripDiceTelegraphy', () => {
  it('removes a bracketed roll report', () => {
    expect(stripDiceTelegraphy('You slip past (2d6+3 = 9) and run.')).toBe('You slip past and run.');
  });
  it('removes a line that is only a roll report', () => {
    expect(stripDiceTelegraphy('Roll: 2d6+3 → 9')).toBe('');
  });
  it('leaves a number in ordinary prose', () => {
    expect(stripDiceTelegraphy('You have 4 rations and a 7-foot pole.')).toBe('You have 4 rations and a 7-foot pole.');
  });
});
```

- [x] **Step 2: Run them to verify they fail.** `cd frontend && npx vitest run src/lib/rollText.test.ts`
- [x] **Step 3: Implement the filter** and apply it to narration segments and the `turn.prose` fallback in `TurnSegments`, and to live narration in `turnStreamProcessor`. Speech is not filtered.
- [x] **Step 4: Run them to verify they pass**, plus `npx vitest run` and `npx tsc --noEmit`.
- [x] **Step 5: Commit.**

### Task 4: The card in the theater

**Files:**
- Modify: `frontend/src/components/StoryTheater.tsx`
- Test: `frontend/src/components/StoryTheater.test.tsx`

- [x] **Step 1: Write the failing test** that a turn whose active beat has a `check_ref` renders a `DiceCheckCard` for that check.
- [x] **Step 2: Run it to verify it fails.**
- [x] **Step 3: Implement:** look up `currentTurn.checks` by `segments[activeIndex]?.check_ref` and render the card beside the beat when found. Do not interleave checks into the beat list: playback and audio are indexed by segment, so a check is an annotation on its beat, not a beat.
- [x] **Step 4: Run it to verify it passes.**
- [x] **Step 5: Commit.**

## Verification

- `go test ./...`, then `cd frontend && npx vitest run && npx tsc --noEmit`.
- `mise run lint`.
