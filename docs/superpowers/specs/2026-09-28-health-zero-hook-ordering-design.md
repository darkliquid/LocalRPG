# Health-Zero Hook Ordering Design

**Date:** 2026-09-28
**Status:** Implemented (2026-09-28); extended to per-NPC health the same day.

**Follow-up settled (2026-09-28): per-NPC health.** `healthOutcomes` now resolves
the effect for the player and for every character named in the turn, deduplicated
and player-first, and the two passes are unioned by entity
(`mergeHealthEffects`). An entity that has no numeric value for the declared stat
never fires, which also fixes a latent player bug: the old check treated a missing
stat as zero. A hook's context receives the pass-1 effects. Per-entity idempotence
matches the player's existing behaviour: an entity still at zero when a later turn
ends records the effect again.
**Scope:** See a health change made by an `onTurnEnd` hook in the same turn, by evaluating health-zero after the hook while keeping the recorded effect and the turn-end context consistent
**Related:** `pkg/engine/orchestrator.go`, `pkg/engine/mechanics_engagement.go`, `pkg/rules/js_engine.go`; implements the residual gap in `docs/superpowers/specs/2026-09-25-turn-memory-mechanics-followups-design.md` and follows `docs/superpowers/specs/2026-09-28-mechanics-engagement-depth-design.md`

## 1. Overview & Goals

Health-zero is resolved before the turn is recorded
(`pkg/engine/orchestrator.go:1103`) but `onTurnEnd` runs afterwards
(`orchestrator.go:1167`). A hook that drives the declared health stat to zero —
the natural way to express "the poison takes hold at the end of the round" — is
therefore not seen until the next turn, or never if the stat is restored first.

**Goals:**

- Evaluate health-zero after every writer for the turn, including `onTurnEnd`.
- Keep the recorded `Turn.HealthEffects` correct and fire the effect once.
- Keep `health_effects` available to the `onTurnEnd` hook context, as the
  followups spec requires.
- Not change when entity voicing, the working set, or the index write happen.

**Non-Goals:**

- Per-NPC health (still recorded as an adjacent follow-up).
- Changing what `zero_effect` means or how it is resolved.
- Reordering hooks relative to one another.

**Success Criteria:**

- A hook that subtracts the player's health stat to zero records the resolved
  `zero_effect` on the same turn.
- A hook that subtracts to zero and a state change that also reaches zero record
  exactly one effect.
- The `onTurnEnd` context still carries `health_effects`.
- The turn is still the single durable record for all of the above.

## 2. Investigation Findings

The current finalisation order in `ProcessActionStream` is:

| Step | Location | Effect |
| --- | --- | --- |
| Apply structured state changes | `orchestrator.go:1098` region | writes stats through the bridge |
| Earn advancement | `orchestrator.go:1098` | writes stats/memories |
| Resolve health-zero | `mechanics_engagement.go` (`healthOutcome`), called at `:1103` | reads the health stat once |
| Set `Turn.WorldTick` | `:1106` | records the tick directive |
| Record the turn | `:1108` (`RecordTurnContextStructured`) | writes entity notes, history, index |
| Voice new entities, apply working set | after `:1108` | |
| `onTurnEnd` hooks | `:1156-1167` | may write stats through the bridge |

`ExecuteTurnEnd` does not read the recorded turn; its context is built from
`turnNum`, `turn.Narration`, `turn.Entities`, `turn.Checks`, `turn.Verdict`, and
the newly added `turn.HealthEffects` / `turn.WorldTick`. Nothing between the
recording and the hook depends on the hook having already run.

`healthOutcome` (`mechanics_engagement.go`) reads
`rulesEngine.HostAPI().GetStat(playerID, health.Stat)` and resolves the effect
through `EvaluateHealthZero`. It is cheap and side-effect free, so it can be
called twice in a turn.

## 3. Design

### 3.1 Two-pass evaluation around the hook

Keep the hook where its other readers expect it, but evaluate health on both
sides of it and record the effect once.

1. **Pass 1** — after `ApplyStateChanges` and `applyAdvancement`
   (`orchestrator.go:1098`), call `healthOutcome()` and hold the result as
   `pendingEffect`.
2. **Run `onTurnEnd`** (`:1156-1167`) with the context unchanged, including
   `health_effects` seeded from `pendingEffect` (so a hook can react to the
   state it caused, and the documented field remains populated).
3. **Pass 2** — after the hook, call `healthOutcome()` again as `finalEffect`.
4. **Record** exactly one effect: `pendingEffect` when it is non-empty (the
   player was already at zero during the turn), otherwise `finalEffect` (the hook
   drove it to zero). Set `turn.HealthEffects` before
   `RecordTurnContextStructured`.
