# Malformed Response Retry Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#69 RB-4](https://github.com/darkliquid/Projects/LocalRPG/issues/69)
**Epic:** [#23 Malformed output and playback integrity](https://github.com/darkliquid/Projects/LocalRPG/issues/23)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §5 (RB-4)
**Depends on:** [#66 RB-1](https://github.com/darkliquid/Projects/LocalRPG/issues/66)
**Scope:** `pkg/engine`, `pkg/harness`

---

## 1. Problem

When the GM's reply is malformed, the turn is usually **aborted**. RB-1 repairs a structurally damaged
`@` record, but a reply that is still broken after repair — an empty stream, a tool call with
unparseable arguments, a reply with no usable record at all — falls through to the existing paths:

- a repeated validation failure nulls the submission (`pkg/engine/orchestrator.go:1516-1521` in the
  superseded flow; the progressive stream's equivalent is the empty-narration abort);
- an empty reply is `FailureEmptyResponse` (`pkg/engine/orchestrator.go:1612-1622`);
- a tool-argument parse error is fed back **once** as a `tool` message (`:2013-2015`), but a second
  failure ends the loop (`:2103`, "tool loop ended without an answer").

There is no **bounded retry with a repair instruction**: the model is not told what went wrong and
asked to try again. The completion recovery (`pkg/engine/recovery.go`) handles a *cut* reply (trim or
continue), not a *malformed* one.

So a single bad reply loses the turn, when a nudge ("your `@roll` record was not valid JSON; re-emit
it") would usually fix it.

## 2. Goals

- On a malformed reply, **re-ask the model once or twice with a repair instruction** naming the
  problem, before aborting.
- Bounded: a small cap, configurable, so a broken model cannot loop.
- It composes with the existing recovery: a cut reply is still trimmed/continued; this handles a
  malformed one.
- The failure taxonomy records the retry, so a trace explains it.
- The existing prose fallback remains the last resort.

## 3. Non-goals

- Repairing the reply locally (RB-1) and the playback ledger (RB-3).
- Retrying a **provider** error (that is the fallback chain, `pkg/engine/orchestrator.go:1669-1723`).
- Changing the stream protocol.

## 4. Design

### 4.1 What counts as malformed

A reply is malformed, for the retry's purpose, when any of:

- the stream produced **no usable event** (no narration, no speech, no valid record) after RB-1's
  repair;
- a **tool call's arguments** fail to parse and a repair round did not fix them;
- a **record** needed for the turn (an `@roll` that ends it) is still invalid after repair and the
  turn would otherwise proceed as if no roll happened.

A **cut** reply (a length finish, an incomplete sentence) is **not** malformed; that is the existing
completion recovery's job, and the two must not both fire.

### 4.2 The retry

```go
// repairInstruction builds a short, specific nudge for a malformed reply.
func repairInstruction(reason harness.FailureKind, detail string) string
```

Examples:

- a missing roll: "Your previous reply contained no valid `@roll` record. Re-emit the reply, ending
  with a single `@roll {…}` line whose JSON is valid."
- unparseable tool arguments: "The `request_check` call had arguments that were not valid JSON.
  Call it again with valid JSON."
- an empty reply: "Your previous reply was empty. Write the turn now."

The retry appends the instruction to the conversation and re-runs the generation **once** (or up to
`MaxRepairAttempts`, default 2). The retry's reply replaces the malformed one; the malformed one is
discarded, never recorded.

### 4.3 Bounds and composition

- `MaxRepairAttempts` is configurable (`completion.max_repair_attempts` or a sibling), default 2.
- A retry counts against the turn's tool-round cap and the completion timeout, so a pathological model
  cannot exceed the turn's existing bounds.
- The order of recovery is: **repair** (this) → **completion** (trim/continue) → **provider fallback**
  → **abort**. A reply that is both malformed and cut is repaired first, then trimmed if the repair is
  still cut.
- On exhaustion, the turn aborts as today, with the failure taxonomy recording each attempt.

### 4.4 The failure taxonomy

`harness.Attempt` (`pkg/harness/failure.go:30`) already records attempts with a kind. The repair adds
an attempt with `FailureParseError` and a detail (the reason), so the GUI's failure surface and the
trace show "reply was malformed; re-asked once; then succeeded" or "…; aborted".

### 4.5 What is not retried

- A **provider** error (timeout, HTTP 5xx): the fallback chain handles it; a re-ask to a broken
  provider wastes a call.
- A reply that is **valid but poor**: no retry; the GM said something, and the turn uses it.

## 5. Behaviour

| Reply | Result |
| --- | --- |
| valid | used; no retry |
| a repaired record (RB-1) | used; no retry |
| empty | re-asked once; the retry's reply is used |
| a missing roll | re-asked with the roll instruction |
| a bad tool call | re-asked with the tool instruction |
| malformed twice | aborted; both attempts recorded |
| cut | completion recovery, not this |
| a provider timeout | the fallback chain, not this |

## 6. Testing

- `pkg/engine`: an empty reply triggers one retry and uses the second reply; a missing roll triggers a
  roll-specific instruction; two malformed replies abort with two recorded attempts; a valid reply is
  not retried; a cut reply goes to completion recovery, not the repair.
- `pkg/harness`: the attempt is recorded with the right kind and detail.
- `pkg/gui`: the failure surface shows the retries.
- A regression guard: a valid turn makes exactly one generation call (no spurious retry).

## 7. Rollout

Additive: a retry path with a configurable cap. The default cap is small, so a normal turn is
unaffected; a malformed one now often succeeds.

## 8. Risks

- **Cost.** A retry is another call. The cap bounds it, and it only fires on a malformed reply.
- **Doubling the work with completion recovery.** The order is explicit (repair, then completion), and
  a reply is classified as one or the other, not both.
- **A model that is malformed every time.** The cap aborts, as today, with a clear trace; the prose
  fallback still applies where it did.
