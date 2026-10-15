# Replace `any` and the Unsafe Type Assertions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Drive `no-explicit-any` to zero, justify every remaining cast at a boundary, and promote the rule to an error.
**Architecture:** Types and narrowing guards at the boundaries; `Record<string, unknown>` where a bag is genuinely provider-defined. No runtime change except where a guard has a test.
**Tech Stack:** TypeScript 5.9, ESLint (typescript-eslint), Vitest.
**Spec:** `docs/superpowers/specs/2026-10-15-replace-any-design.md`

## File Map

| File | Why |
|---|---|
| `frontend/eslint.config.mjs` | the rule's level, and the comment explaining why it moved |
| `frontend/src/types.ts` | the public DTO `any`s, which propagate furthest |
| `frontend/src/api/client.ts` | the stream and JSON boundaries |
| the components and hooks holding the rest | one batch each |

## Global Constraints

- **Measure before and after**: `npx eslint src 2>&1 | grep -c no-explicit-any`, and the ` as ` count.
- **A cast at a boundary stays, with a comment naming the boundary.**
- **A guard that decides behaviour ships with a test.**
- **`tsc` is the proof, and the existing suite must pass unchanged.**
- **The rule flips to `error` in the same change that reaches zero**, which is what keeps it there.

---

### Task 1: Measure

- [ ] **Step 1: The two counts, recorded in the batch's pull request**

Expected today: 19 `no-explicit-any`, 6 `as any`, and the review's 98 `as` (re-count the last).

---

### Task 2: The public DTOs

- [ ] **Step 1: `types.ts`'s `any` fields**

Each becomes the type the Go side sends, or `unknown` plus a narrowing accessor where the Go side is
`map[string]any`. Two of them are public DTO fields and the rest of the file's consumers follow.

- [ ] **Step 2: Test and measure**

---

### Task 3: The API client's boundaries

- [ ] **Step 1: `JSON.parse` on a stream frame**

The frame is the boundary; the shape after parsing is a type, so the cast moves to the boundary and
the rest of the function is typed.

- [ ] **Step 2: The `as any`s that remain in the client**

---

### Task 4: The components

- [ ] **Step 1: One batch per group**, each with its own measurement, splitting by area rather than by file count.

---

### Task 5: Hooks and libs, then the rule

- [ ] **Step 1: The duplicated helper** removed, one copy left in the lib.
- [ ] **Step 2: `no-explicit-any` to `error`** in `eslint.config.mjs`, with a comment saying what it replaced.
- [ ] **Step 3: `mise run lint:frontend`, `mise run test`, `mise run lint`**
