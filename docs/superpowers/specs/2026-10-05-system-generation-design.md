# System Generation Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#84 SG-1](https://github.com/darkliquid/LocalRPG/issues/84)
**Epic:** [#26 AI system generation](https://github.com/darkliquid/LocalRPG/issues/26)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §9 (SG-1)
**Depends on:** [#41 SYS-5](https://github.com/darkliquid/LocalRPG/issues/41), [#43 SYS-7](https://github.com/darkliquid/LocalRPG/issues/43)
**Scope:** new `pkg/sysgen`, `pkg/gui`, `frontend`

---

## 1. Problem

System generation is field-scoped. `/api/generate-text` fills a name, a description, and a rules
prompt (`pkg/gui/text_generate.go:98`); `mechanics.js` is not generated at all, and the declarative
`mechanics` block is not even editable (SYS-5). A user who wants "a gritty d20 system with sanity and
wounds" gets three text fields and must write the mechanics by hand.

The engine can now express a great deal (SYS-1, SYS-2, SYS-5), and SYS-7 can verify a system without
playing it. Nothing uses either to author a system.

## 2. Goals

- Generate a **complete** system from a description: `system.yaml` (with the `mechanics` block),
  `mechanics.js`, and `prompts/rules.md`.
- Target the **declarative schema** first and generate JavaScript only for the escape hatches, so the
  output is inspectable and editable.
- **Verify before offering**: load the generated system and run SYS-7's smoke test; a system that
  cannot load is never offered.
- Produce a **draft** reviewed before it is saved.

## 3. Non-goals

- Enhancing an existing system (SG-4) and the base catalogue (SG-5).
- The mechanics editor (SYS-5) and the harness (SYS-7); this uses them.
- Runtime play behaviour.

## 4. Design

### 4.1 The pipeline

A new `pkg/sysgen` orchestrates structured calls:

1. **Shape** — from the description, decide the system's shape: resolution style (a SYS-2 profile),
   stats, skills, health, and whether advancement applies.
2. **Schema** — emit the `mechanics` block as structured data (the SYS-5 schema), including the
   resolution profiles and the outcome vocabulary.
3. **Hooks** — emit `mechanics.js` for the behaviours the schema cannot express (for example an
   `onHealthZero` effect, a resource spend on a strong outcome). This step is skipped when the schema
   covers everything.
4. **Rules** — emit `prompts/rules.md`: the prose a player reads, consistent with the schema.
5. **Verify** — load the result into a `JSEngine` and run a generated smoke scenario (SYS-7): load,
   roll a check through the declared profile, and apply a state change. A failure is reported and the
   system is not offered.

```go
// System is a generated system, before it is saved.
type System struct {
	ID          string
	Name        string
	Version     string
	Description string
	Mechanics   *core.MechanicsSpec
	Script      string
	RulesPrompt string
	Verify      VerifyResult // the smoke test's outcome
}

func Generate(ctx context.Context, gen Generator, brief Brief) (System, error)
```

`Generator` is WG-1's structured-output seam, reused.

### 4.2 Schema-first, JS-second

The prompt for the schema step targets `core.MechanicsSpec`, and the hooks step is told to emit
JavaScript **only** for behaviour the schema lacks. This keeps a generated system readable and
editable in SYS-5's editor, and it keeps the generated JavaScript small, which matters because it is
executed.

### 4.3 Verification is mandatory

The verify step is not optional. A generated `mechanics.js` can have a syntax error, reference an
undefined host call, or declare a profile that never resolves. Loading it in a `JSEngine` and running
the smoke scenario catches all three before the user sees it. `VerifyResult` records the outcome and
any failure detail; a failed system is shown with its failure and a "regenerate" action, never saved.

### 4.4 The draft

A generated system is a draft, reviewed like a world (WG-5's pattern): the user sees the mechanics
(in SYS-5's editor), the script, and the rules, and can edit before saving. Saving writes
`systems/<id>/` via the existing `SaveSystem`.

### 4.5 Bounds

The same cost controls as WG-6 apply: an estimate (four or five calls), a dry run, and the call cap.

### 4.6 Surfaces

- **API**: `POST /api/system/generate` (NDJSON steps and the draft, or a dry-run estimate).
- **Studio**: a "Generate a system" flow in `SystemsStudio.tsx` that collects the description and
  hands the draft to review.

## 5. Behaviour

| Input | Result |
| --- | --- |
| "a gritty d20 system with sanity" | a system with a d20 profile, sanity as a stat, and rules |
| a description the schema cannot express | the schema plus a small `mechanics.js` |
| a generated script with a syntax error | verification fails; the system is shown with the error |
| accept | `systems/<id>/` is written |
| reject | nothing is written |
| no provider | the estimate is zero and the oracle produces a minimal template system |

## 6. Testing

- `pkg/sysgen`: a stub generator produces a system; the schema step's output parses into
  `core.MechanicsSpec`; a script with a syntax error fails verification; a good system passes.
- `pkg/sysgen`: the smoke scenario resolves a check through the generated profile.
- `pkg/gui`: the endpoint streams and writes nothing until accept; accept writes `systems/<id>/`.
- A regression guard: a generated system with no hooks has an empty script, not a stub.

## 7. Rollout

Additive: a new package, an endpoint, and a studio flow. No existing system changes.

## 8. Risks

- **Executable output.** A generated `mechanics.js` runs. The mandatory verification is the control;
  a system that fails it is never offered. The user still reviews the script before saving.
- **Schema drift.** The generation targets `core.MechanicsSpec`; a change to it must update the
  prompt and the parser. A test that loads a generated system into the editor catches a drift.
- **Over-generated JavaScript.** The schema-first instruction is the mitigation; SG-2 formalises it.
