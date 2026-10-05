# Opposed Rolls Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#40 SYS-4](https://github.com/darkliquid/LocalRPG/issues/40)
**Epic:** [#18 Systems depth (mechanics and rolls)](https://github.com/darkliquid/LocalRPG/issues/18)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §3 (SYS-4)
**Depends on:** [#37 SYS-1](https://github.com/darkliquid/LocalRPG/issues/37), [#38 SYS-2](https://github.com/darkliquid/LocalRPG/issues/38)
**Scope:** `pkg/core`, `pkg/rules`, `pkg/engine`, `pkg/harness`, `pkg/gui`, `frontend`

---

## 1. Problem

Every check is against a **fixed** target: a difficulty id's number (SYS-2's conventions), a profile's
DC, or a ladder threshold. `CheckRequest.Target` exists (`pkg/harness/turn.go`) but is a string that
resolution ignores.

Many systems resolve **opposed** checks: the attacker's roll against the defender's roll, an
intimidate against a willpower, a contest of strength. The engine cannot express "roll against the
opponent" at all, so a system that needs it must fake it with a fixed difficulty or an `onCheck`
script.

## 2. Goals

- A check can name an **opponent**, and resolution rolls for both sides and compares.
- The opponent's roll uses the same notation, bonuses, and profile, so the two are symmetric.
- Both rolls and both totals are recorded and shown, so the player sees the contest.
- A check with no opponent resolves as today.

## 3. Non-goals

- Position/effect (SYS-2) and modifiers (SYS-1), which this builds on.
- NPC initiative or turn order; this is one contest.
- Automatic opponent selection; the GM names it.

## 4. Design

### 4.1 The request

`CheckRequest` already has `Target` (a string). SYS-4 gives it meaning: when `Target` names an entity,
the check is opposed.

```go
// CheckRequest gains:
	// Opposed, when set, names the stat the opponent rolls with. The check rolls
	// for the actor and the target and compares the totals.
	Opposed string `json:"opposed,omitempty"`
```

`Actor` is the rolling character; `Target` is the opponent's entity id; `Opposed` is the opponent's
governing stat (empty means the opponent rolls with no bonus).

### 4.2 Resolution

`SchemaResolver.Resolve` (and the engine default) gains an opposed path:

1. roll for the actor, summing SYS-1's bonuses (stat, skill, modifiers);
2. roll for the opponent with the same notation, summing the opponent's `Opposed` stat;
3. the higher total wins; a tie is the system's choice (a profile may declare `ties: actor|opponent`,
   defaulting to the actor, which favours the player in an ambiguous case);
4. the outcome maps through the profile or conventions as usual (a win is the better outcome, a loss
   the worse).

`CheckResult` gains the opponent's side:

```go
	// OpposedRoll is the opponent's roll and total, when the check was opposed.
	OpposedRoll  *harness.RollSummary `json:"opposed_roll,omitempty"`
	OpposedTotal int                  `json:"opposed_total,omitempty"`
	OpposedActor string               `json:"opposed_actor,omitempty"`
```

### 4.3 The opponent's entity

The resolver reads the opponent's state through the bridge (`GetStat(target, stat)`), exactly as it
reads the actor's. An unknown opponent or stat contributes 0, so a mistyped target degrades to "the
opponent rolled flat" rather than failing.

### 4.4 The GM names it

`request_check` and `propose_check` (`pkg/harness/turn_tools.go`) gain an `opposed` parameter (the
opponent's stat), and `target` (already a parameter) names the opponent. The mechanics instruction
mentions opposed checks when the system's profile declares them.

A profile may declare a default opposed stat:

```yaml
profiles:
  grapple:
    notation: 2d6
    opposed: might   # the opponent rolls Might
    ties: opponent
    ladder: [...]
```

so the GM can name only the profile and the target.

### 4.5 Presentation

The check card (SYS-3) shows both rolls: "You 9 vs Them 7 → strong". The chronicle's inline check
shows the contest, so an opposed roll reads as one.

## 5. Behaviour

| Request | Result |
| --- | --- |
| `{stat: "might", target: "ogre", opposed: "might"}` | both roll; the higher wins |
| a tie | the profile's `ties` choice, else the actor |
| `target` names no entity | the opponent rolls flat (0 bonus) |
| `opposed` empty, `target` set | not opposed; the fixed path |
| no `target` | not opposed, as today |

## 6. Testing

- `pkg/rules`: an opposed check rolls both sides with the same notation and sums each side's stat; the
  higher total wins; a tie follows the profile; an unknown opponent contributes 0; a non-opposed check
  is unchanged.
- `pkg/harness`: the tool advertises `opposed`; the instruction mentions opposed profiles.
- `pkg/gui`: the check DTO carries the opponent's roll and total; the card renders the contest.
- A property test: swapping the actor and target swaps the outcome (symmetry).

## 7. Rollout

Additive: `Opposed` is optional, and `Target`'s new meaning only applies when `Opposed` is set, so an
existing check that set `Target` as a label is unaffected.

## 8. Risks

- **`Target`'s old meaning.** It was a string that resolution ignored; a system that used it as a
  label (for example a target name) is unaffected because `Opposed` gates the new behaviour. Document
  the change.
- **Tie ambiguity.** The default favours the actor; a profile can override. The trace records the
  choice.
- **Two rolls per check.** A metered dice service would be two calls; the dice are local, so it is
  free. No provider involvement.
