# Campaign Not Found Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop the desktop app opening onto a deleted campaign and make a failed campaign load recoverable instead of an infinite spinner.

**Architecture:** Add a typed HTTP error to the frontend client, validate the remembered campaign against `listGames()` on startup, fall back to the launcher when it is absent, and replace the bare loading placeholder with a loading/unavailable state that offers Retry and Return to Campaigns. Clear the stored key when a campaign is deleted or definitively missing.

**Tech Stack:** React 19, TypeScript, Tailwind v4, Wails v3.

**Spec:** `docs/superpowers/specs/2026-09-22-campaign-not-found-recovery-design.md`

---

## File Structure Map

| File Path | Responsibility |
| :--- | :--- |
| `frontend/src/api/client.ts` | Typed `HTTPError`; keep the JSON helpers consistent |
| `frontend/src/App.tsx` | Startup membership check, unavailable state, 404 handling |
| `frontend/src/components/LauncherHub.tsx` | Clear the stored campaign key on delete |
| `frontend/src/types.ts` | Expose the error type if components need it |

---

### Task 1: Add a typed HTTP error to the client

**Files:**
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Add the error type**

At the top of `client.ts`:

```ts
export class HTTPError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'HTTPError';
    this.status = status;
  }
}
```

- [ ] **Step 2: Use it for the game-state read**

Update `getGameState` (and the other campaign reads as they are touched) to throw it:

```ts
async getGameState(): Promise<GameState> {
  const res = await fetch(`/api/game/${this.gameID}/state`);
  if (!res.ok) throw new HTTPError(res.status, `getGameState: ${res.statusText}`);
  return res.json();
}
```

Confirm the real path from the existing implementation before editing; keep it unchanged if it differs.

- [ ] **Step 3: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/api/client.ts
git commit -m "feat(frontend): expose the HTTP status of campaign read failures"
```

---

### Task 2: Validate the remembered campaign on startup

**Files:**
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Track campaign availability**

Add state alongside `activeGameID`:

```ts
const [campaignStatus, setCampaignStatus] = useState<'idle' | 'loading' | 'ready' | 'unavailable'>('idle');
const [campaignError, setCampaignError] = useState<string | null>(null);
```

- [ ] **Step 2: Add the bootstrap effect**

Place it after the existing corpus effect:

```ts
// The remembered campaign is only a hint: storage survives a delete or a change
// of --dir, so it is verified against the campaign list before it is trusted.
useEffect(() => {
  if (!activeGameID) {
    setCampaignStatus('idle');
    return;
  }
  let cancelled = false;
  setCampaignStatus('loading');
  APIClient.listGames()
    .then((games) => {
      if (cancelled) return;
      if (games.some((game) => game.id === activeGameID)) {
        setCampaignStatus('ready');
        setCampaignError(null);
        return;
      }
      // Definitively gone: forget it and return to the launcher.
      localStorage.removeItem('localrpg_active_game');
      setActiveGameID(null);
      setCampaignStatus('idle');
    })
    .catch((err) => {
      if (cancelled) return;
      // A transient failure must not erase where the player was.
      setCampaignStatus('unavailable');
      setCampaignError(err instanceof Error ? err.message : String(err));
    });
  return () => {
    cancelled = true;
  };
}, [activeGameID]);
```

- [ ] **Step 3: Handle a 404 from the campaign load**

In the corpus effect (`client.getGameState()`), replace the swallow with:

```ts
client.getGameState()
  .then((state) => {
    setGameState(state);
    setCampaignStatus('ready');
  })
  .catch((err) => {
    if (err instanceof HTTPError && err.status === 404) {
      localStorage.removeItem('localrpg_active_game');
      setActiveGameID(null);
      return;
    }
    setCampaignStatus('unavailable');
    setCampaignError(err instanceof Error ? err.message : String(err));
  });
```

Import `HTTPError` from `./api/client`.

- [ ] **Step 4: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/App.tsx
git commit -m "fix(frontend): verify the remembered campaign before opening it"
```

---

### Task 3: Render a recoverable unavailable state

**Files:**
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Replace the loading placeholder**

Where the play shell currently renders `Opening the chronicle...` when `!gameState`, branch on the status:

- `campaignStatus === 'unavailable'` renders a panel with the error, a **Retry** button (`window.location.reload()` or a `loadData`-style refetch), and a **Return to Campaigns** button calling `handleReturnToLauncher`.
- `campaignStatus === 'loading'` or `ready` with no state yet renders the existing loading text.
- `campaignStatus === 'idle'` should not occur inside the play shell.

- [ ] **Step 2: Keep the launcher authoritative**

Ensure `handleReturnToLauncher` also resets `campaignStatus` to `'idle'` and `campaignError` to `null` so a later selection starts clean.

- [ ] **Step 3: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/App.tsx
git commit -m "fix(frontend): replace the dead campaign spinner with a recovery panel"
```

---

### Task 4: Clear the stored key on delete

**Files:**
- Modify: `frontend/src/components/LauncherHub.tsx`

- [ ] **Step 1: Remove the key after a successful delete**

In `runPendingAction`, after `APIClient.deleteGame(id)`:

```ts
if (localStorage.getItem('localrpg_active_game') === id) {
  localStorage.removeItem('localrpg_active_game');
}
```

- [ ] **Step 2: Typecheck**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/LauncherHub.tsx
git commit -m "fix(frontend): forget a deleted campaign's stored selection"
```

---

### Task 5: Verification

- [ ] **Step 1: Frontend gate**

Run: `cd frontend && npx tsc --noEmit`
Expected: clean.

- [ ] **Step 2: Manual smoke**

1. Create a campaign, open it, delete it from the launcher, reload the app: the launcher appears.
2. With a valid campaign stored, stop the server and reload: the unavailable panel appears with Retry, and storage is unchanged.
3. Start the server and press Retry: the campaign loads.
4. Edit `localStorage.localrpg_active_game` to a missing id and reload: the launcher appears and the key is cleared.
