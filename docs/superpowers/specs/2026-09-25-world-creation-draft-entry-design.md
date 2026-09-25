# World Creation Draft Entry Design

**Date:** 2026-09-25
**Status:** Approved
**Scope:** Worlds Studio frontend, Launcher entry points, small world-create backend guard
**Related:** Creation Flow Asset Generation Design (2026-09-24), UI Improvements Design (2026-09-21), Systems and Worlds Studio Design (2026-09-20)

## 1. Overview & Goals

Creating a world in the Worlds Studio is confusing. Pressing "New" blanks the
form but does not add anything to the sidebar, so the screen looks like it is
editing whichever world was previously selected. Opening the studio from any
"Create New World" tile is worse: because the studio always auto-selects the
first saved world, the user lands in an existing world (for example "The Ashen
Reach") and cannot tell they are about to edit it. A saved world that has no
`prompts/lore.md` also silently inherits the Ashen Reach lore prompt, which
compounds the confusion.

This specification makes a new world a real, visible, selectable draft entry in
the sidebar with empty forms, persisted only when the user saves, and makes
every "Create New World" entry point open the studio in new-world mode.

**Goals:**

- Pressing "New" adds a visible draft entry to the sidebar and clears the forms.
- The draft is local until save; abandoning it leaves no files on disk.
- Selecting the draft reopens it; selecting a saved world leaves the editor in a
  consistent state.
- Every external "Create New World" entry point opens the studio in new-world
  mode rather than auto-selecting the first world.
- A saved world with no lore prompt loads an empty lore prompt, not the Ashen
  Reach template. The reference template stays available via an explicit button.
- Creating a world whose derived id already exists does not silently overwrite
  an existing world.

**Non-Goals:**

- Adding a world delete endpoint or UI.
- Persisting drafts to disk or to a database before the user saves.
- Changing the slug derivation rules or the reference template contents.
- Changing the entity draft mechanism beyond renaming the selection checks that
  distinguish draft from saved.
- Reworking the Launcher flyout/gallery again; only the "create" callbacks
  change to signal intent.

**Success Criteria:**

- In an environment with at least one saved world, opening Worlds Studio from
  the dock still browses (auto-selects the first world), while opening it from a
  "Create New World" tile shows a sidebar draft entry and empty forms.
- The sidebar shows the draft with an "unsaved" marker and highlights it when
  selected; saved worlds remain selectable.
- Saving the draft persists it and replaces the draft row with the saved world.
- With no lore prompt for a saved world, the lore editor is empty.
- Creating a world that collides with an existing id returns a clear error
  instead of overwriting.

## 2. Investigation Findings (Root Causes)

Verified in the current tree.

1. **A new world is only ephemeral state.** `handleNewWorld` clears the form
   fields but does not add anything to `worlds`; it relies on
   `selectedID === null` as the sole "draft" signal
   (`frontend/src/components/WorldsStudio.tsx:160-180`).
2. **The sidebar renders persisted worlds only.**
   `worlds.map(...)` iterates the server list, so a draft has no row and no item
   is highlighted (`frontend/src/components/WorldsStudio.tsx:430-465`).
3. **The studio auto-selects the first saved world.** `loadWorlds` picks
   `selectID || wList[0]?.id` and only falls back to a blank slate when there are
   no worlds at all (`frontend/src/components/WorldsStudio.tsx:62-86`).
4. **Entry points do not signal "new".** Every `onCreateWorld` callback merely
   sets `activeStudio = 'worlds'` with no mode
   (`frontend/src/components/LauncherHub.tsx:192-195, 204-207, 220`;
   `launcher/WorldFlyout.tsx:54-64`; `launcher/WorldGallery.tsx:127-137`).
   The studio therefore auto-opens `worlds[0]`.
5. **Implicit Ashen Reach lore injection.** Loading a saved world with no lore
   prompt falls back to `REFERENCE_WORLD_TEMPLATE.lore_prompt`
   (`frontend/src/components/WorldsStudio.tsx:99`). The template is otherwise
   opt-in via the "Load Reference Template" button
   (`WorldsStudio.tsx:182-205, 535-543`;
   `frontend/src/templates/referenceTemplates.ts:111-182`).
6. **No duplicate-id guard.** `SaveWorld` slugifies the name when `id` is empty
   and writes `worlds/<id>/world.yaml` with no existence check, so a name that
   collides with an existing world overwrites it
   (`pkg/gui/service.go:2249-2291`). Create (`POST /api/worlds`) and update
   (`PUT /api/world/:id`) are overloaded through the same method.
7. **`selectedID` boolean checks stand in for selection kind.** Draft-vs-saved
   behaviour is decided by `!selectedID`/`!!selectedID` in several places
   (`WorldsStudio.tsx:336, 351, 380, 587-605, 641`), which conflates "no
   selection" with "draft" and will not survive adding a draft row.

Note: the Creation Flow Asset Generation Design (2026-09-24) section 5 already
made the World Studio default to a blank slate. It did not add a sidebar entry or
remove line 99's implicit fallback; this spec finishes that work.

## 3. Architecture

Two coordinated changes: an explicit selection model in the studio, and an
explicit "new" intent signal from the launcher.

### 3.1 Selection model

Replace the scalar `selectedID` with a discriminated selection that can also
represent a local draft:

```ts
// frontend/src/types.ts
export type WorldSelection =
  | { kind: 'saved'; id: string }
  | { kind: 'draft' }
  | null;

export interface WorldDraft {
  localId: string;          // crypto.randomUUID(), for React keys only
  dirty: boolean;           // any field edited since creation
}
```

The sidebar label is derived from the live form `name`, so the draft does not
carry a second copy that could drift.

The sidebar renders the draft row (pinned above saved worlds) when a draft
exists, followed by the saved `worlds`. The draft row shows
`name || 'Untitled World'` with an "unsaved" pill; saved rows are unchanged.
Highlight is driven by `selection`, not `selectedID`.

Draft lifecycle:

- **Create/reset:** `handleNewWorld()` sets `selection = {kind:'draft'}`,
  creates a fresh `draft` (or resets the existing one), and clears all form
  state exactly as it does today. `name` in the draft is display-only; the Name
  input stays empty per the requirement.
- **Dirty tracking:** form `onChange` handlers (and entity edits) set
  `draft.dirty = true` when `selection.kind === 'draft'`.
- **Discard:** selecting a saved world or pressing New while the draft is dirty
  shows a small inline confirmation ("Discard unsaved world?") using local state
  and the existing modal/overlay styling rather than a native dialog. If the
  draft is not dirty, it is discarded silently.
- **Save:** the existing `handleSaveWorld` is invoked with
  `selectedID = undefined` when `selection.kind === 'draft'`, so
  `saveWorld` issues `POST /api/worlds`. On success the draft is cleared,
  `selection` becomes `{kind:'saved', id: saved.id}`, and `loadWorlds(saved.id)`
  runs as today. Entity drafts continue to be flushed on save.

### 3.2 Launcher intent signal

`LauncherHub` currently stores `activeStudio: 'worlds' | 'systems' | null`. Add a
mode so the studio knows how to open:

```ts
const [activeStudio, setActiveStudio] = useState<
  { studio: 'worlds'; mode: 'new' | 'browse' } | { studio: 'systems' } | null
>(null);
```

- Dock "Worlds Studio" button -> `{ studio: 'worlds', mode: 'browse' }`.
- Flyout/gallery/hero "Create New World" tiles -> `{ studio: 'worlds', mode: 'new' }`.
- `WorldsStudio` takes a `startMode: 'new' | 'browse'` prop (default `'browse'`).
  Because the studio unmounts when `activeStudio` clears, mount-time handling is
  sufficient; no nonce is needed. `startMode === 'new'` calls `handleNewWorld()`
  instead of auto-selecting `worlds[0]`.

Browsing behaviour is preserved: with `mode: 'browse'` the studio keeps
auto-selecting the first saved world, or shows the blank draft when none exist.

### 3.3 Duplicate-id guard (backend)

Distinguish create from update at the service boundary by adding an explicit
method rather than overloading:

```go
// pkg/gui/service.go
func (s *Service) CreateWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error)
func (s *Service) UpdateWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error)
```

- `CreateWorld`: derive the id, then refuse if `worlds/<id>/world.yaml` already
  exists, returning a sentinel `ErrWorldExists` (mapped to `409` by the handler).
- `UpdateWorld`: refuse if the world does not exist (mapped to `404`).
- `SaveWorld` is retained as a thin dispatcher (or removed if nothing else uses
  it); `handleWorldsRoutes` POST calls `CreateWorld`, `handleWorldRoutes` PUT
  calls `UpdateWorld`.

This is the one backend change; it is small and directly protects the new
creation flow.

## 4. Backend Changes

- `pkg/gui/service.go`: add `ErrWorldExists`, `ErrWorldNotFound`, `CreateWorld`,
  `UpdateWorld`; refactor the body of the current `SaveWorld` into a shared
  `writeWorld(dir, req)` helper. Remove `SaveWorld` only if no other caller
  remains (checked: the GUI server is the only caller).
- `pkg/gui/server.go`:
  - `handleWorldsRoutes` POST -> `CreateWorld`; map `ErrWorldExists` to `409`
    with the structured error body from the generation spec's shape (or a plain
    JSON `{"error":{...}}`).
  - `handleWorldRoutes` PUT -> `UpdateWorld`; map `ErrWorldNotFound` to `404`.
  - Add a `DELETE /api/world/:id` only if delete is taken into scope; it is a
    non-goal here, so do not add the route.
