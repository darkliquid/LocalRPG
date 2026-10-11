# Settings Studio Restructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Break the dense Settings panel into navigable panels with secondary tabs and an advanced disclosure, without changing what is persisted.

**Architecture:** A reusable `AdvancedSection` keeps rarely-tuned fields collapsed, `SubTabs` splits each dense area, and the 3,900-line component becomes a shell plus one file per panel.

**Tech Stack:** React 19, TypeScript (strict, `noUnusedLocals`), Tailwind v4, Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-10-settings-studio-restructure-design.md`
**Issue:** [#126](https://github.com/darkliquid/LocalRPG/issues/126)

## Global Constraints

- The persisted YAML for an unchanged form stays byte-identical.
- `tsc --noEmit` and `npm run build` are the gate.
- Panel extraction is mechanical: markup moves, behaviour does not.

## File Map

| File | Change |
| --- | --- |
| `frontend/src/components/ui/AdvancedSection.tsx`, `.test.tsx` | disclosure primitive |
| `frontend/src/components/ui/SubTabs.tsx`, `.test.tsx` | secondary tab bar |
| `frontend/src/hooks/useAdvanced.ts`, `.test.ts` | the persisted "show advanced settings" preference |
| `frontend/src/components/settings/PathsPanel.tsx` | storage roots |
| `frontend/src/components/settings/ProvidersPanel.tsx` | cloud keys / embeddings / offline |
| `frontend/src/components/settings/AgentsPanel.tsx` | roles / routing / advanced |
| `frontend/src/components/settings/MediaPanel.tsx` | TTS / STT / images, plus the voice catalogue |
| `frontend/src/components/settings/PreferencesPanel.tsx` | look and feel |
| `frontend/src/components/settings/ContextLimitsPanel.tsx` | context, recall, retrieval, tool rounds |
| `frontend/src/components/settings/GenerationLimitsPanel.tsx` | generation call and chunk limits |
| `frontend/src/components/providers/CloudKeysPanel.tsx` | the shared account keys |
| `frontend/src/components/providers/EmbeddingsPanel.tsx` | the embedding provider |
| `frontend/src/components/providers/TTSProviderEditor.tsx` | one speech engine |
| `frontend/src/components/providers/STTProviderEditor.tsx` | one transcription engine |
| `frontend/src/components/providers/ImageProviderEditor.tsx` | one image generator |
| `frontend/src/components/providers/LLMRoleEditor.tsx` | one LLM role |
| `frontend/src/lib/roleLabels.ts` | the shared role-name vocabulary |
| `frontend/src/components/SettingsStudio.tsx` | the shell: config draft, save, tab bars, modals |

---

### Task 1: The advanced disclosure

**Files:**
- Create: `frontend/src/components/ui/AdvancedSection.tsx`, `AdvancedSection.test.tsx`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Write the failing test** that the section hides its children until expanded, collapses again, and takes a custom label.
- [x] **Step 2: Run it to verify it fails.** `cd frontend && npx vitest run src/components/ui/AdvancedSection.test.tsx`
- [x] **Step 3: Implement** the primitive, and wrap the "Context & Response Limits" and "Generation Limits" cards in the AI Agents tab so they are collapsed by default.
- [x] **Step 4: Run it to verify it passes**, plus `npx tsc --noEmit` and `npx vitest run`.
- [x] **Step 5: Commit.**

### Task 2: The secondary tab bars

**Files:**
- Create: `frontend/src/components/ui/SubTabs.tsx`, `SubTabs.test.tsx`
- Modify: `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Write the failing test** that the bar switches, reports `aria-selected`, and takes a label.
- [x] **Step 2: Run it to verify it fails.**
- [x] **Step 3: Implement** the primitive, then use it under Providers (Cloud Keys, Embeddings, Offline), AI Agents (Roles, Routing, Advanced), and Media Engines (TTS, STT, Images).
- [x] **Step 4: Remove the three list-only `ProviderManager` mounts** from the Providers tab, which duplicated Media Engines without being editable.
- [x] **Step 5: Run the tests and `tsc --noEmit`, then commit.**

### Task 3: The advanced preference

**Files:**
- Create: `frontend/src/hooks/useAdvanced.ts`, `useAdvanced.test.ts`
- Modify: `frontend/src/components/ui/AdvancedSection.tsx`, `frontend/src/components/SettingsStudio.tsx`

- [x] **Step 1: Write the failing test** that the hook defaults to off, reads a stored preference, and persists a change both ways.
- [x] **Step 2: Run it to verify it fails.**
- [x] **Step 3: Implement** the hook on `localStorage` key `localrpg.settings.advanced`, and make `AdvancedSection` re-sync when its `defaultOpen` changes so a section follows the preference and stays user-toggleable after.
- [x] **Step 4: Add the studio-header switch.** With it off, AI Agents shows Roles and Routing and each role's limits stay collapsed; with it on, the Advanced tab appears and the sections open.
- [x] **Step 5: Move the five per-role fields** (`thinking_budget`, `top_p`, `top_k`, `max_tokens`, and temperature) inside one disclosure in the role editor.
- [x] **Step 6: Write a `SettingsStudio` test** that the secondary bars render, the limits are behind their disclosures, the Advanced tab is gated on the preference, and only one media engine shows at a time.
- [x] **Step 7: Run the tests and `tsc --noEmit`, then commit.**

### Task 4: Decompose the component

**Files:** the File Map above.

- [x] **Step 1: Extract the four smallest panels** (paths, preferences, and the two limit panels), verifying `tsc --noEmit` and `vitest run` after each move.
- [x] **Step 2: Extract the providers and agents panels**, moving the sub-tab state, the role-list derivation, and the Gemini model fetch into the panel that uses them.
- [x] **Step 3: Extract the media panel**, moving the selected provider entries, the voice inspection, and the voice catalogue modal with it.
- [x] **Step 4: Extract the three media editors** and the LLM role editor, leaving each provider editor to take the one entry it edits.
- [x] **Step 5: Extract the cloud keys and embeddings cards**, leaving the providers panel as a tab bar over three components.
- [x] **Step 6: Run the full frontend gate, then commit.**

## Verified outcomes

- `SettingsStudio.tsx` is 455 lines, down from 3,933.
- Every panel and provider editor in the spec's file map exists as its own file.
- `npx tsc --noEmit` clean; `npx vitest run` 67 files, 335 tests green.
- The persisted config is untouched: no panel writes anything the shell did not.

## Deferred

- **The Media Engines "Voices & Casting" sub-tab.** The voice profile library is
  per TTS provider entry and lives inside that entry's editor, so a fourth tab
  would either duplicate the TTS editor or need the library lifted into its own
  state. The spec's own note settles the direction: until the editors are moved
  under Providers, Media Engines keeps TTS, STT, and Images, and the voice
  library stays with the engine it belongs to.

## Verification

- `cd frontend && npx vitest run && npx tsc --noEmit`; `mise run lint`.
