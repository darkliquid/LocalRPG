# Extract the Phases of ProcessActionStream Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the 987-line `ProcessActionStream` into a table of contents over named phase methods, with no behaviour change, so the eighty-line exemption for it can be removed.
**Architecture:** The turn's shared locals move into one `turnRun` value created by `beginTurn`; each phase becomes a method on `TurnOrchestrator` that reads and writes that value. Orchestrator fields that outlive a turn stay where they are.
**Tech Stack:** Go 1.27, `go/ast` for the function-length gate, the existing `pkg/engine` suite as the proof.
**Spec:** `docs/superpowers/specs/2026-10-15-turn-phases-design.md`

## File Map

| File | Why |
|---|---|
| `pkg/engine/orchestrator.go` | add `turnRun`; shrink `ProcessActionStream` to phase calls; add the phase methods in call order |
| `pkg/systemtest/function_length_test.go` | remove the `ProcessActionStream` exemption; rewrite `runGenerationLoop`'s reason to name RD-11 |

## Global Constraints

- **Move, do not rewrite.** The same error strings, the same order, the same `defer`s, the same instrumentation attributes. Two exceptions, both behaviour-preserving and named in the spec: the roll branch's three `break`s become one returned decision, and `o.mechanicsEngagement != "off"` becomes `!= config.EngagementOff`.
- **The deferred work keeps its scope.** `turnSpan.End`, the metrics closure and `finaliseSpan.End` run at the turn's return, in that LIFO order, from one `defer run.finish()`.
- **`o.parser`, `o.roster`, `o.sceneOnly`, `o.forceToolChoice`, `o.pendingCheckRef`, `o.worldTick` and `o.logger` stay on the orchestrator.** Other methods read them, so the run value must not shadow them.
- **The suite passes unchanged**, and it is run before the first edit so a red test is not mistaken for a broken move.
- **The gate is the length's proof.** `TestFunctionLengthGate` fails while the exemption is gone and the function is still long, and fails if an exemption is left for a function that is short.

---

### Task 1: Record the baseline

- [x] **Step 1: Run the engine suite green**

Run: `go test ./pkg/engine/`
Expected: pass, so the extraction is measured against a green tree.

- [x] **Step 2: Record the length**

Run: `go run /tmp/lf/main.go . | grep ProcessActionStream`, or read it from `TestFunctionLengthGate`.
Expected: `ProcessActionStream` at 987 lines, the only entry besides `runGenerationLoop` over eighty.

---

### Task 2: The run value and the prologue

- [x] **Step 1: Add `turnRun` and `finish`**

Declare the type with a doc comment saying it is one turn's in-flight state, created by `beginTurn`
and discarded when the turn ends. `finish` ends `finaliseSpan` when it is non-nil, sets the
`turn.outcome` attribute, records `turnDuration` and `turnCompleted` with the reassigned mode, and
ends `turnSpan`. The span attributes are `turn.mode` and `turn.outcome`, exactly as the closure
did.

- [x] **Step 2: `beginTurn`**

Move 649-704: load history, `turnNum`, the scene-only reset, the usage turn, the forced-check
cadence and its log event, `trace.OrNil`, the `turn.begin` event, the span with its game, number
and mode attributes, `turnStarted`, and the declaration of the fields the later phases fill.

- [x] **Step 3: Compile**

Run: `go build ./pkg/engine/`
Expected: the phases do not exist yet, so keep the old body in place and compile only once the
entry point is rewritten in Task 8; `turnRun` itself must compile.

---

### Task 3: The slash commands

- [x] **Step 1: `handleSlashCommand`, `handleUndo`, `handleRecap`, `handleGo`**

The dispatcher trims the input once and tests `/undo`, `/recap` and a `/go` prefix in that order,
returning `(turn, true, nil)` for a handled command. Each handler moves its block whole, including the recap's
stale-summary regeneration and its `provider.error` event, and the move's player and location
mentions.

- [x] **Step 2: Compile and test the commands**

Run: `go test ./pkg/engine/ -run 'TestProcessAction|TestUndo|TestRecap|TestGo'`
Expected: pass.

---

### Task 4: Mode resolution and context assembly

- [x] **Step 1: `resolveTurnMode`**

Move 789-829, writing `run.mode`, `run.generationPrompt`, `run.gmDirective`, `run.pendingCheck`,
`run.rollRes` and `run.outcome`. The opening branch writes `run.mode` so the deferred metrics see
the reassignment.

- [x] **Step 2: `gatherTurnContext` and `assembleTurnPrompt`**

Move 831-905 and 907-950. Keep the directive drain and its world-tick capture after the prompt is
assembled, and keep `assembly` in the run value so `runGenerationLoop` can take its address.

- [x] **Step 3: Compile**

Run: `go build ./pkg/engine/`
Expected: pass, once the fields the phases read are declared.

