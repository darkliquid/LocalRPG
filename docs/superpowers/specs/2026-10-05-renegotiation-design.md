# Renegotiation Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#49 IR-5](https://github.com/darkliquid/Projects/LocalRPG/issues/49)
**Epic:** [#19 Interactive rolls](https://github.com/darkliquid/Projects/LocalRPG/issues/19)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §11 (IR-5)
**Depends on:** [#45 IR-1](https://github.com/darkliquid/Projects/LocalRPG/issues/45), [#46 IR-2](https://github.com/darkliquid/Projects/LocalRPG/issues/46)
**Scope:** `pkg/engine`, `pkg/gui`, `frontend`

---

## 1. Problem

When the GM proposes a check, the stakes and difficulty are fixed. A tabletop player routinely
**argues** them: "I'm not picking the lock, I'm lifting the bar out of its brackets", "that should be
Risky, not Desperate", "I'm using my scholar background to know the sigil". The GM reconsiders.

The app has no way to do this. The Roll card (IR-2) has an Argue button in its design, but there is
nothing behind it: the pending check's stakes and difficulty are what the GM set, and the only paths
are Roll or write a normal turn (which starts over, losing the pending check).

This is the "argue to renegotiate the roll" the phase set out to support, and it is the feature that
makes the `ask` policy feel like a table rather than a prompt.

## 2. Goals

- The player can **counter-propose**: different stakes, a different approach, or a different
  difficulty.
- The GM **re-adjudicates**: accept the counter, adjust it, or hold firm, with a reason.
- The negotiation is **recorded**, so the chronicle shows the exchange and the final terms.
- The pending check is **updated in place** (or replaced) rather than abandoned, so the player then
  rolls the agreed check.
- A counter that the GM rejects leaves the original check intact, so nothing is lost.

## 3. Non-goals

- The Roll button (IR-2) and manual entry (IR-4).
- Negotiating a check the player initiated (IR-6's Roll mode); a player's own proposal can be edited
  before rolling, but there is no GM to argue with.
- A free-form chat; the counter is structured (stakes, approach, difficulty).

## 4. Design

### 4.1 The counter-proposal

On the Roll card, **Argue** opens a small form:

- **Approach** — how the player reframes the action (free text, for example "lift the bar instead of
  picking the lock").
- **Stakes** — a proposed restatement of the stakes (free text, optional).
- **Difficulty** — a proposed difficulty id from the system's list (optional).

```go
// CounterProposal is a player's argument about a pending check.
type CounterProposal struct {
	Approach   string `json:"approach,omitempty"`
	Stakes     string `json:"stakes,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
}
```

### 4.2 The adjudication

`POST /api/game/{id}/turn/{n}/renegotiate` takes the pending ref and the counter. The engine runs a
**short generation** (not a full turn) that asks the GM to adjudicate:

- the GM sees the pending check and the counter;
- it returns one of: **accept** (adopt the counter's stakes/difficulty), **adjust** (a modified
  stakes/difficulty with a reason), or **hold** (keep the original, with a reason).

The response is a small structured result, not a turn:

```go
// Adjudication is the GM's ruling on a counter-proposal.
type Adjudication struct {
	Ruling     string // "accept" | "adjust" | "hold"
	Stakes     string
	Difficulty string
	Notation   string
	Reason     string
	Profile    string
}
```

### 4.3 Updating the pending check

On accept or adjust, the turn's `PendingCheck.Request` is updated with the new stakes/difficulty (and
the profile/notation if the approach changed the check), and the turn is re-saved. The Roll card then
shows the **agreed** terms, and the player rolls those.

On hold, the pending check is unchanged, and the card shows the GM's reason so the player knows why.

This keeps one pending check, updated in place, rather than a negotiation that spawns turns.

### 4.4 Recording the negotiation

The turn gains a small **negotiation** list:

```go
// Negotiation is one counter-proposal and the GM's ruling on it.
type Negotiation struct {
	Counter CounterProposal
	Ruling  Adjudication
}
```

It is persisted on the turn, so the chronicle can render the exchange: "Player: lift the bar, not
pick the lock. GM: accept — that's a Might check, Risky." The final agreed terms are on the pending
check.

### 4.5 Bounds

- The counter is bounded in length.
- The adjudication is one short generation, so WG-6's estimate/cap apply if a cap is configured.
- A counter on a check the player already rolled is refused (the check is resolved).

### 4.6 What the GM is told

The adjudication prompt is explicit: adopt a counter that is reasonable and improves the fiction;
adjust one that is partly right; hold when the counter does not change the difficulty honestly. The
goal is a fair table, not a pushover.

## 5. Behaviour

| Counter | Result |
| --- | --- |
| "lift the bar, not pick it" | the GM accepts; the check becomes Might, the stakes updated |
| "that's Risky, not Desperate" | the GM accepts or adjusts the difficulty |
| an unreasonable counter | the GM holds, with a reason |
| a counter after rolling | refused (the check is resolved) |
| the player rolls after agreeing | the agreed check resolves |
| the chronicle | shows the counter and the ruling |

## 6. Testing

- `pkg/engine`: a counter updates the pending check on accept/adjust; a hold leaves it unchanged; the
  negotiation is recorded; a resolved check refuses a counter.
- `pkg/gui`: the endpoint returns the ruling; the pending DTO reflects the agreed terms.
- `frontend`: Argue opens the form; the ruling is shown; the card then shows the agreed terms; a hold
  shows the reason.
- A regression guard: a turn with no counter is unchanged.

## 7. Rollout

Additive: an endpoint, a form, and a recorded exchange. A campaign that never argues is unchanged.

## 8. Risks

- **A pushover GM.** The prompt biases toward reasonableness, but a model may accept everything. The
  recorded ruling makes it visible, and the prompt can be tuned. Accepting a good counter is correct;
  accepting a bad one is a model issue, not a design one.
- **Renegotiation loops.** A player could argue forever. The check is one pending check; each counter
  replaces the terms, and the player eventually rolls. A cap on counters per check (a small number)
  is a cheap guard.
- **Structured vs free-form.** A free-form approach field can be anything; the GM maps it to a check.
  The structured difficulty list keeps the target well-formed.
