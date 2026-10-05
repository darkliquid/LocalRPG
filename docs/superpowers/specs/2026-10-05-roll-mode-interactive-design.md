# Roll Mode Interactive Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#50 IR-6](https://github.com/darkliquid/LocalRPG/issues/50)
**Epic:** [#19 Interactive rolls](https://github.com/darkliquid/LocalRPG/issues/19)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §11 (IR-6)
**Depends on:** [#45 IR-1](https://github.com/darkliquid/LocalRPG/issues/45), [#46 IR-2](https://github.com/darkliquid/LocalRPG/issues/46)
**Scope:** `pkg/engine`, `pkg/gui`, `frontend`

---

## 1. Problem

The player's explicit **Roll** action is dead. When a client sends mode `roll`, the orchestrator
builds a `ProposedCheck` and sets an advisory directive:

```go
proposedCheck = &harness.ProposedCheck{Ref: "player-roll", Actor: o.playerID, Description: proposal}
gmDirective = "[PROPOSED CHECK: %s by %s (ref: %s)]"
```

(`pkg/engine/orchestrator.go:708-717`). Nothing resolves it and nothing enforces it; the model may
narrate a result the engine never rolled, or ignore the proposal entirely. Meanwhile the `ask`
policy has a working pending-check flow (`propose_check`, `pkg/engine/orchestrator.go:2035-2048`)
and IR-1/IR-2 give it an endpoint and a card.

So the one action whose whole meaning is "I want to roll" is the one that does not roll.

## 2. Goals

- A player Roll action produces a **pending check**, exactly as the GM's `propose_check` does.
- The Roll card (IR-2) appears and the player resolves it through IR-1.
- It works regardless of the engagement policy, because the player asked for it explicitly.
- The dead `ProposedCheck`/directive path is removed.

## 3. Non-goals

- The endpoint, the card, and the renegotiation flow (IR-1, IR-2, IR-5).
- Automatic checks; those remain the GM's or the cadence floor's job.

## 4. Design

### 4.1 Roll mode ends on a pending check

In the mode dispatch (`pkg/engine/orchestrator.go:708-717`), replace the directive with a pending
check:

```go
	case "roll":
		ref := rollRef(turnNumber, 0)
		result.PendingCheck = &harness.PendingCheck{
			Ref:        ref,
			ProposedBy: "player",
			Request:    o.playerRollRequest(actionInput),
		}
		return // the turn ends; the player resolves it via IR-1
```

`playerRollRequest` builds a `CheckRequest` from the system's conventions and the player's input:

- `Actor` = the player,
- `CheckKind` = the system's default or `"do"`,
- `Notation` = the profile/conventions notation,
- `Stat`/`Skill` = empty unless the player's input names one (the GM can refine the check when it
  adjudicates),
- `Stakes` = empty (the GM sets stakes when it proposed the check; a player-initiated roll has none
  yet, so the card shows the request as-is).

This reuses the same `PendingCheck` shape the `ask` policy produces, so IR-1, IR-2, and IR-3 all
work unchanged.

### 4.2 Why not honour the engagement policy

The engagement policy governs whether the **GM** rolls or asks. A player pressing Roll is an explicit
request that overrides the policy: under `auto` the GM usually resolves checks itself, but a player
who says "I roll" should get to roll. So Roll mode always produces a pending check.

Under `off`, mechanics are disabled; a Roll action in that mode should say so rather than produce a
check. The card is not shown; the engine returns a short narration explaining mechanics are off (or
the client disables the Roll action when `off`).

### 4.3 Removing the dead path

`harness.ProposedCheck` becomes unused once Roll mode stops building it. Remove it, its test
(`pkg/engine/proposed_check_test.go`), and the `[PROPOSED CHECK]` directive. `pendingRoll` (the
`@roll` record path) is unchanged: a model-requested roll still resolves under `auto` and pends under
`ask`.

### 4.4 The client

The Roll action in the action console already exists. With this change it produces a turn whose
`PendingCheck` is set, so the Roll card (IR-2) appears and the player resolves it. When engagement is
`off`, the console disables the Roll action and shows why.

## 5. Behaviour

| Engagement | Player Roll action |
| --- | --- |
| `auto` | a pending check appears; the player rolls via IR-1 |
| `ask` | a pending check appears; the player rolls via IR-1 |
| `off` | no check; the console disables Roll and says mechanics are off |
| any, with a profile | the pending check carries the profile's notation |

## 6. Testing

- `pkg/engine`: a `roll` action under `auto` produces a `PendingCheck` with `ProposedBy: "player"`
  and the system's notation; under `off` it produces none and does not error; the turn ends without
  generation when a check is pending (matching `propose_check`).
- `pkg/engine`: `ProposedCheck` and the directive are gone; no test references them.
- `pkg/gui`: a Roll turn reaches the client with a pending check; IR-1 resolves it.
- `frontend`: the Roll action is disabled under `off`.
- A regression guard: an `@roll` record path under `auto` and `ask` is unchanged.

## 7. Rollout

Behavioural change: a Roll action now pends instead of narrating. This is the intended fix; a client
that relied on the old advisory directive did not get a roll anyway. No migration.

## 8. Risks

- **Player-initiated checks with no stakes.** The GM usually sets stakes; a player roll has none
  until adjudication. The card shows the request without stakes, and the GM states the consequence
  when it adjudicates. Acceptable; a future flow could let the GM pre-state stakes.
- **Engagement `off`.** Disabling the action is the honest behaviour; producing a check under `off`
  would contradict the policy.
- **Removing `ProposedCheck`.** It is referenced only by Roll mode and one test; the compiler and a
  grep confirm it is safe.
