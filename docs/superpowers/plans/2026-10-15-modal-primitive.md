# The Modal Primitive Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every blocking overlay uses the modal shell, so a keyboard user can leave a modal and returns where they were.
**Architecture:** `components/Modal.tsx` owns the dialog semantics, the focus, Escape and the backdrop; converted surfaces supply a title and their content.
**Tech Stack:** React 19, TypeScript 5.9, Vitest + React Testing Library.
**Spec:** `docs/superpowers/specs/2026-10-15-modal-primitive-design.md`

## File Map

| File | Why |
|---|---|
| `frontend/src/components/Modal.tsx` | the primitive (landed) |
| `frontend/src/components/Modal.test.tsx` | its six tests (landed) |
| the 33 overlay files | classified, then converted by area |

## Global Constraints

- **The shell owns Escape and the backdrop**: a converted file removes its own handlers in the same change.
- **A conversion keeps its content and its width class**, and takes the shared header.
- **A drawer the page stays usable behind is not converted**; it gets a comment saying so.
- **Each batch is visually checkable** on its own.

---

### Task 1: Classify

- [x] **Step 1: A first pass, by name and marker** (34 files match `fixed inset-0`, including the primitive itself):

  | Class | Count | Files |
  |---|---|---|
  | already a dialog | 3 | `Modal` (the primitive), `AboutModal`, `DocsModal` |
  | drawer or overlay | 4 | `CinematicOverlay`, `CodexDrawer`, `Drawers`, `StoryTheater` |
  | to convert | 27 | the rest |

  **This pass is a heuristic, not the classification.** Two things it cannot decide, and both are why the step is not finished: six of the 27 are *hosts* that contain one or more overlays rather than being overlays themselves (`App.tsx`, `player/main.tsx`, `LauncherHub`, `SystemsStudio`, `WorldsStudio`, `ContentStudio`), so the surface count is higher than the file count; and a name is not a behaviour — `StoryTheater` is a takeover a user must be able to leave, which is a modal's job, while `Drawers.tsx` may hold panels the page stays usable behind. Each is confirmed by reading it, in the batch that converts it.

- [ ] **Step 2: The confirmed count of surfaces that will not convert**, each with its reason.

---

### Task 2: The three that already declare a dialog

- [ ] **Step 1: `AboutModal`, `DocsModal`, `StoryTheater`** gain `aria-modal`, a labelled title and the
      focus behaviour, or convert outright where their layout allows it.

---

### Task 3: The launcher's modals

- [ ] **Step 1:** Convert, and test each as a dialog that opens, closes on Escape and is announced.

---

### Task 4: The entity, codex and studio dialogs

- [ ] **Step 1:** Convert the blocking ones; classify the drawers.

---

### Task 5: The settings, provider, docs, theatre and export surfaces

- [ ] **Step 1:** Convert the remainder, one area per pull request.

---

### Task 6: Verification

- [ ] **Step 1: `grep -rln "fixed inset-0" frontend/src`** returns only the classified non-modals, each with its comment.
- [ ] **Step 2: `grep -rl 'role="dialog"' frontend/src | wc -l`** is the count of real dialogs, not 3.
- [ ] **Step 3: `npx tsc --noEmit && npx vitest run --coverage && mise run lint:frontend`**
- [ ] **Step 4: `mise run test && mise run lint`**
