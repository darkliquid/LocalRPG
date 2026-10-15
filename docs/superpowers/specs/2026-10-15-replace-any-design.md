# Replace `any` and the Unsafe Type Assertions Design

**Date:** 2026-10-15
**Status:** Proposed
**Issue:** gh-189 (the bead LocalRPG-14a.2)
**Epic:** [#152](https://github.com/darkliquid/LocalRPG/issues/152)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (FE-8)
**Scope:** `frontend/src` and `frontend/eslint.config.mjs`. No Go change.

## 1. Problem

The frontend has **19 `@typescript-eslint/no-explicit-any` warnings**, 6 explicit `as any` casts, and
the review that raised this counted **98 `as` assertions**, one double cast through `unknown`, and an
`errorMessage` helper duplicated beside an existing one. Those numbers are re-measured per batch
below rather than trusted; the 19 is what `npx eslint src` reports today.

`any` is not a style preference here, because of where it sits:

- **Two of them are fields of public DTOs** (`types.ts`), which is the worst place: an `any` in the
  wire type propagates into every consumer, and the compiler then has nothing to say about a field
  the whole app reads.
- **An `as` cast silences the compiler at the point where it was working.** A cast at a genuine
  boundary — `JSON.parse(event.data)`, a DOM property the lib types loosely — is honest and needs a
  comment. A cast between two of this app's own types means a type is wrong, and the cast is how a
  bug learns to compile.
- The rule is currently a **warning** in `eslint.config.mjs`, which is the shape this repository uses
  for work it has scheduled: ESLint's error level gates a build and its warnings do not, and a
  proposal promotes its group when it clears it (FE-2, FE-3 and FE-6 are in that state).

## 2. Goals

- **Zero `no-explicit-any` warnings**, and the rule promoted from `warn` to `error` in the same
  change that reaches zero, so the count cannot come back.
- **Every remaining `as` is justified at a boundary**, with a comment saying which boundary and why
  the compiler cannot see it.
- The two duplicate helpers are one helper.
- **No behaviour change**: the app does the same thing, and the compiler knows more about it.

## 3. Non-goals

- **Replacing `any` with `unknown` everywhere as an end in itself.** `unknown` is the safe half of the
  pair and is where a value with no known shape should start — but a value that is *then* narrowed at
  every use is a type that has not been written down yet. Use `unknown` plus a guard where the shape
  genuinely varies, and a real type where it does not.
- **A rigid type for a dynamic bag.** Provider options and problem-detail payloads are
  provider-defined dictionaries; typing them as a closed interface would be a lie the next provider
  breaks. `Record<string, unknown>` and a narrowing accessor is the honest form.
- **The API client's shape** (FE-7) and the state layer (FE-3), which are their own proposals.
- **The Go side.**

## 4. Design

### What replaces each `any`

| Where it is | What it becomes |
|---|---|
| A public DTO field that mirrors the Go wire type | the type the Go side actually sends, or `unknown` plus a narrowing accessor when the Go side is `map[string]any` |
| A generic container or callback (`any` for "some value") | a type parameter, so the caller's type flows through |
| A caught error | `unknown`, narrowed by the existing `errorMessage` helper rather than by a cast |
| A provider-defined option bag | `Record<string, unknown>` with a typed accessor at the point of use |
| A third-party boundary the lib types loosely | the cast stays, with a comment naming the boundary |

### What replaces each unsafe cast

- **A cast at a boundary** (`JSON.parse` on a stream frame, a `SpeechRecognition` event the DOM lib
  types broadly) stays, with a one-line comment saying which boundary it is.
- **A cast between this app's own types** is removed by fixing the type: a field that is optional and
  used as required, a union that should be narrower, or a helper whose return type is wrong.
- **The double cast through `unknown`** is the one that always gets a real fix, because a double cast
  is a deliberate override of the type system and there is no boundary to name.

### Measured

`npx eslint src 2>&1 | grep -c no-explicit-any` is the deliverable's number, and
`grep -rn " as " src --include=*.ts --include=*.tsx | wc -l` is the second. Each batch records both,
so a later batch can see which way they moved rather than trusting this paragraph.

## 5. Behaviour

None. Types only. The one place a change could be behavioural is a **narrowing guard**: a guard that
rejects a value the app used to accept is a bug, so a guard that decides behaviour (does this payload
have a field? is this option a number?) ships with a test, not only a type.

## 6. Testing

- **`tsc` is the proof**, and it is a real one here: a wrong narrowing or a missing field fails the
  build rather than silently typing a value as something else.
- **The existing Vitest suite** must pass unchanged; a batch that needs to change a test's fixtures to
  satisfy a type has probably typed the wrong thing.
- **The count**, before and after each batch.
- `mise run lint:frontend` at 0 errors, then `mise run test` and `mise run lint`.

## 7. Rollout

Batches by area, one pull request each, the bead open across them and its comment recording what has
landed: the **public DTOs first** (they propagate furthest), then the API client's boundaries, then
components, then hooks and libs. The rule flips to `error` in the last batch.

## 8. Risks

- **A guard that lies is worse than an `any`.** `any` defers the problem; a wrong guard *creates* one,
  and it fails at runtime where the cast failed at compile time. Guards that decide behaviour get
  tests.
- **Over-typing a dynamic bag** makes the next provider's configuration a compile error in a place
  that used to work. `Record<string, unknown>` is the escape and it is not a defeat.
- **A large diff of types is hard to review.** Batched by area, and the batches after the DTOs are
  mechanical.
