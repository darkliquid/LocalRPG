# Surface Failures Instead of Logging Them Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the frontend one way to tell the player a failure happened, and account for every one of its 55 console calls.
**Architecture:** A `FailuresProvider` in `App.tsx` owns a list of failures and renders one `FailuresRegion`; components report through a `useFailures` hook. Expected failures stay silent with a reason.
**Tech Stack:** React 19, TypeScript 5.9, Tailwind v4, Vitest + React Testing Library.
**Spec:** `docs/superpowers/specs/2026-10-15-surface-failures-design.md`

## File Map

| File | Why |
|---|---|
| `frontend/src/context/FailuresContext.tsx` | the provider, the hook and the region |
| `frontend/src/context/FailuresContext.test.tsx` | the region's behaviour |
| `frontend/src/App.tsx` | wraps the app in the provider, reports its 21 failures |
| `frontend/src/api/client.ts` | the seam the tests drive failures through |
| the 15 remaining files | each call site, batched |

## Global Constraints

- **Report what the player caused or can act on.** An abort, a cancel, or a best-effort media path stays silent, with a comment saying why and a test pinning the silence.
- **A sentence for the player, the detail for a developer**, and the detail behind a disclosure.
- **The console line stays** where a developer had one: this adds a channel, it does not remove one.
- **A component's own form error stays its own**, and is not reported twice.
- **No provider, no throw**: `useFailures` outside a provider keeps the console line, so a test that renders one component in isolation does not have to wrap it.

---

### Task 1: The mechanism (its own pull request)

- [ ] **Step 1: `FailuresContext.tsx`**

`Failure { id, text, detail? }`; a provider holding the list; `report` that replaces a repeat of the
same text and appends otherwise; `dismiss(id)`; a `FailuresRegion` with `role="status"` and
`aria-live="polite"`, the newest sentence in full, older ones counted, each dismissible, the detail
behind a disclosure.

- [ ] **Step 2: The hook's contract**

`useFailures()` outside a provider returns a reporter that keeps the console line and does not throw.

- [ ] **Step 3: Mount it in `App.tsx`**

The provider wraps the tree; the region renders beside the existing chrome, below the header.

- [ ] **Step 4: Test the region**

Reports, announces, dismisses, and replaces a repeat rather than stacking it. Plus the no-provider case.

---

### Task 2: `App.tsx` (21 calls)

- [ ] **Step 1: Sort the 21 into report or silent**

A save, a turn, a probe, a delete and a load report; an abort on unmount and a best-effort audio path
stay silent with a comment.

- [ ] **Step 2: Replace them, keeping each console line**

- [ ] **Step 3: Test one reporting path end to end**

Drive a failing turn through the mocked client and assert the sentence reaches the region.

---

### Task 3: The studios and modals (`CampaignSettingsModal` 9, `LauncherHub` 5, the four 2s)

The same split, per file, with a test for the loudest path in each.

---

### Task 4: The tail (`useVoiceInput` 4, the 1s)

`useVoiceInput`'s failures are user-visible (the microphone did not work), so they report; the rest are
mostly best-effort and mostly stay silent, each with its reason.

---

### Task 5: Verification

- [ ] **Step 1: `grep -rn "console\." frontend/src` shows only the deliberate ones**, each with a comment above it saying why.
- [ ] **Step 2: `npx tsc --noEmit && npx vitest run --coverage && npm run format:check && npm run lint`**
- [ ] **Step 3: `mise run test && mise run lint`**
