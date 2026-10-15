# Extract the Phases of ProcessActionStream Design

**Date:** 2026-10-15
**Status:** Proposed
**Issue:** [#169](https://github.com/darkliquid/LocalRPG/issues/169)
**Epic:** [#151](https://github.com/darkliquid/LocalRPG/issues/151)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (RD-3, finding R-3)
**Scope:** `pkg/engine/orchestrator.go` and `pkg/systemtest/function_length_test.go`. No behaviour change.

## 1. Problem

**`ProcessActionStream` is 987 lines**, from `pkg/engine/orchestrator.go:648` to `:1634`. It was the
only function in the repository with a written exemption from the eighty-line budget in
`pkg/systemtest/function_length_test.go`, and the exemption named this proposal as the reason.

The length is not the complaint. The complaint is that every phase of a turn is at one
indentation level, so the phases are invisible and the reader has no table of contents:

| Lines | What happens |
|---|---|
| 649-704 | load history, pick the turn number, start the turn span, register the deferred metrics |
| 706-787 | three slash commands, each an early return: `/undo`, `/recap`, `/go` |
| 789-829 | mode resolution: opening, director correction, player-requested roll, mechanics hook |
| 831-950 | context assembly: location, world tick, recent turns, summary, threads, working set, prompt |
| 952-982 | resolve a pending check before generation |
| 984-1032 | wrap the chunk callback twice (TTFT, then stream segments), seed the roster and parser |
| 1034-1167 | the roll-continuation loop |
| 1169-1254 | classify how the reply was cut, recover it, refuse a genuinely empty one |
| 1256-1325 | build the `Turn`, start extraction concurrently, apply the streamed records |
| 1327-1439 | build the playback script: segments, attributions, anchors, portraits |
| 1441-1472 | continuity findings, and a proposed move |
| 1480-1530 | reconcile extracted entities, apply state changes, advancement, health, turn-end hooks |
| 1532-1583 | record the turn, assign persona voices, persist the working set, log |
| 1585-1629 | enqueue the scene image and any portrait version |

String prefix matching, OpenTelemetry span attributes and entity slugging share that level. A
reader asking "where does a turn start" gets a 987-line answer, and `git log` blames the whole
function for a one-line change inside it.

## 2. Goals

- `ProcessActionStream` reads as a sequence of named phases, each one line long.
- Every phase is a method on `TurnOrchestrator` with one job and a doc comment.
- No behaviour change: the same errors, in the same order, with the same deferred work in the
  same scope.
- The `ProcessActionStream` exemption is removed from the function-length gate.

## 3. Non-goals

- **`runGenerationLoop` (424 lines).** It shares boundaries with this function, so it lands after
  it, in the `pkg/engine` batch of RD-11 (finding R-12). Its exemption is kept here and its
  reason is rewritten to name that batch rather than this proposal.
- **Reorganising the files of `pkg/engine`.** The phases stay in `orchestrator.go`, beside the
  entry point, so the table of contents and its entries are adjacent. A file split is RD-11's
  `pkg/engine` batch, which already has `InitGame` and `stageEntities` to place.
- **Rewriting a phase.** A phase that turns out to be wrong is a different proposal. This one
  moves code, with the two exceptions named below.
- **Coverage for its own sake.** The suite reaches these phases through the orchestrator's
  public entry points; a test is added only where a phase turns out to be one the suite never
  reached.

## 4. Design

### The turn's state value

The phases share about thirty locals, so they travel in one value rather than as out-parameters:

```go
// turnRun is the state of one turn in flight. beginTurn creates it, the phases
// of ProcessActionStream read and write it, and it is discarded when the turn
// returns.
type turnRun struct {
    ctx       context.Context
    mode      string
    input     string
    pastTurns []Turn
    turnNum   int
    ...       // the mode, context, stream, narration and record fields
}

// finish ends the turn's spans and records its duration and completion
// metrics, in the order the entry point's defers ran in when this was one
// function.
func (r *turnRun) finish() { ... }
```

The value is not an interface and not a state machine: it is the closure of the old function
body, named. Orchestrator fields that outlive a turn (`o.parser`, `o.roster`, `o.sceneOnly`,
`o.forceToolChoice`, `o.pendingCheckRef`, `o.worldTick`, `o.logger`) stay on the orchestrator,
because other methods already read them.

### The phases

`ProcessActionStream` becomes:

```go
func (o *TurnOrchestrator) ProcessActionStream(ctx context.Context, mode, actionInput string, onChunk func(text string) error) (*Turn, error) {
    run, err := o.beginTurn(ctx, mode, actionInput)
    if err != nil {
        return nil, err
    }
    defer run.finish()

    if turn, handled, err := o.handleSlashCommand(run.ctx, run); handled || err != nil {
        return turn, err
    }

    if err := o.resolveTurnMode(run); err != nil {
        return nil, err
    }
    o.gatherTurnContext(run)
    if err := o.assembleTurnPrompt(run.ctx, run); err != nil {
        return nil, err
    }
    o.resolvePendingCheckBeforeGeneration(run.ctx, run)
    o.installStreamObservers(run, onChunk)
    o.beginRollLoop(run)
    if err := o.runRollContinuationLoop(run.ctx, run); err != nil {
        return nil, err
    }
    if err := o.finaliseNarration(run.ctx, run); err != nil {
        return nil, err
    }

    o.buildTurn(run)
    o.startTurnExtraction(run.ctx, run)
    o.applyTurnRecords(run)
    o.assembleTurnSegments(run)
    o.annotateTurnSegments(run)
    o.checkTurnContinuity(run.ctx, run)
    if err := o.applyProposedMove(run); err != nil {
        return nil, err
    }
    if err := o.recordTurn(run.ctx, run); err != nil {
        return nil, err
    }
    o.enqueueSceneArt(run)
    o.enqueuePortraitVersions(run)
    return &run.turn, nil
}
```

The phases, in call order:

| Phase | Lines moved | Job |
|---|---|---|
| `beginTurn` | 649-704 | history, turn number, scene-only reset, usage turn, forced-check cadence, turn span, run value |
| `handleSlashCommand` | 706-787 | dispatch to `handleUndo`, `handleRecap` or `handleGo` |
| `handleUndo` | 707-721 | rewind one turn |
| `handleRecap` | 727-751 | regenerate a stale summary, then render the recap |
| `handleGo` | 754-787 | resolve a location, move the player, record the move |
| `resolveTurnMode` | 789-829 | opening, correction, player roll, mechanics hook |
| `gatherTurnContext` | 831-905 | location, world tick, recent turns, summary, threads, working set, tool capability, echo and opening decisions |
| `assembleTurnPrompt` | 907-950 | `ContextAssembler.Assemble`, `ExecuteTurnBegin`, drain hooks into the directive |
| `resolvePendingCheckBeforeGeneration` | 952-982 | resolve a pending check that generation must narrate |
| `installStreamObservers` | 984-1032 | TTFT, then segments; seed the roster and parser; observe the player's line |
| `beginRollLoop` | 1056-1061 | anchor a check resolved before the continuation |
| `drainTurnEvents` | 1066-1076 | move the parser's events into the turn |
| `runRollContinuationLoop` | 1077-1167 | generate, collect, and continue while the reply ends on a roll |
| `streamTurnReply` | 1078-1115 | one generation call and its failure reporting |
| `resolveEndingRoll` | 1122-1160 | the cap, the resolution, the anchor and the continuation directive |
| `finaliseNarration` | 1169-1254 | finalise span, context attributes, the roll outcome, the log and the record strip |
| `classifyAndRecoverReply` | 1185-1233 | cut classification, recovery, empty-reply failure |
| `buildTurn` | 1256-1275 | the `Turn` value |
| `startTurnExtraction` | 1277-1300 | the concurrent extractor and its completion channel |
| `applyTurnRecords` | 1302-1325 | mentions, streamed records, personae, checks, report, await the extractor |
| `assembleTurnSegments` | 1327-1393 | events to segments, attributions, persona identity, a roll outcome, the player's line, roll anchors |
| `annotateTurnSegments` | 1395-1439 | the segment log, speech mentions, the scene break, portrait URLs |
| `checkTurnContinuity` | 1441-1456 | continuity findings |
| `applyProposedMove` | 1458-1472 | a move from a record or from extraction |
| `recordTurn` | 1476-1583 | the cancellation check, then `reconcileExtractedEntities`, `applyTurnStateChanges`, `runTurnLifecycle`, `persistTurn`, `assignPersonaVoices`, `persistWorkingSet`, `logRecordedTurn` |
| `enqueueSceneArt` | 1585-1618 | the scene prompt and the reference image |
| `enqueuePortraitVersions` | 1620-1629 | a new portrait version for anyone whose appearance changed |

Phases that would be a two-line dispatcher over two other phases are not extracted; the entry
point calls `gatherTurnContext`/`assembleTurnPrompt`, `assembleTurnSegments`/`annotateTurnSegments`
and `enqueueSceneArt`/`enqueuePortraitVersions` directly.

### What stays put, and why

- **The deferred work keeps its scope.** `turnSpan.End`, the metrics closure and
  `finaliseSpan.End` used to run at the function's return, in that LIFO order. They run at the
  same moment, in the same order, from `run.finish`, deferred once from the entry point. A span
  that ends where a phase ends would be a trace change, and this proposal does not make one.
- **`mode` is reassigned by the opening branch**, and the deferred metrics read the reassigned
  value, so `resolveTurnMode` writes `run.mode`, not a local.
- **`o.pendingCheckRef` is cleared by the pending phase**, because it is the next turn's input,
  not this turn's state (line 981).
- **`drainTurnEvents` stays a method on `TurnOrchestrator`**, because it drains `o.parser`, which
  `applyRecords`, `recordReport` and `runGenerationLoop` also use. It takes the run for the two
  accumulator fields.
- **The cancellation check before recording** (1476-1478) reads `ctx.Err()` and stays at the head
  of `recordTurn`, where the old code put it.
- **Error strings are moved, not retyped**, including the `fmt.Errorf` wrappers, so a failure
  reaches the caller through the same path with the same message.

### The two deliberate changes

Everything else is a move. Two statements could not survive it unchanged, and both preserve
behaviour exactly:

- **The roll branch's three `break`s become one returned decision.** The block that ends a reply
  on a `@roll` record stops the loop from inside it; a method cannot break its caller's `for`, so
  `resolveEndingRoll` reports whether to continue and the loop breaks on `false`. The cap event,
  the resolution, the anchor and the continuation directive all stay where they were, in the same
  order.
- **`o.mechanicsEngagement != "off"` becomes `!= config.EngagementOff`** (line 808). The literal
  is the constant RD-8 introduced for exactly this comparison, and the two are byte-identical.

## 5. Behaviour

None. Every phase is a move inside one package: the compiler catches a phase that was left
behind or a helper that captured the wrong variable, and the existing suite catches a reordered
one. The visible differences are `git log`, which stops blaming a thousand-line function for a
one-line change, and the trace, which `orchestrator_telemetry_test.go` asserts is unchanged.

The one deliberate exception is control flow, not behaviour: `resolveEndingRoll` returns the
decision the three `break`s used to make, which is what lets the loop body be a method at all.

## 6. Testing

- **The suite is the proof.** `pkg/engine` reaches these phases through
  `orchestrator_stream_test.go`, `orchestrator_test.go`, `orchestrator_session_test.go`,
  `turnstream_integration_test.go`, `pending_check_test.go`, `roll_mode_test.go`,
  `replace_turn_test.go`, `orchestrator_failure_test.go`, `orchestrator_metrics_test.go`,
  `orchestrator_telemetry_test.go` and `orchestrator_timeline_test.go`.
- **No path stopped running.** Measured by comparing the two profiles function by function rather
  than by their statement totals: no function in `pkg/engine` that was fully covered before the
  extraction lost that afterwards. The statement total moved from 82.1% to 81.5% in the same
  comparison, and that number is not evidence of anything: a block's statement count is repeated
  once per code range the block produces, so moving a comment moves the total without changing a
  statement. `mise run coverage:diff` (`scripts/coverage-diff.sh`) is the check.
- **A phase with no coverage gets one**, added with the extraction rather than after it.
- **The gate proves the length.** `TestFunctionLengthGate` fails if `ProcessActionStream` is
  still over eighty lines once its exemption is gone, and fails again if an exemption is left
  behind for a function that is no longer over the budget.
- `go vet ./...`, `mise run test`, `mise run lint`.

## 7. Rollout

One pull request, one commit: the spec, the plan, the extraction and the gate change. The bead is
closed when the suite and the lint pass; `runGenerationLoop` is left to RD-11's `pkg/engine`
batch.

## 8. Risks

- **A mechanical move is where behaviour hides.** The mitigation is that each phase moves whole,
  that the deferred work is inventoried before the first edit and checked after it, and that the
  suite covers every phase except the optional art and voice paths.
- **A shared struct invites state that outlives the turn.** The value is created by `beginTurn`
  and dies with the function; nothing caches it on the orchestrator.
- **Over-extraction.** A phase that is two lines and called once, such as the cancellation check,
  stays inline. The line is drawn where the phase has a name a reader would use.
