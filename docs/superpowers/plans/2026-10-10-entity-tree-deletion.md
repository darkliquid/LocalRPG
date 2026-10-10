# Entity Tree Deletion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Offer Delete for entity notes in the tree's row menu, with a confirm dialog, wherever the tree is used.

**Architecture:** `EntityTree` gains an optional `onDeleteEntity`; the note branch of its menu adds a destructive Delete item only when the prop is supplied; a `DeleteEntityDialog` confirms; the world path reuses `DeleteWorldEntity`, and the game path gains the missing `DeleteGameEntity` endpoint and client method.

**Tech Stack:** React 19, TypeScript, Vitest; Go 1.27, `pkg/gui`.

**Spec:** `docs/superpowers/specs/2026-10-10-entity-tree-deletion-design.md`
**Issue:** [#128](https://github.com/darkliquid/LocalRPG/issues/128)

## Global Constraints

- A consumer without a working delete path shows no Delete item (the prop gates it).
- Deletion is permanent; the dialog names the entity's display name.
- Game entity deletion removes the note and its index entry, mirroring `MergeEntity`'s ordering.

## File Map

| File | Change |
| --- | --- |
| `frontend/src/components/EntityTree.tsx` | `onDeleteEntity`, menu item, dialog state |
| `frontend/src/components/TreeDialogs.tsx` | `DeleteEntityDialog` |
| `frontend/src/components/WorldsStudio.tsx` | wire `onDeleteEntity` |
| `frontend/src/components/ContentStudio.tsx`, `CodexDrawer.tsx` | wire the game path |
| `pkg/gui/service.go`, `server.go` | `DeleteGameEntity` and the route branch |
| `frontend/src/api/client.ts` | `deleteEntity` |

---

### Task 1: `EntityTree` menu item and dialog

**Files:**
- Modify: `frontend/src/components/EntityTree.tsx`, `frontend/src/components/TreeDialogs.tsx`
- Test: a new `EntityTree.test.tsx`

- [ ] **Step 1: Write failing tests** that a note row shows a destructive Delete only when `onDeleteEntity` is passed, and that the dialog confirms and cancels.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Add the prop, the menu item, the dialog state, and the dialog.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 2: Wire the Worlds Studio tree

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`
- Test: `frontend/src/components/WorldsStudio.delete.test.tsx`

- [ ] **Step 1: Write a failing test** that deletes a world entity from the tree (not the header) and asserts `deleteWorldEntity` was called with the right ids.
- [ ] **Step 2: Run it to verify it fails.**
- [ ] **Step 3: Pass `onDeleteEntity={(id) => void handleDeleteEntity(id)}`.**
- [ ] **Step 4: Run it to verify it passes.**
- [ ] **Step 5: Commit.**

### Task 3: `DeleteGameEntity` and its route

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/server.go`
- Test: the entity tests beside `system_crud_test.go`

- [ ] **Step 1: Write failing tests** that the note file and index entry are removed and a missing entity errors.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Implement the method and the `DELETE` branch.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**

### Task 4: Client and the game consumers

**Files:**
- Modify: `frontend/src/api/client.ts`, `frontend/src/components/ContentStudio.tsx`, `frontend/src/components/CodexDrawer.tsx`
- Test: the consumer tests

- [ ] **Step 1: Write failing tests** that deleting from a game tree clears the open note and refreshes the list.
- [ ] **Step 2: Run them to verify they fail.**
- [ ] **Step 3: Add `deleteEntity` and wire both consumers.**
- [ ] **Step 4: Run them to verify they pass.**
- [ ] **Step 5: Commit.**