5. **Idempotence** — one entry per turn, for the player, matching today.

This keeps the followups spec's `onTurnEnd` context contract while making the
hook's own health writes visible in the same turn.

### 3.2 Ordering after the change

| Step | Before | After |
| --- | --- | --- |
| State changes / advancement | 1 | 1 |
| Health pass 1 | 2 | 2 |
| `Turn.WorldTick` | 3 | 3 |
| `onTurnEnd` | after record | 4 (before record) |
| Health pass 2 | — | 5 |
| Record turn | 4 | 6 |
| Voicing / working set | 5 | 7 |

Moving the hook before the record is a behaviour change: a subsequent turn's
`onTurnEnd`-written state is now part of the same record's index. That is the
point. Hooks already write through the bridge (entity notes) rather than to the
index directly, so nothing they do is lost by the reorder, and a hook that fails
is still logged and never fails the turn.

### 3.3 Why not re-record or patch

- Appending a second record for the hook's effect would break the
  one-turn-one-record invariant and `/undo`.
- Patching `history.jsonl` after the fact is fragile and would fight the
  append-only design.

Two passes keep the single record authoritative.

## 4. Interfaces

No signature changes. The internal shape becomes:

```go
// pkg/engine (internal)
pendingEffect := o.healthOutcome()   // pass 1
// ... build hookCtx including health_effects from pendingEffect, run ExecuteTurnEnd ...
finalEffect := o.healthOutcome()     // pass 2
if effect := firstNonEmpty(pendingEffect, finalEffect); effect != "" {
    turn.HealthEffects = append(turn.HealthEffects, HealthEffect{Entity: o.playerID, Effect: effect})
}
```

`hookCtx["health_effects"]` is populated from `pendingEffect`.

## 5. Error Handling

| Situation | Behaviour |
| --- | --- |
| No health schema | Both passes return ""; no effect, unchanged |
| Zero during state changes | Pass 1 fires; the hook sees it; recorded once |
| Driven to zero only by the hook | Pass 1 empty; pass 2 fires; recorded |
| Zero in pass 1, healed by the hook | Recorded (the effect happened); pass 2 does not add a second |
| Hook errors | Logged; pass 2 still runs; turn is not failed |
| Health stat unreadable | Both passes skip; logged once per turn |

## 6. Testing & Verification

Go (stdlib `testing`):

- `pkg/engine`: a state change to zero records the effect (existing test) and the
  `onTurnEnd` context contains it (existing test).
- `pkg/engine` (new): a mechanics script whose `onTurnEnd` subtracts health to
  zero produces a turn with the resolved `zero_effect`, with the hook running
  before the record.
- `pkg/engine` (new): a turn that is already at zero and is healed by the hook
  records exactly one effect.
- `pkg/engine`: a hook error leaves the turn recorded with the pass-1 result.
- `pkg/gui`: a full turn through `TurnSession` still maps `HealthEffects` to the
  DTO.

## 7. Compatibility & Rollout

- Behaviour change: an `onTurnEnd` health write now lands on the same turn. This
  is the intended fix; audited systems that relied on the one-turn delay are the
  only affected content.
- Hook ordering relative to recording changes; hooks that assume the turn is
  already indexed (none found) would be affected.
- `history.jsonl` format is unchanged.
- Revertible by restoring the single pass and the later hook call.

## 8. Open Questions

- Should `onTurnEnd` receive the pre-hook effect only, or both passes, so a hook
  can distinguish "already down" from "I just downed them"?
- Should the same two-pass pattern apply to any future post-hook derived state,
  and if so is there a general "post-hook resolution" step worth naming?
- Should per-NPC health ride this change, since the ordering is now correct for
  the player and would be for entities too?
- Does moving the hook earlier affect hook-driven `injectGMDirection`, which is
  drained at the *start* of a turn and so applies to the next turn either way?

## 9. References

- Code: `pkg/engine/orchestrator.go:1098-1167`,
  `pkg/engine/mechanics_engagement.go` (`healthOutcome`),
  `pkg/rules/js_engine.go` (`EvaluateTurnEnd`, `EvaluateHealthZero`),
  `pkg/engine/history.go` (`Turn.HealthEffects`).
- Specs: `2026-09-25-turn-memory-mechanics-followups-design.md` (§3.2),
  `2026-09-28-mechanics-engagement-depth-design.md`,
  `2026-09-26-mechanics-trigger-and-cadence-design.md`.
