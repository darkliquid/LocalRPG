# Mechanics Engagement & Declarative Schema Design

**Date:** 2026-09-25
**Status:** Proposed
**Scope:** Declarative `mechanics` block in `system.yaml`, check resolution and `mechanics.js` override, host-API exposure, `state_changes` application, turn lifecycle hooks, GUI rules loading fix
**Related:** Structured Turn Protocol Design (2026-09-25), Entity Memories & Memory Tools Design (2026-09-25), `pkg/rules`, `pkg/core`, `pkg/gui/service.go`, `cmd/localrpg/play.go`

## 1. Overview & Goals

The engine is deliberately schema-agnostic: there are no declared stats, skills,
or health anywhere, `Roll` mode bypasses the system entirely and evaluates raw
dice notation, and `onAction` handlers run only when a non-Roll mode reaches the
rules engine. Worse, the GUI never calls `RuleLoader.LoadRules`, so web campaigns
run with an empty mechanics VM. There is no way for the engine to know which
mechanics a system wants, to validate a check, or to apply a state change
authoritatively.

This specification introduces an optional declarative `mechanics` block in
`system.yaml`, exposes it to `mechanics.js` for hooking, resolves checks through
schema convention or a js override, applies the GM's `state_changes` through the
rules layer, adds the missing `onTurnBegin` hook, and fixes the GUI rules-loading
gap. It is what makes the structured turn's `request_check` and `state_changes`
(protocol spec) meaningful.

**Goals:**

- Optional `mechanics` schema: stats, skills, health, and check conventions
  (notation, outcome vocabulary, difficulty ladder).
- The schema is the shared source of truth, readable by Go for validation and UI,
  and exposed to `mechanics.js` through the host API.
- Resolution order for a check: declared convention by default; a js-registered
  resolver overrides it when present.
- `state_changes` from the GM are validated against declared stats (when a system
  declares any) and applied through the host layer, which persists Markdown
  frontmatter state.
- Add `onTurnBegin`; enrich `onTurnEnd` with the turn, verdict, checks, and
  entities instead of just the turn number.
- Fix the GUI so it loads `mechanics.js` and world overrides.
- Outcome vocabulary is system-declared when present, free-form otherwise.

**Non-Goals:**

- Hardcoding any rule system; the engine still knows nothing about HP, classes, or
  skills beyond what a system declares.
- A GUI rule editor for `mechanics` (YAML is authored by hand for now).
- Changing `EvaluateRoll` or the dice library.
- Enforcing mechanics on providers that cannot call tools (protocol fallback).

**Success Criteria:**

- A system with a `mechanics` block produces valid check suggestions, validates
  `state_changes` against declared stats, and resolves a check to a declared
  outcome.
- A `mechanics.js` resolver registered for a check kind overrides the schema
  convention.
- Editing a stat through `setStat` in js and through a GM `state_change` both
  persist to the entity's Markdown frontmatter (via the `EntityWriter`).
- `onTurnBegin` fires once per turn with the turn number and location; `onTurnEnd`
  receives the verdict, checks, and entities.
- A web (GUI) campaign loads `mechanics.js`; a scripted `onAction` hook runs on a
  `Do` turn.
- A system with no `mechanics` block behaves exactly as today.

## 2. Investigation Findings

- `SystemManifest` (`pkg/core/types.go:11`) has only `id/name/version/description`
  and `character_creation`; no stats/skills/health.
- `pkg/rules` JSEngine binds `roll`, `getStat`, `setStat`, `getLocation`,
  `setLocation`, `injectGMDirection`, `log`, `onAction`, `onTurnEnd`,
  `onWorldTick` (`js_engine.go:34-119`). No `onTurnBegin`.
- `ExecuteAction` returns `ActionResult{Success, Outcome, Message, Roll, Data}`
  (`host_api.go:10-16`); no `RollRequest` type exists.
- `Turn` mode logic: `Roll` calls `rules.EvaluateRoll(actionInput)` directly and
  bypasses the system (`orchestrator.go:487`); other modes call `ExecuteAction`.
- `ExecuteTurnEnd` is passed only `{"turn": n}` and its error is discarded
  (`orchestrator.go:800`).
- `RuleLoader.LoadRules` reads `systems/<id>/mechanics.js` and
  `worlds/<id>/system_overrides/<system>/hooks.js` (`pkg/rules/loader.go:23`) but
  is called only from `cmd/localrpg/play.go:94`; `pkg/gui/service.go:1110` creates
  the engine without loading rules.
- `DefaultHostBridge` persists state through the `EntityWriter` (Timeline) when
  present (`host_api.go:130-138`), so Markdown stays canonical.

## 3. Declarative Schema

`system.yaml` gains an optional block, parsed into `core.SystemManifest`:

```yaml
mechanics:
  stats:
    - id: might
      label: Might
      type: number        # number | string | bool
      default: 0
      min: -3
      max: 5
    - id: hp_max
      label: Max HP
      type: number
      default: 10
    - id: hp
      label: Health
      type: number
      default: 10
  skills:
    - { id: athletics, label: Athletics, stat: might }
    - { id: stealth,   label: Stealth,   stat: agility }
  health:
    stat: hp
    max_stat: hp_max
    zero_effect: "incapacitated"   # free text or a js hook id
  checks:
    notation: "2d6"
    outcome: [critical, pass, partial, fail]
    difficulty:
      - { id: easy,        label: Easy,        target: 6 }
      - { id: standard,    label: Standard,    target: 8 }
      - { id: hard,        label: Hard,        target: 10 }
```

Go types mirror this in `pkg/core`:

