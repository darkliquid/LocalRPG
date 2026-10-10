# Entity Tree Deletion Design

**Date:** 2026-10-10
**Status:** Proposed
**Issue:** [#128](https://github.com/darkliquid/LocalRPG/issues/128)
**Epic:** [#122 Studio UX and authoring](https://github.com/darkliquid/LocalRPG/issues/122)
**Scope:** `frontend`, `pkg/gui`

---

## 1. Problem

The folder tree's three-dot (`RowMenu`) menu offers **Delete** for folders but not for
entity notes. In `EntityTree.tsx:225-264` the folder branch of the menu array includes
"New subfolder", "Rename folder", and "Delete folder"; the note branch contains exactly
one item, "Move to folder...". There is no Delete item for a note.

Entity deletion does exist, but only as a button in the right-hand editor header:
`WorldsStudio.tsx:1369-1377` renders a `Trash2` button titled "Delete entity template"
that calls `handleDeleteEntity` (`:537-563`). So a user can delete the entity they have
*open*, but not any entity from the list, and the folder they are browsing has a Delete
item their files do not.

The primitives are all present. `RowMenuItem` already supports `destructive`
(`RowMenu.tsx:5-15`), the tree node for a note carries its `entity`
(`treeModel.ts:79`, `EntityTree.tsx:252`), and the world backend already deletes one
note: `Service.DeleteWorldEntity` (`pkg/gui/service.go:5061`) resolves the file via
`findWorldEntityNote` and removes it, served by `DELETE /api/world/{id}/entity/{id}`
(`pkg/gui/server.go:1309`) and `APIClient.deleteWorldEntity` (`client.ts:665`). Only the
menu item, the confirm dialog, and the consumer wiring are missing.

The game-side consumers have less to build on: `ContentStudio` and `CodexDrawer` render
the same `EntityTree`, but there is **no game-entity delete endpoint or client method**
at all. Only `deleteWorldEntity` exists; the game entity route (`server.go:935`) handles
`GET` and `PUT` and nothing else.

## 2. Goals

- An entity note's three-dot menu offers Delete, wherever the tree is used.
- Deleting from the tree is confirmed, then removes the note and its index entry.
- The world path reuses the existing `DeleteWorldEntity`; the game path gains the
  missing endpoint so Content Studio and the Codex drawer can offer the same action.
- Draft (unsaved) world entities delete locally, as they already do from the header.

## 3. Non-goals

- Deleting folders; that already works and is unchanged.
- Multi-select or bulk delete.
- Undo. Deletion is permanent and is confirmed; no trash can is added.

## 4. Design

### 4.1 EntityTree

Add an optional prop:

```ts
interface EntityTreeProps {
  // …
  onDeleteEntity?: (entityId: string) => void;
}
```

The note branch of the menu array gains a destructive item **only when the prop is
provided**, so a consumer that cannot delete does not show a dead action:

```tsx
: data.entity
  ? [
      {
        label: 'Move to folder...',
        icon: <MoveRight className="w-3 h-3" />,
        onSelect: () => setDialog({ kind: 'move-note', entity: data.entity as EntitySummary }),
      },
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
    ]
  : []
```

A `DialogState` variant `{ kind: 'delete-note'; entity: EntitySummary }` is added, and
the dialog rendered with a new `DeleteEntityDialog` in `TreeDialogs.tsx` (which already
exports `FolderNameDialog`, `MoveNoteDialog`, and `DeleteFolderDialog`):

```tsx
interface DeleteEntityDialogProps {
  isOpen: boolean;
  name: string;
  onCancel: () => void;
  onSubmit: () => void;
}
```

It reads "Delete <name>? Its note is removed permanently." The submit handler calls
`onDeleteEntity(dialog.entity.id)` and closes. `treeModel.ts` needs no change; the
dialog counts nothing.

### 4.2 World entities (Worlds Studio)

The `EntityTree` usage at `WorldsStudio.tsx:1318-1351` gains:

```tsx
onDeleteEntity={(id) => void handleDeleteEntity(id)}
```

`handleDeleteEntity` already does the right thing in both modes: draft entities are
filtered out of local state and the selection advanced; saved entities call
`APIClient.deleteWorldEntity(savedID, entityId)` and reload. The header Delete button
stays as a second affordance for the selected entity.

### 4.3 Game entities (Content Studio and Codex drawer)

The game side needs the endpoint the world side already has.

**Backend.** Add to `pkg/gui/service.go`:

```go
// DeleteGameEntity removes one campaign entity note and its index entry.
func (s *Service) DeleteGameEntity(ctx context.Context, gameID, entityID string) error
```

It validates both ids with `pathutil.ValidateID`, resolves the note file under
`games/<id>/entities` (including subfolders, matching the world helper), removes it, and
then `store.DeleteEntity(entityID)` plus a `Syncer` sync, following the pattern
`MergeEntity` already uses for the note it folds away (`service.go:1386-1401`). A missing
note returns an error the handler maps to `404`.

`server.go:935`'s `case "entity"` gains a `DELETE` branch beside its `PUT`, calling the
method and returning `200` (or `404` for an absent note), mirroring the world entity
handler at `:1309`.

**Client.** `frontend/src/api/client.ts`'s game client gains:

```ts
async deleteEntity(entityID: string): Promise<void>  // DELETE /api/game/{gameID}/entity/{entityID}
```

**Wiring.** `ContentStudio.tsx` passes `onDeleteEntity` that calls `client.deleteEntity`
and refreshes `entities` (and clears the open note if it was the deleted one).
`CodexDrawer.tsx` does the same against its game client, and dismisses the drawer if it
was showing the deleted entity.

## 5. Behaviour

| Action | Before | After |
| --- | --- | --- |
| Note's three dots | "Move to folder..." only | "Move to folder...", "Delete" |
| Folder's three dots | New/Rename/Delete | unchanged |
| Delete a world entity from the tree | impossible | confirmed, file removed, in draft or on disk |
| Delete a game entity from the tree | impossible | confirmed, file and index entry removed |
| Delete the open entity from the tree | header button only | menu and header both work |
| A consumer with no delete path | n/a | no Delete item shown |

## 6. Testing

- `frontend`: `EntityTree`'s note menu contains a destructive Delete when `onDeleteEntity`
  is passed and not when it is omitted; `DeleteEntityDialog` confirms and cancels.
  `WorldsStudio.delete.test.tsx` gains a test that deletes from the tree (not just the
  header) and asserts `deleteWorldEntity` was called with the right ids.
- `pkg/gui`: `DeleteGameEntity` removes the file and the index entry and reports a
  missing entity; the `DELETE` branch of the game entity route returns `200`/`404`
  (extend `service_test.go` or the entity tests alongside `system_crud_test.go`).
- `tsc --noEmit` and `npm run build` for the new props and client method.

## 7. Rollout

Frontend plus one small backend addition (a service method and a route branch). The world
path is purely frontend and can ship first; the game path lands with its endpoint.

## 8. Risks

- **Deletion is permanent.** The confirm dialog is the only guard. Use the entity's
  display name in the prompt, not its id, so the user confirms the note they mean.
- **The open note must not be left dangling.** When the deleted entity is the one
  selected, the consumer clears or advances the selection, exactly as
  `handleDeleteEntity` already does for drafts.
- **The index must not keep a dead entity.** The game path mirrors `MergeEntity`'s
  remove-then-sync ordering; if the sync fails the note is gone and the index row with
  it, and `EnsureIndexed` repairs any drift on the next open.
- **Subfolders.** Both delete paths resolve the note by walking `entities/` rather than
  assuming a flat directory, so a note inside a folder deletes correctly.