---

### Task 5: Generation

- [x] **Step 1: `resolvePendingCheckBeforeGeneration`**

Move 952-982, including the `pending.resolve_error` event and the `[PLAYER ROLL: ...]` directive.

- [x] **Step 2: `installStreamObservers`**

Move 984-1032: the TTFT wrapper, the roster and parser seeding, the segment wrapper for both the
present and absent `onChunk` cases, and the player's leading line. Store the wrapped callback as
`run.onChunk`.

- [x] **Step 3: `beginRollLoop`, `drainTurnEvents`, `runRollContinuationLoop`, `streamTurnReply` and `resolveEndingRoll`**

Move 1034-1076 and 1077-1167. `beginRollLoop` keeps the resolved-pending anchor; the `drainEvents`
closure becomes the `drainTurnEvents` method, because it reads `o.parser`; the loop skeleton, the
one generation call and its failure reporting become `runRollContinuationLoop` and
`streamTurnReply`; and the roll branch becomes `resolveEndingRoll`, whose three `break`s return
`false` instead, including the cap event, the resolution, the anchor and the continuation
directive.

- [x] **Step 4: `finaliseNarration` and `classifyAndRecoverReply`**

Move 1164-1254. The roll outcome, the `generation.complete` event and `stripRecordLines` go in
`finaliseNarration`; the finalise span, the cut classification, the recovery and the empty-reply
failure go in `classifyAndRecoverReply`. Store `finaliseSpan` in the run so `finish` can end it.

- [x] **Step 5: Test generation**

Run: `go test ./pkg/engine/ -run 'TestTurn|TestRoll|TestPending|TestRecovery|TestStream'`
Expected: pass.

---

### Task 6: The turn's record

- [x] **Step 1: `buildTurn`, `startTurnExtraction`, `applyTurnRecords`**

Move 1256-1300, 1302-1325. The extractor goroutine's spans and its `extract.error` events move
with it; `applyTurnRecords` also awaits the channel and logs the failure.

- [x] **Step 2: `assembleTurnSegments` and `annotateTurnSegments`**

Move 1327-1439. `assembleTurnSegments` covers the events, the attributions, the persona identity
remap, the roll outcome segment, the player's line and the roll anchors; `annotateTurnSegments`
covers the segment log, the speech mentions, the scene break and the portrait URLs. Both are
called from the entry point rather than from a two-line dispatcher.

- [x] **Step 3: `checkTurnContinuity` and `applyProposedMove`**

Move 1441-1456 and 1458-1472.

- [x] **Step 4: `recordTurn` and its phases**

Move 1476-1583: the cancellation check, `reconcileExtractedEntities`, `applyTurnStateChanges`,
`runTurnLifecycle` (advancement, world tick, the two health passes and the turn-end hooks),
`persistTurn` (the single-turn replace and the structured record), `assignPersonaVoices`,
`persistWorkingSet` and `logRecordedTurn`.

- [x] **Step 5: `enqueueSceneArt` and `enqueuePortraitVersions`**

Move 1585-1629, splitting the scene prompt and the portrait versions.

---

### Task 7: The entry point

- [x] **Step 1: Rewrite `ProcessActionStream`**

It becomes `beginTurn`, `defer run.finish()`, `handleSlashCommand`, and the phase calls in order,
ending with `return &run.turn, nil`.

- [x] **Step 2: Run the engine suite**

Run: `go test ./pkg/engine/`
Expected: pass unchanged.

- [x] **Step 3: Move `ProcessAction` beside it**

`ProcessAction` stays the one-line non-streaming wrapper.

---

### Task 8: The gate

- [x] **Step 1: Remove the `ProcessActionStream` exemption**

Delete the entry from `exemptFunctions`, since the function is now under eighty lines and the gate
fails on an exemption that is no longer needed.

- [x] **Step 2: Rewrite `runGenerationLoop`'s reason**

It is still over eighty and now waits on the `pkg/engine` batch of RD-11 (finding R-12), not on
RD-3.

- [x] **Step 3: Run the gate**

Run: `go test ./pkg/systemtest/ -run TestFunctionLength`
Expected: pass, with one exemption left.

---

### Task 9: Verification

- [x] **Step 1: Vet and lint**

Run: `go vet ./... && mise run lint`
Expected: pass.

- [x] **Step 2: The full suite**

Run: `mise run test`
Expected: pass, including the coverage floor.

- [x] **Step 3: Re-measure**

Run: `go test ./pkg/systemtest/ -run TestFunctionLength -v`
Expected: `ProcessActionStream` absent from the over-eighty list.

- [x] **Step 4: Close the bead**

`bd close LocalRPG-92l.11 --reason "..."`, and create a follow-up bead for the `pkg/engine` batch of
RD-11, which RD-3 unblocks.
