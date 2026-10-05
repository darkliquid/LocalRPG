# Reference Systems Corpus Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#42 SYS-6](https://github.com/darkliquid/LocalRPG/issues/42)
**Epic:** [#18 Systems depth (mechanics and rolls)](https://github.com/darkliquid/LocalRPG/issues/18)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §3 (SYS-6)
**Depends on:** [#37 SYS-1](https://github.com/darkliquid/LocalRPG/issues/37), [#38 SYS-2](https://github.com/darkliquid/LocalRPG/issues/38)
**Scope:** new `pkg/refsystems`, `pkg/gui`, `frontend`

---

## 1. Problem

There is exactly one reference system, and it is a frontend constant
(`REFERENCE_SYSTEM_TEMPLATE`, `frontend/src/templates/referenceTemplates.ts:29-79`). It registers
`onAction` for `do`/`say`/`story` and a `2d6` ladder in prose, but it is a single example, it lives
only in the frontend, and nothing loads it in a Go test. The engine therefore has no corpus that
exercises complex state, skills, or the resolution profiles that SYS-1 and SYS-2 add.

Two consequences: a new user has one template to start from, and a change to the rules engine has no
system-level regression suite. The deep-dive's systems work cannot be trusted without one.

## 2. Goals

- Three complete, runnable systems covering the three resolution shapes: a PbtA `2d6` ladder, a
  `d20` + DC system, and a `d10` success pool.
- One source of truth: the systems are Go-embedded, served to the studio, and loadable in tests, so
  the frontend and the tests cannot drift.
- Each system exercises stats, skills, a resolution profile, and a `mechanics.js` hook.
- The Systems Studio offers them as starting points instead of a single hardcoded template.

## 3. Non-goals

- The test harness that runs scripted turns (SYS-7). This ships the corpus; SYS-7 runs it.
- Generating a system (SG-*).
- Shipped sample **worlds**; this is systems only.

## 4. Design

### 4.1 A new leaf, `pkg/refsystems`

A standard-library-only package that embeds the systems and exposes them:

```
pkg/refsystems/
  refsystems.go            // types, List, Get, embed
  systems/
    narrative_2d6/
      system.yaml
      mechanics.js
      prompts/rules.md
    d20_dc/
      system.yaml
      mechanics.js
      prompts/rules.md
    dice_pool/
      system.yaml
      mechanics.js
      prompts/rules.md
```

```go
// ReferenceSystem is a complete, runnable system shipped as a starting point.
type ReferenceSystem struct {
	ID          string
	Name        string
	Version     string
	Description string
	RulesPrompt string
	Script      string
	Mechanics   *core.MechanicsSpec
}

// List returns every reference system, ordered by ID.
func List() []ReferenceSystem

// Get returns one reference system by id.
func Get(id string) (ReferenceSystem, bool)
```

`List` reads the embedded files, parses `system.yaml` into `core.SystemManifest`, and fills the
struct. Because it imports `pkg/core`, it is not a leaf in the import graph, but it has no
dependencies beyond core and the standard library.

### 4.2 The three systems

**`narrative_2d6`** — the existing template, completed with real mechanics:

```yaml
id: narrative_2d6
name: Narrative 2d6
version: "1.0"
mechanics:
  stats:
    - { id: might,  label: Might,  type: number, default: 1 }
    - { id: edge,   label: Edge,   type: number, default: 1 }
    - { id: heart,  label: Heart,  type: number, default: 1 }
  skills:
    - { id: stealth, label: Stealth, stat: edge }
  checks:
    notation: 2d6
    outcome: [strong, weak, miss]
    profiles:
      pbta:
        ladder:
          - { min: 10, outcome: strong }
          - { min: 7,  outcome: weak }
          - { min: 0,  outcome: miss }
  engagement: auto
```

**`d20_dc`** — a class-and-level flavoured system: stats, a skill, and a `d20` DC profile.

**`dice_pool`** — a pool system: a `pool` profile with `success_on: ">=8"` and outcome ranges, plus
a `mechanics.js` that spends a resource on a strong outcome via `setStat`.

Each `mechanics.js` registers at least `onAction("do", …)` and `onTurnEnd(…)`, and uses `roll`,
`getStat`, and `setStat` so the engine's host API is exercised.

### 4.3 Serving them to the studio

`GET /api/reference-systems` returns the list (id, name, version, description, rules prompt, script,
mechanics). The Systems Studio replaces its hardcoded `REFERENCE_SYSTEM_TEMPLATE` with a fetch of
this list and offers all three as starting points. The existing "reset to reference" action loads
`narrative_2d6`, preserving today's behaviour.

The world template stays in the frontend, since it is not a system.

### 4.4 Tests

- `pkg/refsystems`: every embedded system parses, has a non-empty script and rules prompt, and its
  `Mechanics` validates (`CheckConventions.Validate`, SYS-2).
- `pkg/engine` (or a new `pkg/refsystems` integration test): each system loads into a `JSEngine` and
  resolves a check through its profile to a declared outcome, and its `onAction` hook returns a
  result for a `do` action. This is the corpus's purpose; SYS-7 generalises it into a harness.
- A parity test: the served list equals `List()`, so the endpoint cannot drift from the embed.

## 5. Behaviour

| Action | Result |
| --- | --- |
| Studio opens | three starting systems are offered |
| "Reset to reference" | loads `narrative_2d6` |
| A test loads `d20_dc` and rolls | a `d20` check resolves success/fail at the DC |
| A test loads `dice_pool` and rolls | a pool check resolves by success count |
| A test loads `narrative_2d6` and rolls 2d6 | strong/weak/miss by the ladder |

## 6. Testing

Covered in §4.4, plus:
- `frontend`: the studio lists the fetched systems and loads one into the draft.
- A regression guard: `narrative_2d6`'s script and rules prompt are unchanged from the current
  template except for the added mechanics block.

## 7. Rollout

Additive: a new package, a new endpoint, and a studio change. No migration. The frontend's hardcoded
system template is removed in favour of the fetch; the world template stays.

## 8. Risks

- **Two sources again.** The whole point is one source; if the studio keeps a fallback hardcoded
  copy, drift returns. The studio should fail gracefully (show no templates) rather than fall back to
  a stale constant.
- **Systems that do not reflect real play.** These are starting points, not refereed systems. Label
  them "reference" in the studio and keep them small and correct rather than complete.
- **Embed size.** Three small systems are trivial to embed; no concern.
