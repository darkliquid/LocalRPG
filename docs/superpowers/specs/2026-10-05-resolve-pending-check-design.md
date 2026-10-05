# Resolve Pending Check Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#45 IR-1](https://github.com/darkliquid/LocalRPG/issues/45)
**Epic:** [#19 Interactive rolls](https://github.com/darkliquid/LocalRPG/issues/19)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §11 (IR-1)
**Depends on:** [#37 SYS-1](https://github.com/darkliquid/LocalRPG/issues/37)
**Scope:** `pkg/gui`, `pkg/engine`

---

## 1. Problem

Under the `ask` engagement policy the GM proposes a check and the turn ends with a `PendingCheck`
(`pkg/engine/orchestrator.go:2035-2048`), persisted on the turn and shown to the player. To act on
it, the player must **compose a whole new turn** carrying `PendingCheckRef`
(`pkg/gui/types.go:582-584`); the engine resolves the pending check before generating
(`pkg/engine/orchestrator.go:861-884`).

That is a workaround, not a roll. There is no way to say "I roll" — the player has to write
something, and the roll is a side effect of the next narration. A tabletop roll is a distinct act:
the player decides to roll, the dice land, and the fiction follows. The engine already resolves the
check correctly; what is missing is an entry point that means "roll this".

## 2. Goals

- A dedicated request that rolls a pending check and produces the GM's adjudication.
- It reuses the existing resolution and turn pipeline; no second resolution path.
- The resulting fiction is recorded as a **continuation** of the turn that proposed the check, so the
  chronicle reads as one scene, not two.
- The player may supply the dice (a physical roll) or let the server roll.
- It works whether or not the player has anything to add.

## 3. Non-goals

- The Roll card UI. That is IR-2.
- Resuming the *same* stream mid-flight. That is IR-3; this spec produces a continuation turn, which
  the existing history already models (`Turn.ContinuationOf`, `Turn.ResolvesCheckRef`).
- Renegotiating the stakes. That is IR-5.

## 4. Design

### 4.1 The endpoint

```
POST /api/game/{id}/turn/{n}/resolve-check
```

Body:

```go
type ResolveCheckRequestDTO struct {
	PendingRef   string `json:"pending_check_ref"`
	ManualResult *int   `json:"manual_result,omitempty"` // a player-supplied total, optional
	Note         string `json:"note,omitempty"`          // optional player remark
}
```

Response: the same **NDJSON turn stream** as `POST /api/game/{id}/turn`
(`pkg/gui/server.go:1482-1543`), so the client already knows how to render it. The endpoint is a thin
wrapper around the turn pipeline with `PendingCheckRef` set.

### 4.2 Reusing the pipeline

`Service` gains `ResolveCheck(ctx, gameID string, turnNumber int, req ResolveCheckRequestDTO, emit func(TurnEvent) error) error`:

1. Load turn `n` from history; find its `PendingCheck`; if absent or its ref does not match, return
   `400`.
2. Acquire the per-campaign turn lock exactly as `BeginTurn` does (`pkg/gui/service.go`), returning
   `409` while another turn is in flight.
3. Submit a turn through the existing `TurnSession.Run` path with:
   - `Mode = "roll"` (the existing Roll mode),
   - `PendingCheckRef = req.PendingRef`,
   - `Input = req.Note`,
   - and, when `ManualResult` is set, a flag the engine honours to use the supplied total instead of
     rolling.

The engine's existing `pendingCheckRef` handling (`pkg/engine/orchestrator.go:861-884`) resolves the
check and prepends `[PLAYER ROLL: …]` to the generation, so the GM adjudicates the outcome it already
received. No new resolution code.

### 4.3 Manual results

A manual result is a player-supplied total. The engine's `resolveCheck` gains an optional
`ForcedTotal *int` on the request context: when set, the resolver uses that total instead of rolling,
and records the source as `manual`. This is the hook IR-4 (manual roll entry) builds on; IR-1 only
carries the value through.

A manual result is **trusted**, because the app is local-first; it is recorded with its source so the
chronicle is honest about where the number came from.

### 4.4 Recording the continuation

The resulting turn is recorded with:

- `ContinuationOf = n` (the turn that proposed the check),
- `ResolvesCheckRef = req.PendingRef`,
- and its own checks containing the resolved result.

The timeline already writes these fields (`pkg/engine/timeline.go`), and the chronicle can group a
turn with its continuation. The proposing turn keeps its `PendingCheck` (now resolved), so a reload
shows the roll it produced.

### 4.5 Errors

| Condition | Response |
| --- | --- |
| turn `n` does not exist | `404` |
| turn `n` has no pending check | `400` |
| the ref does not match the turn's pending check | `400` |
| another turn is in flight | `409` |
| the check is already resolved | `409` (idempotency: the client refreshes) |
| the provider fails | the existing turn-failure behaviour |

## 5. Behaviour

| Situation | Result |
| --- | --- |
| a pending check, server roll | the check resolves; the GM adjudicates; a continuation turn is recorded |
| a pending check, manual total | the check resolves to the manual total; source recorded as manual |
| a pending check, with a note | the note is included in the generation input |
| no pending check | `400` |
| a turn already in flight | `409` |
| the ref mismatches | `400` |

## 6. Testing

- `pkg/gui`: the endpoint resolves a pending check and streams a turn whose `ContinuationOf` and
  `ResolvesCheckRef` are set; a missing or mismatched ref is `400`; a concurrent turn is `409`; a
  resolved check is `409`.
- `pkg/engine`: a forced total resolves the check to that total and marks the source manual; without
  it, the resolver rolls as today.
- A regression guard: the existing `PendingCheckRef` path through `POST /turn` is unchanged.

## 7. Rollout

Additive endpoint and one optional request field. Nothing changes for a campaign that never uses the
`ask` policy.

## 8. Risks

- **Two entry points for a pending check.** `POST /turn` with `PendingCheckRef` and the new endpoint
  both work. That is deliberate: the endpoint is the direct path, the turn path remains for a player
  who writes prose and rolls as part of it. The engine path is shared, so they cannot diverge.
- **Manual-result trust.** Recorded, not enforced. The alternative (verifying a physical roll) is
  impossible and unwanted.
- **Continuation semantics.** A continuation is a new turn number; a client must group it with its
  parent. The DTO already exposes `ContinuationOf`; IR-2 renders the grouping.