```go
type SystemManifest struct {
    // ...existing...
    Mechanics *MechanicsSpec `yaml:"mechanics,omitempty"`
}

type MechanicsSpec struct {
    Stats      []StatSpec       `yaml:"stats,omitempty"`
    Skills     []SkillSpec      `yaml:"skills,omitempty"`
    Health     *HealthSpec      `yaml:"health,omitempty"`
    Checks     CheckConventions `yaml:"checks,omitempty"`
}
```

Everything is optional. A nil `Mechanics` means "schema-agnostic", the current
behaviour.

## 4. Check Resolution

### 4.1 Boundary

The structured-turn protocol calls `request_check`; the engine resolves it here.

```go
// pkg/rules
type CheckResolver interface {
    Resolve(req harness.CheckRequest, actor *entity.Entity) (*harness.CheckResult, error)
}
```

`RuleEngine` (the orchestrator's rules dependency) gains
`ResolveCheck(ctx, req) (*harness.CheckResult, error)`:

1. If `mechanics.js` registered a resolver for `req.CheckKind`, use it. A js
   resolver is registered through a new host binding
   `onCheck(kind, fn)`; the result is normalised into `CheckResult`.
2. Else, if the system declares `checks`, use the convention: roll `notation`,
   add the actor's `stat`/skill bonus, compare to the difficulty target, map to
   the declared `outcome` vocabulary.
3. Else, roll `req.Notation` (or a default `2d6`) and return `pass`/`fail` by
   total ≥ 8, with the raw `RollResult` for transparency.

Every branch returns one of the declared outcomes when a vocabulary exists, so
the narration can rely on the key.

### 4.2 Host API exposure

`GameHostAPI` gains read access to the loaded schema for `mechanics.js`:

```go
type GameHostAPI interface {
    // ...existing...
    ListStats() []StatSpec
    ListSkills() []SkillSpec
    CheckConventions() CheckConventions
}
```

`DefaultHostBridge` implements them from `core.SystemManifest`. `mechanics.js` can
therefore derive stats, build its own check tables, and register `onCheck`
resolvers and `onTurnBegin`/`onTurnEnd` hooks that read the schema.

## 5. State Changes

The GM's `state_changes` (protocol spec §3.2) are applied here:

- `op: set` writes `value`; `op: add`/`sub` mutate a numeric stat.
- Under a system with a `mechanics` block, the target path must be a declared stat
  (or a stat's nested field); an undeclared path is a validation error
  (`undeclared_stat`) unless the system sets `allow_freeform_state: true`.
- Without a `mechanics` block, any path is allowed (today's behaviour).
- Application goes through the host layer (`SetStat`), so the `EntityWriter`
  persists frontmatter and `state_change` trace events record before/after values.
- `health.zero_effect` is evaluated after application: when `health.stat` reaches
  0, the engine invokes the declared effect (a js hook or free-text outcome) and
  records it on the turn. The engine never hardcodes what "zero HP" means.

## 6. Turn Lifecycle Hooks

- **New `onTurnBegin`** (`onTurnBegin(fn)`): fires once per turn after context
  assembly and before generation, with `{turn, location}`. It may inject a
  directive (`injectGMDirection`) or mutate state.
- **Enriched `onTurnEnd`**: passed `{turn, verdict, checks, entities, narration}`
  instead of only `{turn}`; its error is no longer discarded but logged
  (`turn.end_hook_error`) without failing the turn.
- **`onAction`** unchanged, but the protocol's mode mapping means `Do`/`Say`/
  `Story` all reach it; a js handler may still short-circuit with an outcome.

## 7. GUI Rules Loading Fix

`pkg/gui/service.go` constructs the JSEngine but never loads rules. It must build
a `rules.RuleLoader` and call `LoadRules(systemID, worldID)` when a campaign is
prepared (`ensureIndexed`/first turn), mirroring `cmd/localrpg/play.go:94`. This
is a prerequisite for any mechanics behaviour in web campaigns and is included in
this spec.

## 8. Testing

- `LoadSystemManifest` parses a `mechanics` block; a missing block yields nil.
- Check resolution: schema convention maps a roll to a declared outcome; a js
  `onCheck` resolver overrides it; no schema falls back to `2d6`/pass-fail.
- `state_changes`: `set`/`add`/`sub` persist to Markdown via the `EntityWriter`;
  an undeclared stat is rejected under a schema system and accepted without one.
- `health.zero_effect` fires a registered hook at zero and is recorded.
- `onTurnBegin` fires once with `{turn, location}`; `onTurnEnd` receives verdict
  and checks; a hook error is logged, not fatal.
- GUI: a web campaign loads `mechanics.js`; a scripted `onAction` runs on `Do`.
- Regression: a system with no `mechanics` block behaves as before.

## 9. Compatibility & Migration

- `mechanics` is optional and additive; `SystemManifest` stays backward
  compatible.
- New host bindings are additive; existing `mechanics.js` scripts keep working.
- The GUI rules-loading fix changes behaviour only for web campaigns that have a
  `mechanics.js` (currently none ship), so it is safe.
- `Roll` mode is folded into the protocol (spec 1 §7): the raw-notation path
  remains available as `request_check` with an explicit `notation`, but a bare
  player roll no longer bypasses the system.

## 10. Open Questions

- Should `skills` be objects (with a governing stat) or a plain id list? (Current
  proposal: objects, because a convention needs the stat to add.)
- Should `checks.difficulty` support per-check-kind ladders, or one global ladder?
  (Current proposal: one global ladder; a js resolver handles per-kind cases.)
- Is `allow_freeform_state` per system or per world override? (Current proposal:
  system, overridable in `world.yaml` later.)
