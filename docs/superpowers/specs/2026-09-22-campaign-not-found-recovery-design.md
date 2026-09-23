# Design Spec: Recovering From a Missing Active Campaign

**Date:** 2026-09-22
**Status:** Draft
**Target:** `frontend/src/App.tsx`, `frontend/src/components/LauncherHub.tsx`, `frontend/src/api/client.ts`

---

## 1. Executive Summary

Opening the desktop app often lands on the play page for a campaign that no longer exists. The page is inert: the header shows the stale campaign ID, the chronicle shows `Opening the chronicle...` forever, and the only escape is the Campaigns button. The cause is that the remembered campaign ID is restored from `localStorage` and never validated against the campaign list, so a deletion (or a different `--dir` root) leaves a dangling selection.

This spec makes the remembered campaign a *hint*, not an authority: the app verifies it against the campaign list on startup, falls back to the launcher when it is gone, and shows an explicit recovery state instead of an infinite spinner when a load fails for any other reason.

---

## 2. Findings

### 2.1 The remembered ID is trusted unconditionally

`frontend/src/App.tsx:40` initialises `activeGameID` straight from storage:

```ts
const [activeGameID, setActiveGameID] = useState<string | null>(() => {
  return localStorage.getItem('localrpg_active_game') || null;
});
```

Nothing checks that this campaign still exists. The launcher only renders when `activeGameID` is `null` (`App.tsx:337`).

### 2.2 Failures are swallowed

The corpus effect and `refreshCorpus` both attach `.catch(console.error)` to `getGameState`, `getChronicle`, `getGraph`, and `getRecap` (`App.tsx:75-90`). A deleted campaign therefore leaves `gameState` at `null`, which renders the `Opening the chronicle...` placeholder (`App.tsx:419`) rather than an error or a return to the launcher.

### 2.3 Deletion does not clear the selection

`LauncherHub.runPendingAction` calls `APIClient.deleteGame(id)` and then `loadData()` (`LauncherHub.tsx:107`). It never clears `localrpg_active_game`, which is harmless while the launcher is already open, but it is the mechanism that leaves a stale key behind for the next launch. The same key survives when the campaign directory is removed by hand or the app is pointed at a new `--dir`.

### 2.4 `listGames` is the only authority

`APIClient.listGames()` (`frontend/src/api/client.ts:56`) is static and returns every campaign under the resolved games directory. It is already used by the launcher, so a membership check needs no new backend surface.

---

## 3. Design

### 3.1 Treat the stored ID as a hint

Add a bootstrap effect in `App` that runs when the component mounts (and when `activeGameID` changes to a value restored from storage):

1. If `activeGameID` is `null`, do nothing.
2. Call `APIClient.listGames()`.
3. If the list contains `activeGameID`, keep it and continue loading the campaign.
4. If the list loads successfully and does **not** contain `activeGameID`, the campaign is gone: remove `localrpg_active_game`, call `setActiveGameID(null)`, and return to the launcher.
5. If `listGames()` itself fails (server not up yet, transient error), keep the selection and surface a retryable error state rather than clearing it. A network blip must not erase the player's place.

The list result is cheap (directory read plus manifest parse) and is already fetched by the launcher, so startup cost is unchanged.

### 3.2 A real "campaign unavailable" state

Replace the bare `Opening the chronicle...` placeholder with a distinct states:

- **Loading**: the campaign exists and its first fetch is in flight.
- **Unavailable**: the first `getGameState` failed. Show the reason, a `Return to Campaigns` button, and a `Retry` button. If the failure is a 404, also clear the stored id and switch to the launcher automatically.

To distinguish 404 from a transient failure, `APIClient.getGameState` needs to preserve the status. Introduce a small typed error (`HTTPError` carrying `status`) thrown by the client's fetch helpers, and treat only `404` as "campaign gone".

### 3.3 Clear the key on delete

`LauncherHub` clears `localrpg_active_game` after a successful delete when it matches the deleted id. This is belt-and-braces: the bootstrap check is the real guard, but removing a known-dead key keeps storage honest.

### 3.4 Guard against a switching root

`gui.NewService(rootDir)` resolves relative paths against `rootDir` when `--dir` is set. A different root yields a different games directory, so the stored id from a previous root will simply be absent from `listGames` and fall through to the launcher via 3.1.

---

## 4. Data Flow

```text
App mount
  └─ activeGameID from localStorage
       ├─ null ─────────────────────────────► LauncherHub
       └─ value ─► APIClient.listGames()
                      ├─ contains id ───────► load campaign (existing effect)
                      ├─ absent ────────────► clear key, activeGameID=null ► LauncherHub
                      └─ request failed ────► Campaign unavailable + Retry
```

---

## 5. File Map

| Action | Path | Description |
| :--- | :--- | :--- |
| Modify | `frontend/src/api/client.ts` | Typed `HTTPError` with status; validate/keep helpers |
| Modify | `frontend/src/App.tsx` | Bootstrap membership check; campaign-unavailable state; clear key on 404 |
| Modify | `frontend/src/components/LauncherHub.tsx` | Clear `localrpg_active_game` after deleting the active campaign |
| Modify | `frontend/src/types.ts` | (Only if the error type needs exposing) |

---

## 6. Acceptance Criteria

1. Deleting a campaign and relaunching the app opens the launcher, not a dead play page.
2. Pointing the app at a games directory that lacks the remembered campaign opens the launcher.
3. A transient startup failure keeps the remembered campaign and offers Retry rather than discarding it.
4. The play page never shows an indefinite `Opening the chronicle...` when the campaign cannot be loaded.
5. `npx tsc --noEmit` passes.
