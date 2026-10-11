# Settings Studio Restructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Break the dense Settings panel into navigable panels with secondary tabs and an advanced disclosure, without changing what is persisted.

**Architecture:** A reusable `AdvancedSection` keeps rarely-tuned fields collapsed. Secondary tabs and the panel extraction follow in a later pass.

**Tech Stack:** React 19, TypeScript (strict, `noUnusedLocals`), Tailwind v4, Vitest + React Testing Library.

**Spec:** `docs/superpowers/specs/2026-10-10-settings-studio-restructure-design.md`
**Issue:** [#126](https://github.com/darkliquid/LocalRPG/issues/126)

## Global Constraints

- The persisted YAML for an unchanged form stays byte-identical.
- `tsc --noEmit` and `npm run build` are the gate.
- **This plan is being executed in halves.** This half is the advanced disclosure (spec §4.2). The secondary tabs (§4.1) and the file decomposition (§4.3) follow, and the panel extraction alone is a large mechanical change.

## File Map

| File | Change |
| --- | --- |
| `frontend/src/components/ui/AdvancedSection.tsx`, `.test.tsx` | new disclosure primitive |
| `frontend/src/components/SettingsStudio.tsx` | the two limit cards move behind it |

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

## Deferred to a second pass on this proposal

- **Spec §4.1, the information architecture.** Secondary tab bars under Providers, AI Agents, and Media Engines; removing the Providers tab's list-only media managers; and a `SubTabs` primitive.
- **Spec §4.3, the file decomposition.** Extracting the eight tab bodies and the provider editors from the 3,393-line `SettingsStudio.tsx`, and the `useAdvanced` toggle that persists in `localStorage`.
- **The per-role disclosure.** `max_tokens`, `temperature`, `thinking_budget`, `top_p`, and `top_k` in the role editor.

## Verification

- `cd frontend && npx vitest run && npx tsc --noEmit`; `mise run lint`.