- Entity CRUD (`SaveWorldEntity`, `GetWorldEntity`, `DeleteWorldEntity`) is
  unchanged.

## 5. Frontend Changes

### 5.1 `WorldsStudio.tsx`

- State: replace `selectedID: string | null` with `selection: WorldSelection`,
  and add `draft: WorldDraft | null`.
- `loadWorlds(selectID?)`:
  - If `startMode === 'new'`, call `handleNewWorld()` and skip auto-select.
  - Else if `selectID` is given, load it; else auto-select `worlds[0]` or create
    a draft when the list is empty (current fallback).
- `loadWorldDetail(id)`: set `selection = {kind:'saved', id}`; set
  `draft = null`; parse tags; **change line 99 to
  `setLorePrompt(detail.lore_prompt || '')`** so no implicit Ashen Reach content
  appears.
- `handleNewWorld()`: set `selection = {kind:'draft'}`, set `draft =
  {localId, dirty:false}`, and clear all fields and entity drafts as
  today. Preserve the existing `defaultSystem` seeding (first system or empty).
- Sidebar: render the draft row when `draft` is present, then `worlds`. Draft
  row click -> `handleNewWorld()` if no reselect needed, or simply keep the draft
  and refocus; saved row click -> discard-confirm-if-needed then
  `loadWorldDetail(w.id)`.
