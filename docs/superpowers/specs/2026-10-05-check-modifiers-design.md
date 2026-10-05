# Check Modifiers Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#37 SYS-1](https://github.com/darkliquid/LocalRPG/issues/37)
**Epic:** [#18 Systems depth (mechanics and rolls)](https://github.com/darkliquid/LocalRPG/issues/18)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §3 (SYS-1)
**Scope:** `pkg/harness`, `pkg/rules`, `pkg/engine`, `pkg/gui`, `frontend`

---

## 1. Problem

A check resolves to `total >= target`, where the total is the dice plus **one** optional stat value:

- `CheckRequest` carries `Actor, Target, CheckKind, Stat, Difficulty, Stakes, Outcomes, Notation`
  (`pkg/harness/turn.go:70-79`), but only `Stat` and `Notation` influence resolution.
- `SchemaResolver.Resolve` adds `statValue(bridge, actor, req.Stat)` when `Stat` is set
  (`pkg/rules/resolver.go:23-59`), and `statValue` reads exactly one path from actor state
  (`pkg/rules/resolver.go:62-78`).
- The default resolver (`pkg/engine/check_resolver.go`) uses a fixed pass at total >= 8 on `2d6`.

Systems can already **declare** skills (`core.SkillSpec`, `pkg/core/mechanics.go:86-90`), and the
host bridge exposes them (`ListSkills`, `pkg/rules/host_api.go:206-227`), but a declared skill
never changes a roll. A system that says "add your Stealth rating" cannot be expressed.

## 2. Goals

- A check can draw on more than one declared value: the governing stat **and** a skill rating.
- A check can carry explicit modifiers the GM names (situational bonuses and penalties).
- The breakdown is visible, so a player can see why a 7 became a 9.
- A check that names only a stat resolves exactly as it does today.

## 3. Non-goals

- Resolution profiles (PbtA ladders, dice pools, position/effect). That is SYS-2.
- Opposed rolls. That is SYS-4.
- A GUI for editing the values; that is SYS-5.

## 4. Design

### 4.1 Richer `CheckRequest`

`pkg/harness/turn.go`:

```go
// CheckModifier is one named adjustment to a check's total.
type CheckModifier struct {
	Source string `json:"source"`          // e.g. "high ground", "wounded"
	Value  int    `json:"value"`           // signed
	Reason string `json:"reason,omitempty"`
}

// CheckRequest gains:
	// Skill names a declared skill whose rating is added, alongside Stat.
	Skill string `json:"skill,omitempty"`
	// Modifiers are situational adjustments the GM names. Their sum is added.
	Modifiers []CheckModifier `json:"modifiers,omitempty"`
```

`Stat` and `Skill` may both be set; either may be empty. `Modifiers` is optional.

### 4.2 Resolution sums the parts

`SchemaResolver.Resolve` (`pkg/rules/resolver.go:23-59`) computes:

```
total = dice + statBonus + skillBonus + sum(modifiers)
```

- `statBonus` is the existing `statValue(actor, req.Stat)` (0 when `Stat` is empty).
- `skillBonus` is `statValue(actor, req.Skill)` — skills are stored on entity state like stats, so
  the same reader works. Rename the helper to `stateValue` for honesty, keeping behaviour.
- `modifiers` are summed as given.

The result records the breakdown:

```go
// CheckResult gains:
	// Applied lists every bonus that contributed, for display.
	Applied []AppliedModifier `json:"applied,omitempty"`

// AppliedModifier is one contribution to a check total.
type AppliedModifier struct {
	Source string `json:"source"` // "Stealth", "high ground"
	Value  int    `json:"value"`
}
```

`Applied` includes the dice separately (the existing `Roll` summary already carries the dice), so a
renderer can show "2d6 (7) + Stealth (2) + high ground (1) = 10".

The default resolver (`pkg/engine/check_resolver.go`) gains the same treatment so a system with no
`onCheck` still honours a skill and modifiers.

### 4.3 The GM can name a skill and modifiers

`request_check` (`pkg/harness/turn_tools.go:34-49`) gains two optional parameters:

- `skill`: the skill id.
- `modifiers`: an array of `{source, value, reason}`.

`ParseCheckRequest` (`pkg/harness/turn_tools.go:85-91`) already unmarshals into `CheckRequest`, so
the new fields decode for free. The tool description is extended to say that `stat` and `skill` may
both apply and that `modifiers` are for situational factors.

The mechanics instruction (`FormatMechanicsInstructions`, `pkg/harness/mechanics_instructions.go`)
gains one line: "Name the skill as well as the stat when a check tests a trained ability, and list
situational modifiers in `modifiers`." The system's declared skills are appended the way stats
already are (`mechanics_instructions.go:45-55`).

### 4.4 Validation

A skill or stat the system did not declare resolves to a 0 bonus (it is a name lookup that misses),
not an error: a check must never fail because the model named a value that does not exist. If the
system declares skills and the model names one, it applies; otherwise it is silently 0 and the
`Applied` list simply omits it. This mirrors how `stateValue` already returns `(0, false)` on a
miss (`pkg/rules/resolver.go:62-78`).

## 5. Behaviour

| Request | Total |
| --- | --- |
| `{notation:"2d6", stat:"might"}` (Might 2) | dice + 2 (unchanged) |
| `{notation:"2d6", stat:"might", skill:"stealth"}` (Might 2, Stealth 3) | dice + 5 |
| `{notation:"2d6", modifiers:[{source:"wounded", value:-2}]}` | dice − 2 |
| `{notation:"2d6", stat:"luck"}` (no such stat) | dice + 0 |
| `{}` (no stat, no skill, no modifiers) | dice (unchanged) |

## 6. Testing

- `pkg/rules`: `SchemaResolver` adds a stat, a skill, both, and modifiers; a missing value adds 0;
  `Applied` names every contribution; a request with only a stat matches the pre-change total
  exactly (a regression guard).
- `pkg/engine`: the default resolver honours a skill and modifiers the same way.
- `pkg/harness`: `ParseCheckRequest` decodes `skill` and `modifiers`; `TurnToolSpecsFor` advertises
  the new parameters.
- `pkg/harness`: `FormatMechanicsInstructions` lists declared skills.
- `pkg/gui`: the check DTO carries `Applied`; the frontend `DiceCheckCard` renders the breakdown.
- A property test: for any combination of stat/skill/modifiers, the total equals
  `dice + stat + skill + sum(modifiers)`.

## 7. Rollout

Additive. `CheckRequest`'s new fields are optional and absent from older records; `CheckResult`'s
`Applied` is optional. A stored turn without `Applied` renders as it does today.

## 8. Risks

- **Silent typos.** A model that names a misspelled skill gets a 0 and never knows. Mitigation: the
  mechanics instruction lists the exact declared ids, and the `Applied` breakdown shows what was
  counted, so a wrong name is visible.
- **Modifier inflation.** A model could stack bonuses. Mitigation: the instruction asks for named
  situational factors; the breakdown makes stacking visible to the player. A cap can be added later
  without a schema change.
- **Two resolvers drift.** The engine default and `SchemaResolver` must sum identically. Mitigation:
  share the summation in one helper in `pkg/rules` that both call.
