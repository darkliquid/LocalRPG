# Roll Card Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#46 IR-2](https://github.com/darkliquid/LocalRPG/issues/46)
**Epic:** [#19 Interactive rolls](https://github.com/darkliquid/LocalRPG/issues/19)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §11 (IR-2)
**Depends on:** [#45 IR-1](https://github.com/darkliquid/LocalRPG/issues/45), [#39 SYS-3](https://github.com/darkliquid/LocalRPG/issues/39)
**Scope:** `frontend`, `pkg/gui`

---

## 1. Problem

A pending check renders as a small `pending_check` object in the turn (`frontend/src/types.ts:136-141`)
with no affordance beyond composing a new turn. There is no Roll button, no visible stakes, no
difficulty, and no way to see what the roll will mean. The interactive roll the engine now supports
(IR-1) has no surface.

A player who cannot see the stakes cannot decide whether to roll, and a GM who states stakes and
then gets silence has done the design work for nothing.

## 2. Goals

- A prominent card for a pending check that shows: who rolls, what is at stake, the difficulty or
  target, the possible outcomes, and the actor's relevant values.
- A **Roll** button that calls IR-1 and streams the adjudication into the chronicle.
- An **Argue** affordance that opens renegotiation (IR-5).
- A **manual roll** affordance (IR-4) for a physical dice result.
- The card is unmissable but not modal: the player may ignore it and act another way.
- After resolution, the card becomes the resolved check (SYS-3's card), so the fiction reads as one
  scene.

## 3. Non-goals

- The endpoint (IR-1) and the renegotiation flow (IR-5) and manual entry (IR-4); this is the card.
- Changing the resolution.
- A sound or animation for the roll; a light touch is enough.

## 4. Design

### 4.1 The card

`PendingCheckCard.tsx`, mounted by `TurnSegments.tsx` where a `pending_check` is present, in place of
the current inline rendering:

```
┌──────────────────────────────────────────────────────────┐
│ ⚄  A CHECK IS WAITING                                     │
│                                                          │
│  Kaelen tries to slip past the guard.                    │
│  Stakes: the alarm sounds if you fail.                   │
│                                                          │
│  Roll: 2d6 + Edge 3        Target: 10+ strong / 7+ weak  │
│  Outcomes: strong · weak · miss                          │
│                                                          │
│  [ Roll ]   [ Enter a roll ]   [ Argue the stakes ]      │
└──────────────────────────────────────────────────────────┘
```

Contents, from the pending check and the system's conventions:

- **Actor and intent** — the actor and the description the GM gave.
- **Stakes** — `CheckRequest.Stakes`.
- **Roll** — the notation (profile or conventions) and the SYS-1 bonuses that will apply, so the
  player sees the arithmetic before rolling.
- **Target and outcomes** — from the profile ladder, the DC, or the difficulties, plus the outcome
  vocabulary.
- **Actions** — Roll, Enter a roll, Argue.

### 4.2 Rolling

The Roll button calls `POST /api/game/{id}/turn/{n}/resolve-check` with the pending ref and streams
the response exactly as a normal turn: chunks, segments, speech, and the final `turn` event. The
stream is fed through the same `TurnStreamProcessor` (`frontend/src/lib/turnStreamProcessor.ts`), so
nothing new is needed to render the adjudication.

While the stream runs, the card shows a rolling state and disables its buttons; the turn lock
(`409`) is surfaced as "the turn is busy" rather than an error.

### 4.3 Prominence

- The card uses a distinct accent (the sky/purple already used for player/NPC, or a dedicated
  amber) and a dice glyph.
- A dot appears on the Character and Chronicle affordances while a check is pending, matching the
  existing status-dot pattern.
- The card is **not** modal: the player can still type an action, which submits a normal turn
  carrying `PendingCheckRef` (the existing path). Both routes reach the same engine path.

### 4.4 After resolution

Once the continuation turn lands, the pending card is replaced by the resolved `DiceCheckCard` for
the same check, and the continuation's narration follows. Grouping is by `ContinuationOf`, which the
turn DTO already carries, so the chronicle can render the pair together.

On reload, the proposing turn still shows the pending check; if it was resolved, the continuation is
present and the card renders resolved. A resolved check's card offers no Roll button.

### 4.5 Data

The card needs the check request (notation, stat, skill, modifiers, stakes, outcomes, profile) and
the actor's values. The pending check DTO (`PendingCheck`) already carries the request
(`harness.PendingCheck.Request`, `pkg/harness/turn.go:122-126`); this spec adds the actor's relevant
values and the resolved notation/bonuses to the DTO so the card can show the arithmetic before the
roll. The system's conventions are already available to the client from the system detail (SYS-5).

## 5. Behaviour

| State | Card |
| --- | --- |
| a pending check | full card with Roll / Enter / Argue |
| rolling | buttons disabled, a rolling state |
| resolved | the resolved check card (SYS-3), no Roll |
| another turn in flight | buttons disabled, "the turn is busy" |
| no pending check | nothing |
| mechanics off | nothing |

## 6. Testing

- `frontend`: the card renders the stakes, target, outcomes, and bonuses; Roll calls the endpoint and
  streams; the rolling state disables the buttons; a resolved check shows no Roll; a `409` shows the
  busy state; a turn with no pending check renders nothing.
- `pkg/gui`: the pending-check DTO carries the actor values and the resolved notation/bonuses.
- A regression guard: a campaign on the `auto` policy renders no pending card, exactly as today.

## 7. Rollout

Frontend plus small additive DTO fields. Nothing changes for a campaign that never uses `ask`.

## 8. Risks

- **An unmissable card that nags.** A pending check is meant to be noticed; that is the point. It is
  dismissed only by resolving or ignoring it, and ignoring is always allowed.
- **Stream reuse.** The resolve-check stream is the same shape as a turn stream, so the existing
  processor handles it; a divergence would be caught by the endpoint's tests.
- **Double submission.** Roll disables while in flight and the server serialises per campaign, so a
  double click cannot roll twice.
