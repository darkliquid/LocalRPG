# Settings Save Persistence Design

**Date:** 2026-10-06
**Status:** Proposed
**Issue:** [#31 MP-5](https://github.com/darkliquid/LocalRPG/issues/31) (reported alongside the provider manager)
**Scope:** `frontend`

---

## 1. Problem

The launcher's Global Settings modal closes the moment the user saves. `LauncherHub`
(`frontend/src/components/LauncherHub.tsx:343`) renders the studio as

```tsx
<SettingsStudio onSaved={() => setIsSettingsOpen(false)} isCompact={false} />
```

so the success callback is wired straight to closing the modal. Saving is not a dismissal, and the
user loses their place after every save, which is exactly when they want to keep working.

The in-game modal (`frontend/src/App.tsx:1109`) does it right: its `onSaved` re-fetches the settings
and leaves the modal open. Only the launcher's copy closes on save. The X button is the only control
that should dismiss the modal, and it already does (`LauncherHub.tsx:335`).

This is pre-existing behaviour, independent of the provider manager work.

## 2. Goals

- Saving settings leaves the modal open.
- Only the X (and the modal's own dismiss affordances, if any) closes it.
- The user still gets the saved confirmation, which the studio already shows.

## 3. Non-goals

- Changing what `SaveSettings` persists or returns.
- Any change to the in-game modal, which already behaves correctly.

## 4. Design

Drop the close from the launcher's `onSaved`. `SettingsStudio` already replaces its own state with
the saved configuration (`setConfig(res.config)`) and shows a success message, and `LauncherHub`
holds no configuration of its own (its only prop is `onSelectGame`), so there is nothing to refresh
in the parent. The simplest correct change is to omit the prop entirely:

```tsx
<SettingsStudio isCompact={false} />
```

If a future parent needs to react to a save, it should refresh state there, not close the modal.

## 5. Behaviour

| Action | Before | After |
| --- | --- | --- |
| Save in the launcher modal | modal closes | modal stays open, success message shows |
| Click the X | modal closes | modal closes |
| Save with an error | modal stays open, error shows | unchanged |

## 6. Testing

The frontend has no JS test runner; the gate is `tsc --noEmit` and `npm run build`, plus a manual
check: open the launcher's Global Settings, save, and confirm the modal is still open with the
success message.

## 7. Rollout

Frontend only, one line. No config or API change.

## 8. Risks

- **A hidden reliance on `onSaved`.** `LauncherHub` takes no config and nothing else consumes the
  studio's save, so there is none. A grep for `onSaved` confirms the in-game parent already refreshes
  without closing.