- Replace every `!selectedID` / `!!selectedID` check with
  `selection?.kind === 'draft'` / `selection?.kind === 'saved'`:
  - entity save branch (`WorldsStudio.tsx:336`),
  - entity delete branch (`WorldsStudio.tsx:351`),
  - entity create branch (`WorldsStudio.tsx:380`),
  - slug auto-derivation on name/AI change (`WorldsStudio.tsx:587-605`),
  - slug input disabled (`WorldsStudio.tsx:641`).
- Header: `selection.kind === 'draft' ? 'Create New World' : name || 'Edit World'`.
- `handleSaveWorld`: build the payload with `id: selection.kind === 'draft' ?
  slugID.trim() || undefined : selection.id`. When creating, catch a `409` and
  show a toast telling the user the id is taken.
- Dirty tracking: set `draft.dirty` from the name/description/genre/tags/
  art_style/lore/entity handlers when in draft mode.
- Import a small `DiscardDraftConfirm` overlay component (new file) or reuse an
  existing modal shell.

### 5.2 `LauncherHub.tsx`

- Change `activeStudio` state to the discriminated shape in section 3.2.
- Update the two studio overlay render branches to pass the mode / no mode.
- Pass `startMode` to `WorldsStudio`.
- Update all `onCreateWorld` callbacks (flyout, gallery, hero) to set
  `{ studio: 'worlds', mode: 'new' }`, and the dock handler to
  `{ studio: 'worlds', mode: 'browse' }`.

