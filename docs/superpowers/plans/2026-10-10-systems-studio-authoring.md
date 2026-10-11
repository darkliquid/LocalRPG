# Systems Studio Authoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Make the Systems Studio authoring story coherent: one non-destructive base-system entry point, examples throughout, a place to define and edit tests, and character-creation prompts that mean something.

**Architecture:** Frontend-led. `From a Base` seeds a new draft instead of overwriting the open system; a per-entry `example` carries the sandbox documentation. The Tests tab edits `systemtest.Scenario` files through two new service methods, and character-creation fields gain `options`/`default` and are rendered by the campaign form.

**Tech Stack:** React 19, TypeScript (strict, `noUnusedLocals`), Tailwind v4, Vitest + React Testing Library; Go 1.27 and `pkg/systemtest` for the Tests tab.

**Spec:** `docs/superpowers/specs/2026-10-10-systems-studio-authoring-design.md`
**Issue:** [#127](https://github.com/darkliquid/LocalRPG/issues/127)

## Global Constraints

- The scenario YAML format is unchanged; the editor is a structured builder over `systemtest.Scenario`.
- A saved scenario is re-parsed with `systemtest.LoadScenario` before the write lands.
- `DefaultCharacterFields()` remains the fallback, so systems that author nothing are unchanged.
- **This plan is being executed in halves**, as the spec's rollout suggests. This half is the authoring polish (spec §4.1 and §4.3). The remainder is deferred, below.

## File Map

| File | Change |
| --- | --- |
| `frontend/src/components/SystemsStudio.tsx` | Starting points removed, `handleStartFromBase` |
| `frontend/src/components/BaseSystemCatalogue.tsx`, `.test.tsx` | relabel, start-from-base semantics |
| `frontend/src/components/mechanics/ScriptReference.tsx`, `.test.tsx` | per-entry examples |

---

### Task 1: One base-system entry point (spec §4.1)

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx`, `BaseSystemCatalogue.tsx`, `BaseSystemCatalogue.test.tsx`

- [x] **Step 1: Write the failing test.** The catalogue test now looks for "Start from this base".
- [x] **Step 2: Run it to verify it fails.** `cd frontend && npx vitest run src/components/BaseSystemCatalogue.test.tsx`
- [x] **Step 3: Implement.** Remove the Starting points row; `handleStartFromBase` seeds a new unsaved draft rather than overwriting the open system; relabel the catalogue button.
- [x] **Step 4: Run it to verify it passes.**
- [x] **Step 5: Commit.**

### Task 2: Sandbox API examples (spec §4.3)

**Files:**
- Modify: `frontend/src/components/mechanics/ScriptReference.tsx`, `ScriptReference.test.tsx`

- [x] **Step 1: Write the failing test** that every global and hook has a non-empty `example`.
- [x] **Step 2: Run it to verify it fails.**
- [x] **Step 3: Implement** the `example` field, expandable rendering, and examples for every entry.
- [x] **Step 4: Run it to verify it passes**, plus `npx tsc --noEmit`.
- [x] **Step 5: Commit.**

### Task 3: The Tests tab (spec §4.4)

**Files:**
- Modify: `pkg/systemtest/scenario.go`, `pkg/gui/service.go`, `pkg/gui/server.go`
- Create: `pkg/gui/system_scenario_test.go`, `frontend/src/components/ScenarioEditor.tsx`, `ScenarioEditor.test.tsx`
- Modify: `frontend/src/components/SystemsStudio.tsx`, `frontend/src/api/client.ts`, `frontend/src/types.ts`, `pkg/gui/docs/09-systems-studio.md`, `pkg/gui/docs/22-editing-content.md`

- [x] **Step 1: Write the failing tests** for `SaveSystemScenario` (round-trip, reject step-less), `DeleteSystemScenario` (removes, `fs.ErrNotExist`), the route dispatch, and the editor's save/disable behaviour.
- [x] **Step 2: Run them to verify they fail.**
- [x] **Step 3: Implement** `systemtest.EncodeScenario`, the two service methods, the `POST`/`DELETE` route branches, the client methods, and the Tests tab with a structured editor.
- [x] **Step 4: Run them to verify they pass**, plus `npx vitest run` and `npx tsc --noEmit`.
- [x] **Step 5: Commit.**

### Task 4: The mechanics form (spec §4.2)

**Files:**
- Create: `frontend/src/components/ui/Example.tsx`, `Example.test.tsx`
- Modify: `frontend/src/components/MechanicsEditor.tsx`, `MechanicsEditor.test.tsx`, `frontend/src/types.ts`

- [x] **Step 1: Write the failing tests** for the example primitive and the profile's opposed/tie fields.
- [x] **Step 2: Run them to verify they fail.**
- [x] **Step 3: Implement** the `Example` primitive, attach one to the stats and checks sections, and expose `opposed`/`ties` on `ResolutionProfile` and in `ProfilesEditor`.
- [x] **Step 4: Run them to verify they pass**, plus `npx vitest run` and `npx tsc --noEmit`.
- [x] **Step 5: Commit.**

### Task 5: Character creation prompts (spec §4.5, editor half)

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx`

- [x] **Step 1: Write the failing test.** Covered by the editor suite and `tsc`; the studio's own render is not unit-tested.
- [x] **Step 2: Run it to verify it fails.**
- [x] **Step 3: Implement** the `default` input, the comma-separated options list a `select` needs, per-field help describing every attribute and kind, and a worked example.
- [x] **Step 4: Run it to verify it passes.**
- [x] **Step 5: Commit.**

## Deferred to a later pass on this proposal

- **Spec §4.5, rendering the authored fields in the campaign form.** The editor now collects `options` and `default`, but `NewCampaignModal` still hard-codes its six fields, so a `select` has nothing to render into yet. Making `kind` drive the campaign form is the remaining piece.
- **Spec §4.2, examples on the remaining sections.** Attached to stats and checks; skills, health, advancement, and policy could carry one too.

## Verification

- `cd frontend && npx vitest run && npx tsc --noEmit`; `mise run lint`.
