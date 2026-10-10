# Settings Studio Restructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Break the dense Settings panel into navigable panels with secondary tabs and an advanced disclosure, without changing what is persisted.

**Architecture:** Mechanical extraction first, then structure: each tab body moves to a focused component under `components/settings/` and each provider editor to `components/providers/`; two new UI primitives (`SubTabs`, `AdvancedSection`) carry the secondary bars and disclosure; a `useAdvanced` hook persists the toggle in `localStorage`.

**Tech Stack:** React 19, TypeScript (strict, `noUnusedLocals`), Tailwind v4, Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-10-settings-studio-restructure-design.md`
**Issue:** [#126](https://github.com/darkliquid/LocalRPG/issues/126)

## Global Constraints

- The persisted YAML for an unchanged form must be byte-identical after the change.
- Every panel receives the same `{ config, setConfig }` pair; no panel keeps its own copy.
- `tsc --noEmit` and `npm run build` are the gate; no unused imports may survive extraction.

## File Map

| File | Change |
| --- | --- |
| `frontend/src/components/ui/SubTabs.tsx` | new: secondary tab bar |
| `frontend/src/components/ui/AdvancedSection.tsx` | new: disclosure |
| `frontend/src/hooks/useAdvanced.ts` | new: `localStorage`-backed toggle |
| `frontend/src/components/settings/*Panel.tsx` | new: extracted tab bodies |
| `frontend/src/components/providers/*Editor.tsx` | new: extracted provider editors |
| `frontend/src/components/SettingsStudio.tsx` | reduced to the shell |

---

### Task 1: `SubTabs`, `AdvancedSection`, `useAdvanced`

**Files:**
- Create: `frontend/src/components/ui/SubTabs.tsx`, `AdvancedSection.tsx`, `frontend/src/hooks/useAdvanced.ts`
- Test: beside each

- [ ] **Step 1: Write the failing tests** for tab switching, `aria-selected`, collapsed-by-default, and default-false persistence.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the three.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 2: Extract the provider editors

**Files:**
- Create: `frontend/src/components/providers/TTSProviderEditor.tsx`, `STTProviderEditor.tsx`, `ImageProviderEditor.tsx`, `LLMRoleEditor.tsx`, `CloudKeysPanel.tsx`, `EmbeddingsPanel.tsx`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [ ] **Step 1: Confirm the existing SettingsStudio tests pass before moving code.**
- [ ] **Step 2: Extract each form into its component, wiring the same props.**
- [ ] **Step 3: Run `npm run build` to catch unused imports.**
- [ ] **Step 4: Run the SettingsStudio tests and confirm they still pass.**
- [ ] **Step 5: Commit.**

### Task 3: Extract the tab bodies and add secondary tabs

**Files:**
- Create: `frontend/src/components/settings/PathsPanel.tsx`, `ProvidersPanel.tsx`, `AgentsPanel.tsx`, `MediaPanel.tsx`, `PreferencesPanel.tsx`, `ContextLimitsPanel.tsx`, `GenerationLimitsPanel.tsx`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [ ] **Step 1: Write a failing test** that each top tab renders its secondary bar and switches panels.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Extract the panels, remove the three list-only media managers from Providers, and wire `SubTabs`.**
- [ ] **Step 4: Run the tests and `npm run build`.**
- [ ] **Step 5: Commit.**

### Task 4: Advanced gating

**Files:**
- Modify: `frontend/src/components/providers/LLMRoleEditor.tsx`, `frontend/src/components/settings/AgentsPanel.tsx`, `frontend/src/components/SettingsStudio.tsx`
- Test: the panel tests

- [ ] **Step 1: Write failing tests** that the limit fields are hidden when advanced is off and shown when on, and that the role's `max_tokens`/`temperature` sit behind the disclosure.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Wrap the groups in `AdvancedSection` and gate the Advanced sub-tab on `useAdvanced`.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 5: Save-persistence regression

**Files:**
- Test: a fixture-config round-trip test

- [ ] **Step 1: Write a test** that saves an unchanged fixture config and asserts the serialised YAML equals the pre-change output.
- [ ] **Step 2: Run it; fix any panel that mutated the draft on mount.**
- [ ] **Step 3: Commit.**
