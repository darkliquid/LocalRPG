# Manual Roll Entry Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#48 IR-4](https://github.com/darkliquid/Projects/LocalRPG/issues/48)
**Epic:** [#19 Interactive rolls](https://github.com/darkliquid/Projects/LocalRPG/issues/19)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §11 (IR-4)
**Depends on:** [#45 IR-1](https://github.com/darkliquid/Projects/LocalRPG/issues/45), [#46 IR-2](https://github.com/darkliquid/Projects/LocalRPG/issues/46)
**Scope:** `frontend`, `pkg/gui`, `pkg/engine`

---

## 1. Problem

The engine rolls the dice. A tabletop player often rolls **physical** dice and wants to enter the
result, either because they prefer the ritual or because the table has agreed to it. Nothing accepts a
player's number: the Roll card (IR-2) has a Roll button and an Argue affordance, but no way to say
"that was a 9".

IR-1 already carries a `manual_result` on the resolve-check request and the engine already has a forced
total (IR-1 Task 3). IR-4 is the surface and the honesty: entering a number, validating it is
plausible, and recording that it came from the player.

## 2. Goals

- An **Enter a roll** affordance on the Roll card: type a total (or the dice) and submit.
- The entered result resolves the check, exactly as a server roll would.
- The result records its **source** (`manual`), so the chronicle is honest.
- A wildly implausible entry is **warned**, not blocked: a local-first app trusts the player.
- The server records what it received, so a trace is faithful.

## 3. Non-goals

- Preventing cheating. A local-first app cannot and should not police a physical roll.
- Rolling dice on the player's behalf (the Roll button does that).
- The Argue flow (IR-5) and the resume protocol (IR-3).

## 4. Design

### 4.1 The affordance

On the Roll card (IR-2), **Enter a roll** expands a small input:

- a number field for the **total**, and
- optionally, a field for the **dice** (for example `4 3` for 2d6), which the client sums and shows as
  the total.

The client shows the expected range for the notation (`2d6` → 2-12) beside the field, so the player
sees what is plausible.

### 4.2 Plausibility

The client warns when the entry is outside the notation's possible range (a `2d6` total of 20) or is
not a number, but does not block it: a system may use a notation the client cannot range (a pool), and
a table may have a house rule. The warning is a hint, not a gate.

The **server** re-derives the range from the request's notation when it can and records a `manual`
result with the entered total. It never rejects a plausible-looking number.

### 4.3 The request

IR-1's `ResolveCheckRequestDTO` already has `ManualResult *int`. IR-4 adds the dice form:

```go
type ResolveCheckRequestDTO struct {
	PendingRef   string `json:"pending_check_ref"`
	ManualResult *int   `json:"manual_result,omitempty"` // the total
	ManualDice   []int  `json:"manual_dice,omitempty"`   // the individual dice, optional
	Note         string `json:"note,omitempty"`
}
```

When `ManualDice` is present, the server sums it and treats the sum as the total; the dice are recorded
on the result for the chronicle ("You rolled 4 3").

### 4.4 Recording

`CheckResult` already carries `Source` (IR-1) and `Roll`. A manual entry sets:

- `Source = "manual"`,
- `Total` = the entered total,
- `Roll.Dice` = the entered dice when provided, else a single face equal to the total (so the
  chronicle's dice display is coherent),
- and the modifiers (SYS-1) are added **after** the entered total, so a manual roll is the dice and the
  system's bonuses still apply. This is important: a player enters the dice, not the final number.

The chronicle shows "rolled 4 3 (manual) + Stealth 2 = 9".

### 4.5 The alternative reading

A user might expect to enter the **final total** (dice plus bonuses). The card makes the distinction
explicit: the input is labelled "your dice", the bonuses are shown beside it, and the computed total is
previewed. If a user enters a total that already includes bonuses, the preview shows the double-count,
so it is visible.

## 5. Behaviour

| Entry | Result |
| --- | --- |
| `4 3` for 2d6 | dice 4,3; total 7 + bonuses |
| `7` for 2d6 | a single die 7; total 7 + bonuses |
| `20` for 2d6 | a warning shown; still accepted |
| a non-number | blocked client-side with a message |
| no entry, Roll pressed | the server rolls (IR-1) |
| a manual result | recorded with source `manual` |

## 6. Testing

- `pkg/gui`: a manual total resolves the check to that total plus bonuses; manual dice are summed and
  recorded; the source is `manual`; an out-of-range entry is accepted and warned.
- `frontend`: the input shows the notation's range; a non-number is blocked; the preview shows the
  total; submitting sends the manual result.
- A regression guard: pressing Roll with no entry is unchanged.
- A property test: for any dice the player enters, the recorded total equals the dice sum plus the
  system's bonuses.

## 7. Rollout

Additive: an input and two optional request fields. A campaign that never enters a roll is unchanged.

## 8. Risks

- **Trust.** A player can enter anything. That is the point; the source is recorded, and the chronicle
  says `manual`. The app does not police it.
- **Dice-vs-total confusion.** The explicit labelling and the preview are the mitigation; the risk is a
  player double-counting bonuses, which the preview exposes.
- **Notation the client cannot range.** A pool or a custom notation has no client-side range; the input
  is free, and the server accepts it. The range hint is omitted when unknown.
