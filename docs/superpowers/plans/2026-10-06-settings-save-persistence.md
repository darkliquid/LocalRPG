# Settings Save Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep the launcher's Global Settings modal open when the user saves; only the X closes it.

**Architecture:** Drop the close from `LauncherHub`'s `onSaved`; the studio already refreshes its own state and shows the confirmation.

**Tech Stack:** React 19 + Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-10-06-settings-save-persistence-design.md`

## Global Constraints

- `noUnusedLocals`/`noUnusedParameters` are on: no unused imports or params.
- The frontend gate is `tsc --noEmit` and `npm run build`.
- Conventional Commits with a scope, subject under 72 chars.

## File Map

- Modify `frontend/src/components/LauncherHub.tsx` - stop closing the modal on save.

---

### Task 1: Do not close on save

**Files:**
- Modify: `frontend/src/components/LauncherHub.tsx`

- [ ] **Step 1: Remove the close**

Change `<SettingsStudio onSaved={() => setIsSettingsOpen(false)} isCompact={false} />` to
`<SettingsStudio isCompact={false} />`. The studio already replaces its state with the saved config
and shows its success message, and `LauncherHub` holds no configuration to refresh.

- [ ] **Step 2: Confirm the X still closes**

Check the header button (`LauncherHub.tsx:335`) still calls `setIsSettingsOpen(false)`, and that
`isSettingsOpen` has no other writer.

- [ ] **Step 3: Typecheck and build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/LauncherHub.tsx
git commit -m "fix(frontend): keep the settings modal open after saving"
```

---

### Task 2: Verification

- [ ] **Step 1: The full gates**

Run: `mise run test && mise run lint`
Expected: PASS and clean.

- [ ] **Step 2: Confirm the acceptance criteria**

- Saving in the launcher's Global Settings keeps the modal open and shows the success message.
- The X still closes it.
- The in-game settings modal is unchanged.
