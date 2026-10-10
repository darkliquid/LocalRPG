# Systems Studio Authoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Systems Studio authoring story coherent: one non-destructive base-system entry point, examples throughout, a place to define and edit tests, and character-creation prompts that mean something.

**Architecture:** Frontend-led. `From a Base` clone seeds a new draft instead of overwriting the open system; a reusable `Example` primitive and an `ApiEntry.example` field carry the documentation; a new Tests tab edits `systemtest.Scenario` files through two new service methods; character-creation fields gain `options`/`default` and are rendered by the campaign form.

**Tech Stack:** Go 1.27, `gopkg.in/yaml.v3`, `pkg/systemtest`; React 19, TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-10-10-systems-studio-authoring-design.md`
**Issue:** [#127](https://github.com/darkliquid/LocalRPG/issues/127)

## Global Constraints

- The scenario YAML format is unchanged; the editor is a structured builder over `systemtest.Scenario`.
- A saved scenario is re-parsed with `systemtest.LoadScenario` before the write lands.
- Scenario filenames are `entity.Slugify(name)` under `systems/<id>/tests/`; ids are validated with `pathutil.ValidateID`.
- `DefaultCharacterFields()` remains the fallback, so systems that author nothing are unchanged.

## File Map

| File | Change |
| --- | --- |
| `frontend/src/components/ui/Example.tsx` | new primitive |
| `frontend/src/components/MechanicsEditor.tsx` | examples, `opposed`/`ties` fields |
| `frontend/src/types.ts` | `ResolutionProfile` gains `opposed`, `ties` |
| `frontend/src/components/mechanics/ScriptReference.tsx` | per-entry expandable examples |
| `pkg/gui/service.go`, `server.go`, `types.go` | scenario CRUD |
| `frontend/src/components/ScenarioEditor.tsx` | new |
| `frontend/src/components/SystemsStudio.tsx` | Starting points removed, Tests tab, character creation |
| `frontend/src/components/launcher/NewCampaignModal.tsx` | render authored fields |
| `pkg/gui/docs/09-systems-studio.md`, `22-editing-content.md` | docs |

---

### Task 1: One base-system entry point

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx`
- Test: `frontend/src/components/BaseSystemCatalogue.test.tsx` and the studio tests

- [ ] **Step 1: Write failing tests** that the Starting points row is gone, Clone produces a new draft without touching the open system, and "Replace with 2d6" confirms.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Add `handleStartFromBase`, remove the row, relabel, and guard the reset.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 2: Mechanics form examples and the missing fields

**Files:**
- Create: `frontend/src/components/ui/Example.tsx`
- Modify: `frontend/src/components/MechanicsEditor.tsx`, `frontend/src/types.ts`

- [ ] **Step 1: Write failing tests** that a section renders its example and that `opposed`/`ties` render in `ProfilesEditor`.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Add the primitive, the examples, and the fields.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 3: mechanics.js reference entries

**Files:**
- Modify: `frontend/src/components/mechanics/ScriptReference.tsx`
- Test: `frontend/src/components/mechanics/ScriptReference.test.tsx`

- [ ] **Step 1: Write a failing test** that every global and hook has a non-empty `example` and that each entry expands.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Add the examples and the expandable rendering.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 4: Scenario CRUD on the backend

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/server.go`
- Test: `pkg/gui/system_test_test.go`

- [ ] **Step 1: Write failing tests** for save (round-trip, reject step-less), delete (removes, 404 when absent), and method dispatch.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement `SaveSystemScenario` and `DeleteSystemScenario` and the `POST`/`DELETE` branches.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 5: The Tests tab and the scenario editor

**Files:**
- Create: `frontend/src/components/ScenarioEditor.tsx`
- Modify: `frontend/src/components/SystemsStudio.tsx`, `frontend/src/api/client.ts`

- [ ] **Step 1: Write failing tests** for the empty "no scenarios" state, adding a step with an outcome, and save.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the tab, the editor, and the client methods.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 6: Character creation prompts

**Files:**
- Modify: `frontend/src/components/SystemsStudio.tsx`, `frontend/src/components/launcher/NewCampaignModal.tsx`
- Test: the studio and campaign tests

- [ ] **Step 1: Write failing tests** for the options editor, the `default` input, and the campaign form rendering an authored `select` with its options.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the editor fields, the help, and `CharacterFields(manifest)` rendering.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 7: Docs

**Files:**
- Modify: `pkg/gui/docs/09-systems-studio.md`, `pkg/gui/docs/22-editing-content.md`

- [ ] **Step 1: Replace the stale "Dice & Rules Tester" sentence with the Tests tab and add a worked scenario.**
- [ ] **Step 2: Run `mise run lint:docs`.**
- [ ] **Step 3: Commit.**
