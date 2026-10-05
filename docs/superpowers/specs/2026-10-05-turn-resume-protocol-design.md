# Turn Resume Protocol Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#47 IR-3](https://github.com/darkliquid/LocalRPG/issues/47)
**Epic:** [#19 Interactive rolls](https://github.com/darkliquid/LocalRPG/issues/19)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §11 (IR-3)
**Depends on:** [#45 IR-1](https://github.com/darkliquid/LocalRPG/issues/45), [#46 IR-2](https://github.com/darkliquid/LocalRPG/issues/46)
**Scope:** `pkg/engine`, `pkg/gui`, `frontend`

---

## 1. Problem

IR-1 resolves a pending check by producing a **continuation turn**: the proposing turn keeps its
pending check, and the adjudication is a new turn carrying `ContinuationOf`. That is simple and
safe, but it splits one scene across two turn records. The chronicle shows "a check is waiting" on
turn N and the roll and its consequence on turn N+1, so the fiction is two entries where a table
would tell it as one.

IR-2 groups the pair for display, which softens but does not remove the split: the history, the
export, and any consumer that counts turns see two.

## 2. Goals

- An **opt-in** mode where a pending check and its adjudication are one turn record.
- The turn is persisted while pending (so a reload shows the waiting check), then completed in place.
- The default stays IR-1's continuation, so nothing changes for existing campaigns.
- The two modes share the resolution and generation path; only persistence differs.

## 3. Non-goals

- Replacing the continuation model. It is the default and remains the simplest correct option.
- Holding the turn stream open while waiting for the player. That would pin the campaign lock; both
  modes release it at the pending boundary.
- Changing the resolve-check endpoint's request or the Roll card (IR-1, IR-2).

## 4. Design

### 4.1 The mode

A configuration value selects the behaviour:

```yaml
interactive:
  rolls: continuation   # continuation (default) | single-turn
```

A campaign setting overrides the global. `continuation` is IR-1 exactly; `single-turn` is this
spec. The default is `continuation`, so this feature is opt-in and existing campaigns are untouched.

### 4.2 The draft turn

`engine.Turn` gains:

```go
	// Draft marks a turn written while a check is pending, in single-turn mode. The
	// chronicle hides it and it is completed in place when the check resolves.
	Draft bool `json:"draft,omitempty"`
```

When a turn ends on a pending check in `single-turn` mode, it is written as a normal turn with
`Draft: true` and its `PendingCheck`. It is indexed and persisted like any turn, so a reload shows
the waiting check.

### 4.3 Completing in place

`Timeline` gains:

```go
// ReplaceTurn replaces the last recorded turn with a complete one. It is only
// valid for the final turn and is used to complete a draft.
func (t *Timeline) ReplaceTurn(ctx context.Context, turn Turn) error
```

`ReplaceTurn` rewinds to `turn.Number - 1` (the existing `RewindToTurn` machinery, which trims the
log, deletes the indexed turns, and prunes entity history numbers) and then records the complete
turn. Because the draft is always the final turn, the rewind is a single-step trim, not a general
edit.

`POST /turn/{n}/resolve-check` (IR-1) branches on the mode:

- `continuation`: today's behaviour (record a continuation turn).
- `single-turn`: generate the adjudication, then `ReplaceTurn` with one turn carrying the pending
  check (now resolved), the resolved checks, and the full segment set. The response stream is
  identical in shape.

### 4.4 The chronicle

A draft turn renders the pending card (IR-2) and nothing else. When it is completed, the same turn
now carries the adjudication, and the card renders resolved. No grouping is needed because there is
one turn.

An abandoned draft (the player never rolls) stays as a draft; it can be discarded with the existing
`/undo`, which trims it like any turn.

### 4.5 Export and consumers

The export pipeline and any turn-counting consumer see one turn per scene in `single-turn` mode.
Because the draft is a normal turn until completed, an export taken mid-scene includes the draft's
pending state, which the compiler renders as an unresolved check; a completed turn renders normally.

## 5. Behaviour

| Mode | Pending | Resolved |
| --- | --- | --- |
| continuation | turn N with a pending check | turn N+1 with the adjudication, `ContinuationOf = N` |
| single-turn | turn N (draft) with a pending check | turn N completed: pending check resolved, adjudication in the same turn |
| single-turn, reload before roll | turn N (draft) shows the waiting card | n/a |
| single-turn, undo | the draft is trimmed | n/a |

## 6. Testing

- `pkg/engine`: `ReplaceTurn` trims exactly the final turn and records the replacement; it refuses a
  non-final turn; entity history numbers are pruned as `RewindToTurn` does.
- `pkg/gui`: in `single-turn` mode, resolving a pending check yields one turn with both the resolved
  check and the adjudication; in `continuation` mode, behaviour is unchanged.
- `pkg/gui`: a draft turn round-trips through `history.jsonl` and reloads as a draft.
- `frontend`: a draft turn renders the pending card; a completed turn renders the resolved card.
- A regression guard: the default (`continuation`) mode is byte-for-byte the IR-1 behaviour.

## 7. Rollout

Opt-in via configuration; the default is unchanged. `Turn.Draft` is additive to the history record;
older records have no `draft` and are treated as complete. No migration.

## 8. Risks

- **A rewrite in an append-only log.** `ReplaceTurn` is a controlled rewind of the final turn,
  reusing tested machinery. The invariant is that only the final turn may be replaced; the guard is
  explicit and tested.
- **A draft that is never resolved.** It stays a draft and is trimmed by `/undo`. The chronicle
  should not treat a draft as an error, only as waiting.
- **Two modes to reason about.** The cost of the option. The default is the simpler mode, and the
  code paths differ only in persistence, sharing resolution and generation.
