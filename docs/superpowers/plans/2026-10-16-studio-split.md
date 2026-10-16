# Splitting the Two Studios Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each studio becomes a container that owns the state and a view per region, with every view renderable on its own and a smoke test per tab covering the regions that had none.
**Architecture:** The container keeps its path, its export and every hook it holds today; the JSX moves into `components/studios/system/` and `components/studios/world/`, one file per region, each with its own props interface.
**Tech Stack:** React 19, TypeScript 5.9 (strict, noUnusedLocals), Vitest + React Testing Library.
**Spec:** `docs/superpowers/specs/2026-10-16-studio-split-design.md`
**Bead:** LocalRPG-14a.6 (FE-4)
**Branch:** `feat/fe-4-studio-split`

## File Map

| File | Why |
|---|---|
| `frontend/src/components/SystemsStudio.tsx` | the systems container: state, handlers, layout, dialogs |
| `frontend/src/components/studios/system/SystemList.tsx` | the master column (574-700) |
| `frontend/src/components/studios/system/SystemHeader.tsx` | the header, the sub-tabs and the toast (703-961) |
| `frontend/src/components/studios/system/SystemManifestTab.tsx` | tab 1 (962-1292) |
| `frontend/src/components/studios/system/SystemRulesTab.tsx` | tab 2 (1293-1326) |
| `frontend/src/components/studios/system/SystemMechanicsTab.tsx` | tab 2b (1327-1339) |
| `frontend/src/components/studios/system/SystemScriptTab.tsx` | tab 3 (1340-1361) |
| `frontend/src/components/studios/system/SystemTestsTab.tsx` | tab 5 (1362-1440) |
| `frontend/src/components/WorldsStudio.tsx` | the worlds container |
| `frontend/src/components/studios/world/WorldList.tsx` | the master column (705-815) |
| `frontend/src/components/studios/world/WorldHeader.tsx` | the header, the sub-tabs and the toast (816-967) |
| `frontend/src/components/studios/world/WorldLoreTab.tsx` | tab 1 including the artwork block (968-1323) |
| `frontend/src/components/studios/world/WorldPromptTab.tsx` | tab 2 (1324-1358) |
| `frontend/src/components/studios/world/WorldEntitiesTab.tsx` | tab 3 (1359-1478) |
| the two studios' test files | the smoke tests, written before the extraction |

## Global Constraints

- **Verbatim moves.** No class, handler, condition or ordering changes. The one exception is the
  keyed form setter the spec records.
- **A view calls no API and holds no state.** Only the container does.
- **A view takes focused props**, declared in its own interface from what it reads.
- **The container keeps its path and export name**, so the lazy imports in `LauncherHub.tsx` and the
  import in `App.tsx` are untouched.
- **After every extraction**: `npx tsc --noEmit`, `npx eslint src` (0 errors), the studio's tests.

## Task 1: The safety net

- [ ] `SystemsStudio.test.tsx`: a smoke test that mocks the studio's APIs, renders it, and opens each
      of the five tabs in turn asserting a landmark of each (the name field, the rules editor, the
      mechanics editor, the script editor, the scenarios panel).
- [ ] `WorldsStudio.test.tsx`: the same for its three tabs (the lore form, the lore prompt editor, the
      starter entity list).
- [ ] Confirm both pass against the studios as they are, before anything moves.

## Task 2: The systems list and header

- [ ] `SystemList.tsx`: the master column, taking the systems, the selection, the draft and the four
      callbacks that change them.
- [ ] `SystemHeader.tsx`: the title, the sub-tabs, the action buttons and the toast.
- [ ] The container renders both; the smoke test still passes.

## Task 3: The systems tabs

- [ ] `SystemManifestTab.tsx`: tab 1, taking its form values in one object and a keyed setter.
- [ ] `SystemRulesTab.tsx`, `SystemMechanicsTab.tsx`, `SystemScriptTab.tsx`, `SystemTestsTab.tsx`.
- [ ] The container renders the one the active tab selects, as it does today.

## Task 4: The worlds views

- [ ] `WorldList.tsx`, `WorldHeader.tsx`.
- [ ] `WorldLoreTab.tsx` (with the banner and icon block), `WorldPromptTab.tsx`, `WorldEntitiesTab.tsx`.
- [ ] The smoke test still passes.

## Task 5: Gates and the record

- [ ] Measure the two containers and record what each is now.
- [ ] `AGENTS.md`: where a studio's views live and what a view may take.
- [ ] `npx prettier --write`, `npx tsc --noEmit`, `npx eslint src`, `npm run build`.
- [ ] `npx vitest run --coverage` against the floors, `mise run test`, `mise run lint`.
- [ ] Open the pull request, wait for CI, close the bead with the evidence.
