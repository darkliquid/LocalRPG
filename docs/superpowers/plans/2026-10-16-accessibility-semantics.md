# Accessibility Semantics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every drawn bar is stated or hidden, a turn announces its two ends, the four banners have roles, and every icon-only button carries its own name.
**Architecture:** `ui/ProgressBar.tsx` states a value and `ui/LiveStatus.tsx` announces one; the bars that duplicate adjacent text are hidden; a source-scanning test keeps the names.
**Tech Stack:** React 19, TypeScript 5.9, Vitest + React Testing Library, axe-core.
**Spec:** `docs/superpowers/specs/2026-10-16-accessibility-semantics-design.md`
**Bead:** LocalRPG-14a.4 (FE-6)
**Branch:** `feat/fe-6-semantics`

## File Map

| File | Why |
|---|---|
| `frontend/src/components/ui/ProgressBar.tsx` | new: the one way to state a value |
| `frontend/src/components/ui/ProgressBar.test.tsx` | new: its tests |
| `frontend/src/components/ui/LiveStatus.tsx` | new: an always-mounted `role="status"` region |
| `frontend/src/components/ui/LiveStatus.test.tsx` | new: its tests |
| `frontend/src/test/iconButtons.test.ts` | new: the scan that enforces the name rule |
| `components/theater/TheaterTransport.tsx`, `components/TTSBatchPanel.tsx` | the two bars that state their value |
| `components/CharacterSheetDrawer.tsx`, `components/LivingWorldDrawer.tsx`, `components/providers/TTSProviderEditor.tsx`, `components/ModelDownloadModal.tsx`, `components/ExportModal.tsx` | the seven bars hidden as decoration |
| `App.tsx` | the turn announcement, `aria-busy`, four banner roles, five names |
| `AGENTS.md` | the three rules and their reasons |

## Global Constraints

- **Nothing changes visually.** A converted bar keeps its colour, height and width; only its semantics move.
- **A bar that repeats a number written beside it is hidden, not stated**, and the comment names the text it defers to.
- **A live region is mounted empty and then given its sentence**; a region that appears with its text is announced by few screen readers.
- **Two sentences per turn, no more.**
- **The name rule is enforced by the scan**, so the 24 edits are the last time it is done by hand.

## Task 1: The progress bar component

- [ ] `ProgressBar({ value, max = 100, label, className, barClassName })`: clamp `value` into `0..max`,
      render `role="progressbar"` with `aria-valuenow`, `aria-valuemin`, `aria-valuemax` and the label,
      and derive the fill width from the same clamped value so the drawn width and the stated number
      cannot disagree.
- [ ] Tests: the stated value and its label; a value above `max` clamps to `max`; a value below zero
      clamps to zero; a `max` of 6 reports 3 as `aria-valuenow="3"`, `aria-valuemax="6"` at 50% width.

## Task 2: Nine bars, two stated and seven hidden

- [ ] `TheaterTransport`'s rail becomes `<ProgressBar value={progress} label="Playback position" />`.
- [ ] `TTSBatchPanel`'s job bar becomes a `ProgressBar` labelled "Clips stored", replacing the `title`
      that no assistive technology reads on a `div`.
- [ ] `CharacterSheetDrawer` ×2, `LivingWorldDrawer` ×2, `TTSProviderEditor` ×1, `ModelDownloadModal`
      ×1 and `ExportModal` ×1 take `aria-hidden="true"` with a comment naming the text they defer to.
- [ ] `ImportProgressPanel` is untouched: it already states its value.
- [ ] None of the seven changes colour, height or width.

## Task 3: The live region

- [ ] `LiveStatus({ message })` renders an always-mounted `sr-only` paragraph with `role="status"`,
      empty when there is nothing to say.
- [ ] Tests: present and empty with no message; carrying the sentence when given one; still in the tree
      after the message clears.

## Task 4: The turn announcement

- [ ] `App.tsx`: `turnStatus` state, set to "Narrating the turn…" where `beginTurnStream` starts, and to
      "Turn N ready" from an effect on the chronicle, guarded so the first value (the loaded history) is
      not announced.
- [ ] Render `<LiveStatus message={turnStatus} />` in the chronicle container and put
      `aria-busy={turnInFlight}` on that container.
- [ ] Assert the announced number is the turn that landed.

## Task 5: The four banners

- [ ] `fundsError` and `turnError` gain `role="alert"`.
- [ ] The rate-limit notice and the content warnings gain `role="status"`.
- [ ] Assert each where the surface is renderable, and say in the bead which one is left to the e2e
      suite's launcher test.

## Task 6: Names for the icon-only buttons

- [ ] `frontend/src/test/iconButtons.test.ts`: parse every `.tsx` under `src/` with the TypeScript
      compiler; fail on a `<button>` whose subtree has no text and no expression and which has no
      `aria-label` or `aria-labelledby`; report file and line.
- [ ] Give the 7 that have no name one: `App.tsx:1236`, `:1251` ("Dismiss"),
      `AddEntityModal.tsx:54` ("Close"), `OfflinePreset.tsx:128`, `:197` ("Dismiss"),
      `launcher/CampaignGallery.tsx:251` ("Clear the search"),
      `launcher/CampaignSettingsModal.tsx:238` ("Close").
- [ ] Add `aria-label` beside the `title` on the 17 named only by a tooltip, taking the wording from the
      title and keeping it for the tooltip.
- [ ] Confirm the scan reports zero and the existing tests still pass.

## Task 7: Documentation and gates

- [ ] `AGENTS.md`: the progress rule, the announcement rule and the name rule, each with its reason.
- [ ] `npx tsc --noEmit`, `npx prettier --write`, `npx eslint src` (0 errors), `npm run format:check`.
- [ ] `npx vitest run --coverage` against the floors (statements 33, lines 34, functions 33,
      branches 30).
- [ ] `npm run build`, `mise run test`, `mise run lint`.
- [ ] Open the pull request, wait for CI, close the bead with the evidence.
