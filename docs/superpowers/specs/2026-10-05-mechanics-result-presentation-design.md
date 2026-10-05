# Mechanics Result Presentation Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#39 SYS-3](https://github.com/darkliquid/LocalRPG/issues/39)
**Epic:** [#18 Systems depth (mechanics and rolls)](https://github.com/darkliquid/LocalRPG/issues/18)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §3 (SYS-3)
**Depends on:** [#37 SYS-1](https://github.com/darkliquid/LocalRPG/issues/37), [#38 SYS-2](https://github.com/darkliquid/LocalRPG/issues/38)
**Scope:** `frontend`, `pkg/gui`

---

## 1. Problem

Checks render, but thinly. `DiceCheckCard` shows a pass/fail tone from `CheckResult.Outcome`, and
`TurnSegments` already places a check inline before the line it produced, with an unattached check
leading the prose (`frontend/src/components/TurnSegments.tsx:123-142`). What is missing is the
detail the richer resolution now produces:

- the SYS-1 **modifier breakdown** (why a 7 became a 9) is not shown;
- the SYS-2 **profile**, **success count**, **position**, and **effect** are not shown;
- the **stakes** the GM stated are not shown on the card;
- there is no per-turn summary of what mechanics ran, so a quiet turn looks like no mechanics exist;
- the outcome label is shown but not composed into a readable line ("weak hit — you succeed, at a
  cost").

A player who cannot see the arithmetic cannot trust it, and an author cannot tell whether a system
is engaging.

## 2. Goals

- The check card shows the full resolution: dice, every modifier, the outcome, the stakes, and,
  where present, the profile, success count, position, and effect.
- The outcome reads as fiction, not a bare label, using the system's own outcome vocabulary.
- A per-turn mechanics strip summarises the checks and the engagement level, including "none this
  turn".
- Tone follows the system's outcome vocabulary (a weak hit is neither green nor red).
- Nothing renders when a turn has no checks and mechanics are off.

## 3. Non-goals

- The pending-roll card and its Roll button. That is IR-2.
- Any change to how a check resolves.
- Editing the vocabulary; that is SYS-5.

## 4. Design

### 4.1 The check card

`DiceCheckCard` composes, top to bottom:

1. **Who and what** — the actor and the check kind, plus the stakes line when the request carried
   one.
2. **The dice** — the notation and the individual die faces (already available as `Roll.Dice`).
3. **The modifiers** — each `AppliedModifier` as `source +n`/`−n` (SYS-1), so the arithmetic is
   visible.
4. **The total and outcome** — the final total and the outcome label.
5. **Stakes of the outcome** — for a profile, the position and effect chips (SYS-2), and the success
   count for a pool.
6. **The outcome text** — the system's outcome-vocabulary description when the request carried one
   (`CheckRequest.Outcomes[outcome]`), so "weak hit" becomes the sentence the author wrote.

The card is compact when the result is simple (no modifiers, no profile) and expands only for the
detail that exists.

### 4.2 Tone by outcome vocabulary

The card's tone currently infers pass/fail. With a system vocabulary (strong/weak/miss,
success/fail, critical/…), tone is derived from the vocabulary position:

- the first declared outcome is the **best** tone;
- the last is the **worst**;
- anything between is **neutral** (the partial/weak case).

`outcomeTone(outcome, vocabulary)` returns one of three tones; an unknown outcome is neutral. This
replaces the ad-hoc pass/fail colouring and works for any vocabulary.

### 4.3 The per-turn mechanics strip

`MechanicsStrip` (already a component) becomes the turn-level summary:

- `Checks: N` with the outcomes as small chips (`2d6 → 8 weak`), or `Checks: none` when the turn
  resolved none;
- the engagement level (`mechanics: auto` / `ask` / `off`) so the player knows why;
- a pending indicator when the turn ended on a check (links to IR-2's card).

It renders only when the campaign has mechanics configured; with mechanics off it is absent.

### 4.4 Data

Everything needed already reaches the DTO after SYS-1 and SYS-2: `Checks[].Applied`,
`.Outcome`, `.Total`, `.Roll`, `.Stakes`, `.Profile`, `.Position`, `.Effect`, `.Successes`, and the
turn's engagement level (from settings). This spec adds no backend field except, if absent, the
per-check stakes already on `CheckResult` and the turn's engagement in the DTO; both are small
additive mappings.

## 5. Behaviour

| Turn | Card | Strip |
| --- | --- | --- |
| one simple check | dice + outcome | `Checks: 1 (2d6 → 9 strong)` |
| a check with modifiers | dice + modifiers + total | as above |
| a pool check | dice + success count + outcome | `Checks: 1 (5d10 → 3 successes strong)` |
| a blades check | dice + outcome + position/effect | `Checks: 1 (risky/limited weak)` |
| no checks, mechanics auto | nothing | `Checks: none · mechanics: auto` |
| mechanics off | nothing | nothing |

## 6. Testing

- `frontend`: the card renders dice, modifiers, total, outcome, stakes, and profile fields when
  present; `outcomeTone` maps best/neutral/worst from a vocabulary; the strip shows counts and the
  engagement level and hides when mechanics are off.
- `pkg/gui`: the check DTO carries stakes and the turn DTO carries the engagement level.
- A regression guard: a turn with no checks renders exactly as today.

## 7. Rollout

Frontend plus small additive DTO fields. A stored turn without the new fields renders as today.

## 8. Risks

- **Clutter.** A card with dice, modifiers, stakes, and chips can overwhelm a simple roll. Mitigation:
  progressive detail — only render the sections that have content.
- **Vocabulary assumptions.** A system with no vocabulary falls back to a two-tone pass/fail, which
  is today's behaviour.
- **Strip noise.** A strip on every turn could nag. It is quiet and one line; if it proves noisy it
  can be folded into the existing status affordance without a data change.
