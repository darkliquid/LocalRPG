# Base System Catalogue Design

**Date:** 2026-10-05
**Status:** Proposed
**Issue:** [#88 SG-5](https://github.com/darkliquid/LocalRPG/issues/88)
**Epic:** [#26 AI system generation](https://github.com/darkliquid/LocalRPG/issues/26)
**Proposal:** `docs/proposals/2026-10-05-next-phase-deep-dive.md` §9 (SG-5)
**Depends on:** [#42 SYS-6](https://github.com/darkliquid/LocalRPG/issues/42), [#84 SG-1](https://github.com/darkliquid/LocalRPG/issues/84)
**Scope:** `pkg/refsystems`, `pkg/sysgen`, `pkg/gui`, `frontend`

---

## 1. Problem

SYS-6 ships three reference systems and the studio offers them as starting points. SG-1 and SG-2
generate a system from a description and a template, ignoring the corpus entirely. A user who wants
"a d20 system, but like the reference one with sanity added" has no way to say "start from that one".

The reference systems are the best-tested schemas in the app (SYS-7 runs them). A generation that
begins from one inherits that correctness.

## 2. Goals

- A **base catalogue**: the reference systems (SYS-6) presented as derivable bases.
- **Derive**: generate a variant from a chosen base, keeping its structure and changing what the user
  asks.
- **Clone**: start from a base unchanged and edit it (already possible via the studio; make it
  explicit).
- The catalogue is the SYS-6 list; there is no second source.

## 3. Non-goals

- Generating from nothing (SG-1) or from a template (SG-2).
- Enhancing an existing system (SG-4).
- A user-supplied base beyond the corpus and their own saved systems; a user's own systems are already
  derivable by cloning.

## 4. Design

### 4.1 The catalogue

`refsystems.List()` (SYS-6) is the base catalogue. Each base carries its schema, script, rules, and
its scenarios, so a derived system starts from a tested skeleton. No new data is introduced; the
catalogue is a view of the corpus.

### 4.2 Deriving

`pkg/sysgen` gains a base-aware generation:

```go
// Derive generates a variant of a base system from an instruction, keeping the
// base's structure and changing what the instruction asks.
func Derive(ctx context.Context, gen Generator, base refsystems.ReferenceSystem, instruction string) (System, error)
```

The pipeline is SG-1's, seeded differently:

1. **Seed** — the base's `mechanics`, script, and rules are placed in the prompt as the starting
   point.
2. **Adjust** — the model returns a modified `MechanicsSpec` (and, if needed, script and rules) that
   keeps the base's shape. Because the base is valid, a small change keeps it valid, and SG-3's gate
   confirms it.
3. **Verify** — the smoke gate (SG-3) runs, and the base's own scenarios run too, so a derived system
   that breaks the base's behaviour is caught.

Running the base's scenarios is the extra value: a variant that changes a stat but breaks the base's
expected outcomes is flagged before it is offered.

### 4.3 Cloning

Starting from a base unchanged is the studio's existing "start from a reference system" action
(SYS-6). This spec makes it explicit in the same catalogue UI as Derive, so the two options sit
together: **Clone** (edit by hand) and **Derive** (edit by instruction).

### 4.4 Surfaces

- **API**: `POST /api/system/derive` (base id + instruction → the SG-1 draft shape).
- **Studio**: the base catalogue with Clone and Derive per base, in the new-system flow.

### 4.5 Cost

Derive is SG-1's pipeline with a larger seed prompt, so WG-6's estimate, dry run, and cap apply.

## 5. Behaviour

| Input | Result |
| --- | --- |
| derive `d20_dc` with "add sanity" | a d20 system with sanity, still resolving like the base |
| derive with a change that breaks the base's scenarios | the failure is shown; the draft is not offered |
| clone `narrative_2d6` | the base, editable |
| derive with no instruction | an error (an instruction is required) |
| no provider | the base itself is offered as the draft |

## 6. Testing

- `pkg/sysgen`: `Derive` seeds the prompt with the base; a stub generator returns a valid variant; a
  variant that breaks a base scenario fails validation.
- `pkg/gui`: derive writes nothing until accept; the base's scenarios run.
- `frontend`: the catalogue lists the bases with Clone and Derive.
- A regression guard: cloning a base yields it unchanged.

## 7. Rollout

Additive: a derive endpoint and a studio catalogue view. No existing system changes.

## 8. Risks

- **Base lock-in.** A derived system may keep the base's names. The instruction can rename; the review
  makes it visible.
- **Scenario mismatch.** A user may *want* to change behaviour the base's scenarios assert. The
  scenario failure is shown as a warning, not a block, for a derivation (unlike SG-1's generated
  system, where the gate blocks); the user decides.
- **Catalogue confusion.** Clone and Derive must be clearly different in the UI: one copies, the
  other generates.
