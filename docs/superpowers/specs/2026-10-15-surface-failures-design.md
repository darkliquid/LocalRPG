# Surface Failures Instead of Logging Them Design

**Date:** 2026-10-15
**Status:** Proposed
**Issue:** gh-187 (the bead LocalRPG-14a.1)
**Epic:** [#152](https://github.com/darkliquid/LocalRPG/issues/152)
**Proposal:** `docs/architecture/quality-review-2026-10-12.md` (FE-9)
**Scope:** `frontend/src` only. No Go change.

## 1. Problem

**55 console calls in the frontend's source swallow failures, 52 of them `console.error`**, and 21 of
those are in `App.tsx` alone:

| File | Calls |
|---|---|
| `App.tsx` | 21 |
| `components/launcher/CampaignSettingsModal.tsx` | 9 |
| `components/LauncherHub.tsx` | 5 |
| `hooks/useVoiceInput.ts` | 4 |
| `WorldsStudio`, `RegistryModal`, `NewCampaignModal`, `DocsModal` | 2 each |
| 15 more files | 1 each |

A failed save, a failed turn, a failed provider probe: the player's screen does not change. The
console has the reason and the player does not, and `noUnusedLocals` cannot see a dropped error, so
nothing in the toolchain asks a caller to do anything with it.

Two things make this worse than a missing feature. The components that do surface failures do it
their own way — `ContentStudio` and `ContextDrawer` each hold an `error` string local to themselves
— so there is no one place a reader can look for "how does this app tell the user something went
wrong". And a failure raised while a modal is closed, or in a background reconcile, has no
component to belong to at all.

## 2. Goals

- **One way to surface a failure that is not a form's own validation error**, owned by the app rather
  than by whichever component happened to catch it.
- **Every one of the 55 call sites is accounted for**: it reports through that mechanism, or it stays
  silent with a written reason and stays silent on purpose.
- The mechanism is **announced**, not merely drawn: a failure the player cannot see is the bug.
- A test proves a failing path reaches the screen.

## 3. Non-goals

- **Form validation.** A component that owns a form keeps its inline `error` state: it is closer to
  the field that is wrong. This proposal is for failures that have no field to sit beside.
- **A logging layer.** `console` stays as the developer's record. The proposal is that it stops being
  the *only* record.
- **Reporting everything.** A failure the player can do nothing about, or one that is expected, must
  not become noise. See the rule below.
- **The Go side**, which already returns errors with classes (`harness.FailureCode`).

## 4. Design

### A failures region, fed by a context

```tsx
// useFailures returns the app's failure reporter. A component that catches
// something the player should know about calls report; a component whose failure
// only concerns its own form keeps using its own error state.
const { report, failures, dismiss } = useFailures();
report({ text: 'Could not save the campaign.', detail: String(err) });
```

- `FailuresProvider` sits in `App.tsx` above everything it reports for, and renders one
  `FailuresRegion`: the newest failure as a sentence, older ones counted, each dismissible, in a
  container with `role="status"` and `aria-live="polite"` so it is announced rather than only drawn.
- A failure carries a **sentence the player can read** and the **detail a developer wants**; the detail
  is shown behind a disclosure, so a stack trace never leads the message.
- Reporting the same text twice in a row replaces rather than stacks, so a retry loop cannot fill the
  region.

### What each call site does

| The call site | What it becomes |
|---|---|
| A user-initiated action that failed (a save, a generate, a probe, a delete) | `report(...)` with a sentence naming the action |
| A load whose failure leaves the app usable but incomplete | `report(...)`, worded as what is missing |
| An expected failure: an aborted fetch on unmount, a cancelled download, a best-effort media or audio path | stays silent, with a comment saying why, and a test where the silence is load-bearing |
| Anything already shown inline by its own form | stays inline, and keeps its console line for the record |

**The rule for the third row is the one that decides whether this is an improvement:** a failure is
reported when the player either caused it or can act on it. An `AbortError` from a component that
unmounted is neither, and reporting it would train the player to ignore the region.

## 5. Behaviour

- A failed save, turn or generation now says so on screen, in a sentence, with the detail available.
- The console keeps its line wherever a developer had one, so a bug report loses nothing.
- Existing inline form errors are unchanged.
- Nothing new is shown for the failures that are expected, which is what keeps the region worth
  reading.

## 6. Testing

- **The region**: renders a reported failure, announces it, dismisses it, and replaces a repeat rather
  than stacking it.
- **One reporting call site per batch**, driven by a mocked API failure, asserting the sentence appears
  — the seam is the fetch layer (`frontend/src/api/client.ts`), which every caller already goes
  through.
- **The silence**: a test per deliberately-silent path, so a later reader cannot quietly turn it into
  noise.
- `npx tsc --noEmit`, `npx vitest run --coverage` (the frontend floors), Prettier and ESLint, then
  `mise run test` and `mise run lint`.

## 7. Rollout

One pull request per batch, so each is reviewable and its own revert; the bead stays open across them
and its comment records what has landed. The provider and the region land first, alone, so the
mechanism can be reviewed before 55 call sites depend on it.

## 8. Risks

- **Over-reporting is worse than under-reporting.** A region that fires on aborts and best-effort
  failures is ignored within a day. The rule in section 4 and the silence tests are the control.
- **A context adds a provider to every test that renders a reporting component.** The hook's default,
  when there is no provider, is to keep the console line rather than throw, so an existing test that
  renders a component in isolation does not break.
- **55 sites is a large diff.** Batched by file group, and the batches after the first are mechanical.
