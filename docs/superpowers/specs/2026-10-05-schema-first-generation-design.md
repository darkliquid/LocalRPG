# Schema-First Generation Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#85 SG-2](https://github.com/darkliquid/LocalRPG/issues/85)
**Epic:** [#26 AI system generation](https://github.com/darkliquid/LocalRPG/issues/26)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §9 (SG-2)
**Depends on:** [#84 SG-1](https://github.com/darkliquid/LocalRPG/issues/84), [#42 SYS-6](https://github.com/darkliquid/LocalRPG/issues/42)
**Scope:** `pkg/sysgen`

---

## 1. Problem

SG-1 asks the model to emit a `mechanics` block as free-form JSON. That is flexible but fragile: the
model can produce a profile with an empty ladder, a skill pointing at a stat it forgot to declare, or
a notation that does not parse. SG-1 catches these at verification, but the failure costs a
regeneration and the model may repeat it.

At the same time, the systems people actually want cluster around a few shapes. SYS-6 ships three of
them (a PbtA ladder, a d20 + DC, a dice pool). SG-1 ignores them.

## 2. Goals

- Generate the schema by **selecting a known-good template and filling parameters**, not by inventing
  structure.
- Guarantee the assembled schema is valid by construction: a template's structure is fixed, only its
  parameters vary.
- Generate JavaScript **only** for a behaviour with no template, from a closed list of escape hatches.
- Make generated systems more consistent with each other and with the reference corpus.

## 3. Non-goals

- Replacing SG-1; this refines its schema step.
- A template for every conceivable mechanic; templates cover the common resolution, health, and
  advancement shapes, and the escape hatch covers the rest.
- The reference corpus itself (SYS-6); templates are derived from it.

## 4. Design

### 4.1 Templates

`pkg/sysgen` gains a template library, derived from SYS-6's corpus and the SYS-2 profiles:

```go
// Template is a known-good schema shape with parameters to fill.
type Template struct {
	ID         string
	Resolution string // "ladder" | "dc" | "pool"
	Health     string // "single" | "wounds" | "none"
	Advancement string // "spend" | "track" | "threshold" | "none"
}

// Build assembles a MechanicsSpec from a template and its parameters.
func (t Template) Build(params Params) (*core.MechanicsSpec, error)
```

`Params` carries the values the model chooses within the template's fixed structure:

```go
type Params struct {
	Stats     []core.StatSpec
	Skills    []core.SkillSpec
	Notation  string
	Ladder    []core.LadderStep   // for a ladder template
	DC        int                 // for a dc template
	SuccessOn string              // for a pool template
	HealthStat string
	Unlocks   []core.UnlockSpec
}
```

`Build` validates the parameters against the template (a ladder needs at least one step, a dc needs a
non-zero DC, and so on) and returns an error rather than an invalid spec.

### 4.2 The generation step

SG-1's schema step changes from "emit a `MechanicsSpec`" to two smaller calls:

1. **Choose** — from the description, choose a template (resolution, health, advancement) and give a
   one-line reason. The set of templates is small, so this is a classification, which models do well.
2. **Fill** — given the chosen template and its parameter schema, produce the parameters.

The code then calls `Template.Build(params)`, so the resulting `MechanicsSpec` is valid by
construction. This is the schema-first guarantee: the model fills values, it does not choose
structure.

### 4.3 Escape hatches

Some requested behaviour has no template: an unusual resource economy, an opposed roll (SYS-4, if it
lands), a bespoke consequence. The pipeline recognises these from a **closed list** of escape-hatch
kinds and, only then, asks for JavaScript:

```go
var escapeHatches = []string{"resource_spend", "custom_on_check", "custom_on_health_zero", "custom_on_turn_end"}
```

If the description asks for something outside the list, the pipeline says so (a note in the draft)
rather than generating unbounded JavaScript. This keeps the executable surface small.

### 4.4 Consistency with the corpus

Templates are derived from SYS-6's systems, so a generated system and a reference system share shapes.
A test asserts every template builds a spec that SYS-7's harness can run.

### 4.5 Relationship to SG-1 and SG-3

- SG-1 owns the pipeline; SG-2 replaces its schema step.
- SG-3 (the smoke-test gate) still runs; because the schema is valid by construction, verification
  mostly checks the JavaScript and the rules prose.

## 5. Behaviour

| Description | Template | JS |
| --- | --- | --- |
| "PbtA with three stats" | ladder + single health | none |
| "d20 with a DC" | dc + single health | none |
| "dice pool with a spend" | pool + resource_spend hatch | a small script |
| "something no template fits" | the closest template | a note, no unbounded JS |
| a parameter out of range | `Build` errors; the step is retried | n/a |

## 6. Testing

- `pkg/sysgen`: every template builds a valid spec for valid parameters; `Build` errors on an empty
  ladder, a zero DC, and a pool without outcomes.
- `pkg/sysgen`: the choose step maps descriptions to the expected template; a fill step's parameters
  produce a spec SYS-7 runs.
- `pkg/sysgen`: an escape-hatch request yields a small script; a non-hatch request yields none.
- A regression guard: a generated PbtA system's profile resolves like the corpus's.

## 7. Rollout

Refines SG-1's schema step. Additive templates; no existing behaviour changes beyond more consistent
output.

## 8. Risks

- **Template rigidity.** A system that does not fit is constrained to the closest template plus a
  note. That is deliberate: a valid, close system beats an invalid, exact one.
- **Classification errors.** The choose step can pick the wrong template. The reason line and the
  review make it visible and easy to change.
- **Template drift.** Templates must track `core.MechanicsSpec`; a test that builds every template
  catches a drift.
