# Named Resolution Profiles Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#38 SYS-2](https://github.com/darkliquid/LocalRPG/issues/38)
**Epic:** [#18 Systems depth (mechanics and rolls)](https://github.com/darkliquid/LocalRPG/issues/18)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §3 (SYS-2)
**Depends on:** [#37 SYS-1](https://github.com/darkliquid/LocalRPG/issues/37)
**Scope:** `pkg/core`, `pkg/rules`, `pkg/engine`, `pkg/harness`, `pkg/gui`, `frontend`

---

## 1. Problem

Every check resolves the same way. `CheckConventions` declares one notation and a list of named
difficulties (`pkg/core/mechanics.go:100-111`), and `SchemaResolver` reduces any check to
`total >= target` with the difficulty chosen by id (`pkg/rules/resolver.go:23-59`). The engine
cannot express the ways real systems actually resolve:

- a **PbtA ladder** (10+ strong, 7-9 weak, 6- miss) where a "weak hit" is a distinct outcome;
- a **d20 + DC** where success is binary against a class-difficulty;
- a **dice pool** where each die over a threshold is a success and the *count* decides the outcome;
- **Blades** position and effect, chosen before the roll, that colour what a 4/5 means.

The dice library already computes the raw numbers (`pkg/rules/dice.go` over
`github.com/darkliquid/roll`), including success counts and keep/drop. What is missing is a way for
a system to declare how those numbers map to outcomes, and for the GM to pick the right mapping for
the action.

## 2. Goals

- A system declares **named resolution profiles** in its `mechanics` block.
- The GM names a profile per check; the resolver maps the roll through it.
- Four shapes cover the common systems: threshold ladder, DC, success-count pool, and
  position/effect.
- A check with no profile resolves exactly as today (conventions + difficulty).

## 3. Non-goals

- Opposed rolls (SYS-4) and skill/modifier arithmetic (SYS-1, which this builds on).
- Editing profiles in the GUI (SYS-5 adds the editor; this spec defines the schema).
- System generation (SG-*).

## 4. Design

### 4.1 The schema

`CheckConventions` gains a profile map:

```go
type CheckConventions struct {
	Notation   string                       `yaml:"notation,omitempty"`
	Outcome    []string                     `yaml:"outcome,omitempty"`
	Difficulty []DifficultySpec             `yaml:"difficulty,omitempty"`
	Profiles   map[string]ResolutionProfile `yaml:"profiles,omitempty"`
}

// ResolutionProfile maps a roll to an outcome for one style of check.
type ResolutionProfile struct {
	Label     string           `yaml:"label,omitempty"`
	Notation  string           `yaml:"notation,omitempty"`  // overrides the system default
	DC        int              `yaml:"dc,omitempty"`        // total >= DC is a success
	Ladder    []LadderStep     `yaml:"ladder,omitempty"`    // highest-first thresholds
	SuccessOn string           `yaml:"success_on,omitempty"` // pool: a die meeting this is a success
	Outcomes  []SuccessOutcome `yaml:"outcomes,omitempty"`  // pool: success count → outcome
	Position  []string         `yaml:"position,omitempty"`  // blades: controlled|risky|desperate
	Effect    []string         `yaml:"effect,omitempty"`    // blades: limited|standard|great
}

type LadderStep struct {
	Min     int    `yaml:"min"`
	Outcome string `yaml:"outcome"`
}

type SuccessOutcome struct {
	Min     int    `yaml:"min"`
	Max     int    `yaml:"max"` // -1 means unbounded
	Outcome string `yaml:"outcome"`
}
```

Example:

```yaml
mechanics:
  checks:
    notation: 2d6
    outcome: [strong, weak, miss]
    profiles:
      pbta:
        ladder:
          - { min: 10, outcome: strong }
          - { min: 7,  outcome: weak }
          - { min: 0,  outcome: miss }
      d20:
        notation: 1d20
        dc: 15
      pool:
        notation: 5d10
        success_on: ">=8"
        outcomes:
          - { min: 3, max: -1, outcome: strong }
          - { min: 1, max: 2,  outcome: weak }
          - { min: 0, max: 0,  outcome: miss }
      blades:
        position: [controlled, risky, desperate]
        effect: [limited, standard, great]
        ladder:
          - { min: 10, outcome: strong }
          - { min: 7,  outcome: weak }
          - { min: 0,  outcome: miss }
```

### 4.2 The request and the result

`CheckRequest` (`pkg/harness/turn.go`) gains:

```go
	// Profile names a resolution profile from the system's checks. Empty uses the
	// system's default conventions.
	Profile string `json:"profile,omitempty"`
	// Position and Effect are the Blades-style stakes a profile may define.
	Position string `json:"position,omitempty"`
	Effect   string `json:"effect,omitempty"`
```

`CheckResult` gains:

```go
	Profile   string `json:"profile,omitempty"`
	Position  string `json:"position,omitempty"`
	Effect    string `json:"effect,omitempty"`
	Successes int    `json:"successes,omitempty"`
```

### 4.3 Resolution

One function maps a roll to an outcome, shared by `SchemaResolver` and the engine default resolver
(so they cannot drift, mirroring SYS-1's `SumBonuses`):

```go
// ResolveProfile maps a roll total and success count through a profile to an
// outcome. It returns ("", false) when the profile is empty or cannot decide, so
// the caller falls back to the conventions path.
func ResolveProfile(p core.ResolutionProfile, total, successes int) (string, bool)
```

Order of decision:

1. **Ladder** (if non-empty): the first step whose `Min <= total` decides. Steps are author-sorted
   highest-first; the resolver also sorts defensively.
2. **DC** (if non-zero): `total >= DC` is the first declared outcome (or `"success"`), else the
   second (or `"fail"`).
3. **Pool** (if `SuccessOn` and `Outcomes` are set): the success count is matched against the
   `SuccessOutcome` ranges.
4. Otherwise: no decision, fall back to the conventions path.

`SchemaResolver.Resolve` selects the profile (`req.Profile`, else a profile named `default`, else
none), computes the notation from the profile or conventions, rolls, sums SYS-1 bonuses, then calls
`ResolveProfile`. On no decision it uses the existing `outcomeFor` path.

`Position` and `Effect` are validated against the profile's allowed lists and passed through onto
the result; they do not change the arithmetic, only the fiction the GM is told and the UI shows.

### 4.4 The GM names a profile

`request_check` and `propose_check` (`pkg/harness/turn_tools.go`) gain optional `profile`,
`position`, and `effect` parameters. `FormatMechanicsInstructions` lists the declared profiles and
their shapes, the way it lists difficulties today, so the model knows what it may name.

### 4.5 Validation

A system's `mechanics.checks.profiles` is validated when the manifest loads:

- every ladder is non-empty and every step's outcome is in the outcome vocabulary (or the vocabulary
  is empty, in which case any label is accepted);
- a `pool` profile has `success_on` and at least one outcome range;
- a `blades` profile's ladder is present;
- an unknown profile named by the GM at resolution time falls back to the conventions path and is
  logged, never fatal.

## 5. Behaviour

| Request | Result |
| --- | --- |
| `{profile: "pbta"}` on 2d6 total 9 | `weak` |
| `{profile: "d20", notation omitted}` | rolls 1d20, `success` iff ≥ 15 |
| `{profile: "pool"}` on 5d10 with three dice ≥ 8 | `strong` |
| `{profile: "blades", position: "risky", effect: "limited"}` | outcome by ladder, position/effect on the result |
| `{profile: "nope"}` | conventions path, logged |
| `{}` (no profile) | conventions path, exactly as today |

## 6. Testing

- `pkg/core`: manifest validation accepts the four example profiles and rejects an empty ladder, a
  pool without outcomes, and a blades profile without a ladder.
- `pkg/rules`: `ResolveProfile` for each shape, including boundary values (exactly 10, exactly DC,
  exactly the success threshold) and the empty-profile fallback; a regression guard that the
  conventions path is unchanged when no profile is named.
- `pkg/harness`: the tool specs advertise `profile`/`position`/`effect`; the instruction lists
  profiles.
- `pkg/gui`: the check DTO carries profile, position, effect, and successes; the frontend renders
  them.
- A property test: for a ladder covering all totals, every total maps to exactly one outcome.

## 7. Rollout

Additive. `profiles` is absent in existing systems, so every check uses the conventions path. A
stored turn without the new result fields renders as today.

## 8. Risks

- **Profile/prompt mismatch.** The model may name a profile the system does not define. The fallback
  makes it safe and the trace makes it visible.
- **Ladder ambiguity.** Overlapping thresholds must resolve deterministically; highest-first and a
  defensive sort handle it, and the property test pins it.
- **Scope creep.** Position/effect is Blades-specific; keep it as pass-through metadata, not a
  second resolution engine. Anything richer belongs in `onCheck`.