### 5.3 `WorldGallery.tsx` / `WorldFlyout.tsx`

No prop changes beyond the parent's callbacks; their `onCreateWorld` buttons
already exist. Optionally add a short helper text to the gallery create tile
noting it starts a new world, but that is cosmetic.

### 5.4 Delete world (explicitly out of scope)

No delete UI or endpoint is added. The spec records this as a known gap so a
future spec can add it; the sidebar draft row does not expose delete.

## 6. Error Handling

- Save failure: existing toast shows the error message; a `409` world-exists
  error shows a specific "A world with this id already exists" message and
  highlights the slug field.
- Duplicate name on save: the backend derives the id from the name; a collision
  returns `409`, which the user resolves by changing the name or slug.
- Discarding a dirty draft requires confirmation; a clean draft is discarded
  silently.
- Draft with no name cannot be saved (`handleSaveWorld` already requires a name).

## 7. Testing & Verification

**Backend (standard library only):**

- `CreateWorld` with a fresh id writes `worlds/<id>/world.yaml` and returns the
  world.
- `CreateWorld` with an existing id returns `ErrWorldExists`; the handler maps it
  to `409`.
- `UpdateWorld` for a missing id returns `ErrWorldNotFound`; the handler maps it
  to `404`.
- Existing world round-trip tests in `pkg/gui/server_test.go` continue to pass.

**Frontend:**

- `mise run test:frontend` (`tsc --noEmit`) must pass. There is no frontend unit
  runner; behaviour is verified manually.
- Manual checklist:
  1. With at least one saved world, open the dock's Worlds Studio; confirm it
     opens the first saved world (browse behaviour unchanged).
  2. Open Worlds Studio from a "Create New World" tile; confirm a sidebar draft
     row labelled "Untitled World" appears, forms are empty, and no saved world
     is highlighted.
  3. Edit the draft; press New or select a saved world; confirm the discard
     confirmation appears, and that confirming leaves no new world on disk
     (`ls worlds/`).
  4. Fill in a name and Save; confirm the draft row becomes the saved world and
     selecting another world and back shows the persisted content.
  5. Open a saved world whose `prompts/lore.md` is absent; confirm the lore field
     is empty, and that "Load Reference Template" still fills Ashen Reach.
  6. Save a draft whose derived slug equals an existing world; confirm a clear
     error and that the existing world is untouched.

## 8. Compatibility & Migration

- No on-disk format changes. Drafts are in-memory only.
- `POST /api/worlds` now returns `409` on an existing id instead of overwriting.
  Existing `SaveWorld` callers in the GUI are updated to `CreateWorld`/
  `UpdateWorld`.
- `WorldInfo`/`WorldDetail` types are unchanged; only new local types
  (`WorldSelection`, `WorldDraft`) are added.
- The `REFERENCE_WORLD_TEMPLATE` constant and "Load Reference Template" button
  are unchanged.

## 9. Open Questions

- Should selecting another saved world while a draft is dirty block the switch,
  or should the draft be kept alongside the saved world's editor? (Current
  proposal: single editor; confirm-and-discard.)
- Should the draft row be dismissed with an explicit "x" as well as by discard
  during selection? (Current proposal: discard is implicit via selecting away or
  pressing New, with confirmation when dirty.)
