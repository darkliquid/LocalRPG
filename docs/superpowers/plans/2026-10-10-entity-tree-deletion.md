# Entity Tree Deletion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Offer Delete for entity notes in the tree's row menu, with a confirm dialog, wherever the tree is used.

**Architecture:** `EntityTree` gains an optional `onDeleteEntity`; the note branch of its menu adds a destructive Delete item only when the prop is supplied; a `DeleteEntityDialog` confirms; the world path reuses `DeleteWorldEntity`, and the game path gains the missing `DeleteGameEntity` endpoint and client method.

**Tech Stack:** React 19, TypeScript (strict, `noUnusedLocals`), `@headless-tree/react`, Vitest + React Testing Library; Go 1.27, `pkg/gui`, `modernc.org/sqlite`.

**Spec:** `docs/superpowers/specs/2026-10-10-entity-tree-deletion-design.md`
**Issue:** [#128](https://github.com/darkliquid/LocalRPG/issues/128)

## Global Constraints

- A consumer without a working delete path shows no Delete item; the prop gates it.
- Deletion is permanent; the dialog names the entity's display name.
- Game entity deletion removes the note and its index entry, mirroring `MergeEntities`'s remove-then-sync ordering (`pkg/gui/service.go:1386-1401`).
- Errors are wrapped with `fmt.Errorf("...: %w", err)`; no testify.

## File Map

| File | Change |
| --- | --- |
| `frontend/src/components/EntityTree.tsx` | `onDeleteEntity`, menu item, `delete-note` dialog state |
| `frontend/src/components/TreeDialogs.tsx` | `DeleteEntityDialog` |
| `frontend/src/components/EntityTree.test.tsx` | new |
| `frontend/src/components/WorldsStudio.tsx` | wire `onDeleteEntity` |
| `frontend/src/components/WorldsStudio.delete.test.tsx` | a tree-based delete test |
| `pkg/gui/service.go` | `DeleteGameEntity` |
| `pkg/gui/server.go` | `DELETE` branch on the game entity route |
| `pkg/gui/entity_delete_test.go` | new |
| `frontend/src/api/client.ts` | `deleteEntity` |
| `frontend/src/components/ContentStudio.tsx`, `CodexDrawer.tsx` | wire the game path |

---

### Task 1: `EntityTree` menu item and dialog

**Files:**
- Modify: `frontend/src/components/EntityTree.tsx`, `frontend/src/components/TreeDialogs.tsx`
- Test: `frontend/src/components/EntityTree.test.tsx`

- [x] **Step 1: Write the failing test**

```tsx
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import EntityTree from './EntityTree';
import type { EntitySummary } from '../types';

const entity: EntitySummary = { id: 'saltmarch', name: 'Saltmarch', type: 'location' };
const noop = () => {};

function renderTree(onDeleteEntity?: (id: string) => void) {
  render(
    <EntityTree
      folders={[]}
      entities={[entity]}
      onSelect={noop}
      onMoveEntity={noop}
      onMoveFolder={noop}
      onCreateFolder={noop}
      onDeleteFolder={noop}
      onDeleteEntity={onDeleteEntity}
    />,
  );
}

describe('EntityTree note menu', () => {
  it('offers Delete on a note and calls onDeleteEntity after confirmation', async () => {
    const onDeleteEntity = vi.fn();
    renderTree(onDeleteEntity);

    fireEvent.click(screen.getByTitle('Note actions'));
    fireEvent.click(await screen.findByRole('menuitem', { name: /^delete$/i }));
    fireEvent.click(screen.getByRole('button', { name: /delete note/i }));

    await waitFor(() => expect(onDeleteEntity).toHaveBeenCalledWith('saltmarch'));
  });

  it('shows no Delete item when the consumer cannot delete', async () => {
    renderTree(undefined);

    fireEvent.click(screen.getByTitle('Note actions'));
    expect(screen.queryByRole('menuitem', { name: /^delete$/i })).not.toBeInTheDocument();
  });
});
```

- [x] **Step 2: Run it to verify it fails**

Run: `cd frontend && npx vitest run src/components/EntityTree.test.tsx`
Expected: FAIL, the Delete menu item is absent.

- [x] **Step 3: Write the minimal implementation**

Add to `EntityTreeProps`: `onDeleteEntity?: (entityId: string) => void;`

Add to `DialogState`: `| { kind: 'delete-note'; entity: EntitySummary }`.

Destructure `onDeleteEntity` in the component. In the note branch of the menu array (`EntityTree.tsx:252-261`), append the conditional item:

```tsx
...(onDeleteEntity
  ? [
      {
        label: 'Delete',
        icon: <Trash2 className="w-3 h-3" />,
        destructive: true,
        onSelect: () => setDialog({ kind: 'delete-note', entity: data.entity as EntitySummary }),
      },
    ]
  : []),
```

Add `DeleteEntityDialog` to `TreeDialogs.tsx` (import `Trash2`):

```tsx
export interface DeleteEntityDialogProps {
  isOpen: boolean;
  name: string;
  onCancel: () => void;
  onSubmit: () => void;
}

// DeleteEntityDialog names the note, because the deletion is permanent and is the
// only place the blast radius is shown.
export const DeleteEntityDialog: React.FC<DeleteEntityDialogProps> = ({ isOpen, name, onCancel, onSubmit }) => {
  if (!isOpen) return null;
  return (
    <DialogShell
      title="Delete note"
      icon={<Trash2 className="w-4 h-4" />}
      submitLabel="Delete note"
      destructive
      onCancel={onCancel}
      onSubmit={onSubmit}
    >
      <p className="text-xs font-sans text-stone-400">
        Delete <span className="text-stone-200">{name}</span>? Its note is removed permanently.
      </p>
    </DialogShell>
  );
};
```

Render it in `EntityTree`:

```tsx
<DeleteEntityDialog
  isOpen={dialog.kind === 'delete-note'}
  name={dialog.kind === 'delete-note' ? dialog.entity.name : ''}
  onCancel={() => setDialog({ kind: 'none' })}
  onSubmit={() => {
    if (dialog.kind === 'delete-note') onDeleteEntity?.(dialog.entity.id);
    setDialog({ kind: 'none' });
  }}
/>
```

- [x] **Step 4: Run it to verify it passes**

Run: `cd frontend && npx vitest run src/components/EntityTree.test.tsx`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/EntityTree.tsx frontend/src/components/TreeDialogs.tsx frontend/src/components/EntityTree.test.tsx
git commit -m "feat(editor): delete an entity from the tree row menu"
```

### Task 2: Wire the Worlds Studio tree

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`
- Test: `frontend/src/components/WorldsStudio.delete.test.tsx`

- [x] **Step 1: Write the failing test** (a third case in the existing describe)

```tsx
it('deletes from the tree row menu', async () => {
  mockStudio();
  vi.spyOn(APIClient, 'getWorld').mockResolvedValue({
    ...world,
    entities: [{ id: 'saltmarch', name: 'Saltmarch', type: 'location' }],
  });

  render(<WorldsStudio />);
  await waitFor(() => expect(screen.getAllByText('Ember Peak').length).toBeGreaterThan(0));
  fireEvent.click(screen.getAllByText('Ember Peak')[0]);
  await waitFor(() => expect(screen.getByText(/starter entities/i)).toBeInTheDocument());
  fireEvent.click(screen.getByText(/starter entities/i));
  await waitFor(() => expect(screen.getByText(/saltmarch\.md/)).toBeInTheDocument());

  fireEvent.click(screen.getByTitle('Note actions'));
  fireEvent.click(await screen.findByRole('menuitem', { name: /^delete$/i }));
  fireEvent.click(screen.getByRole('button', { name: /delete note/i }));

  await waitFor(() => expect(APIClient.deleteWorldEntity).toHaveBeenCalledWith('ember-peak', 'saltmarch'));
});
```

- [x] **Step 2: Run it to verify it fails**

Run: `cd frontend && npx vitest run src/components/WorldsStudio.delete.test.tsx`
Expected: FAIL, no Note actions Delete item.

- [x] **Step 3: Write the minimal implementation**

In the `EntityTree` usage (`WorldsStudio.tsx:1318-1351`) add:

```tsx
onDeleteEntity={(id) => void handleDeleteEntity(id)}
```

- [x] **Step 4: Run it to verify it passes**

Run: `cd frontend && npx vitest run src/components/WorldsStudio.delete.test.tsx`
Expected: PASS (all three cases).

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx frontend/src/components/WorldsStudio.delete.test.tsx
git commit -m "feat(worlds): delete a template from the tree"
```

### Task 3: `DeleteGameEntity` and its route

**Files:**
- Modify: `pkg/gui/service.go`, `pkg/gui/server.go`
- Test: `pkg/gui/entity_delete_test.go`

- [x] **Step 1: Write the failing test**

```go
package gui

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeleteGameEntityRemovesTheNoteAndIndexEntry(t *testing.T) {
	gameID, svc := setupTestGame(t)

	if err := svc.DeleteGameEntity(context.Background(), gameID, "captain-kaelen"); err != nil {
		t.Fatalf("DeleteGameEntity: %v", err)
	}
	if _, err := svc.GetEntity(context.Background(), gameID, "captain-kaelen"); err == nil {
		t.Fatal("expected the deleted note to be gone from the index")
	}
}

func TestDeleteGameEntityMissingIsNotExist(t *testing.T) {
	gameID, svc := setupTestGame(t)

	err := svc.DeleteGameEntity(context.Background(), gameID, "no-such-note")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestDeleteGameEntityRouteReturnsNotFoundForMissing(t *testing.T) {
	gameID, svc := setupTestGame(t)
	server := NewServer(svc, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodDelete, "/api/game/"+gameID+"/entity/no-such-note", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
```

- [x] **Step 2: Run it to verify it fails**

Run: `go test -run TestDeleteGameEntity ./pkg/gui/`
Expected: FAIL, `svc.DeleteGameEntity` undefined.

- [x] **Step 3: Write the minimal implementation**

Add to `pkg/gui/service.go`, beside `MergeEntities`:

```go
// DeleteGameEntity removes one campaign entity note and its index entry.
func (s *Service) DeleteGameEntity(_ context.Context, gameID, entityID string) error {
	if err := pathutil.ValidateID(gameID); err != nil {
		return fmt.Errorf("invalid game id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return fmt.Errorf("invalid entity id: %w", err)
	}

	entitiesDir := filepath.Join(s.resolver.GameDir(gameID), "entities")
	path, err := s.findEntityNote(entitiesDir, entityID)
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("delete entity %s: %w", entityID, fs.ErrNotExist)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove entity note: %w", err)
	}

	store, err := s.store(gameID)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	if err := store.DeleteEntity(entityID); err != nil {
		return fmt.Errorf("remove entity from the index: %w", err)
	}
	syncer := storage.NewSyncer(store)
	_ = syncer.SyncFile(path)
	return nil
}
```

Add the `DELETE` branch to the game entity route in `pkg/gui/server.go` (in `case "entity"`, after the merge block at `:1024`, before the `GetEntity` fallback):

```go
if r.Method == http.MethodDelete {
	if err := s.service.DeleteGameEntity(r.Context(), gameID, entityID); err != nil {
		writeGameError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
	return
}
```

- [x] **Step 4: Run it to verify it passes**

Run: `go test -run TestDeleteGameEntity ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/server.go pkg/gui/entity_delete_test.go
git commit -m "feat(api): add game entity deletion"
```

### Task 4: Client method and the game consumers

**Files:**
- Modify: `frontend/src/api/client.ts`, `frontend/src/components/ContentStudio.tsx`, `frontend/src/components/CodexDrawer.tsx`

- [x] **Step 1: Write the failing test**

Add a case to `frontend/src/api/client.test.ts`: `new APIClient('game-1').deleteEntity('saltmarch')` issues `DELETE /api/game/game-1/entity/saltmarch`, and a failed response throws its body as the message. The tree behaviour itself is covered by the `EntityTree` and Worlds Studio tests; the consumer wiring is thin glue verified by `tsc`.

- [x] **Step 2: Run it to verify it fails**

Run: `cd frontend && npx vitest run src/api/client.test.ts`
Expected: FAIL, `deleteEntity` is not a function.

- [x] **Step 3: Write the minimal implementation**

In `client.ts`, beside `saveEntity`:

```ts
async deleteEntity(entityID: string): Promise<void> {
  const res = await fetch(`/api/game/${this.gameID}/entity/${entityID}`, { method: 'DELETE' });
  if (!res.ok) throw new Error((await res.text()).trim() || `deleteEntity: ${res.statusText}`);
}
```

In `ContentStudio.tsx`, add to the `EntityTree` usage:

```tsx
onDeleteEntity={async (id) => {
  await client.deleteEntity(id);
  setEntities(await client.listEntities());
  if (note?.id === id) {
    setNote(null);
    setDraft('');
    setSaved('');
  }
}}
```

In `CodexDrawer.tsx`, the same wiring against its game client, and dismiss the drawer when the deleted note is the open one.

- [x] **Step 4: Run it to verify it passes**

Run: `cd frontend && npx vitest run src/components/ContentStudio.delete.test.tsx`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add frontend/src/api/client.ts frontend/src/components/ContentStudio.tsx frontend/src/components/CodexDrawer.tsx frontend/src/components/ContentStudio.delete.test.tsx
git commit -m "feat(content): delete a note from the studio tree"
```

## Verification

- `go test -run TestDeleteGameEntity ./pkg/gui/` and the full `mise run test:backend`.
- `cd frontend && npx vitest run src/components/EntityTree.test.tsx src/components/WorldsStudio.delete.test.tsx src/components/ContentStudio.delete.test.tsx` and `npx tsc --noEmit`.
- `mise run lint` (`go vet` must stay clean).
