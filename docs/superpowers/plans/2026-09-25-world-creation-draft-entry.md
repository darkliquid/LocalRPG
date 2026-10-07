# World Creation Draft Entry Implementation Plan

> **Status:** Implemented and verified against the code on 2026-09-27.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a new world a real, visible, selectable draft entry in the Worlds Studio sidebar with empty forms, persisted only on save, and make every "Create New World" entry point open the studio in new-world mode instead of editing the first saved world.

**Architecture:** The studio replaces its scalar `selectedID` with a discriminated `selection` (`saved` | `draft` | `null`) plus a local `draft`. The sidebar renders the draft row above the saved worlds. The launcher carries an explicit `new | browse` mode so opening the studio from a create tile starts a draft while the dock still browses. The backend splits world create/update so a duplicate id is a `409` rather than a silent overwrite, and a missing update is a `404`.

**Tech Stack:** Go 1.27.1 (`net/http`, `pkg/gui`, `pkg/core`), React 19 + TypeScript + Tailwind v4, Lucide icons (`Plus`, `AlertCircle`, `Trash2`).

**Spec:** `docs/superpowers/specs/2026-09-25-world-creation-draft-entry-design.md`

## Global Constraints

- Go 1.27.1. Standard library only for tests (`testing`, `t.TempDir()`); no testify.
- Use `any`, not `interface{}`; wrap errors with `fmt.Errorf("...: %w", err)`; `go vet ./...` must stay clean.
- TypeScript: `strict`, `noUnusedLocals`, `noUnusedParameters`; `npx tsc --noEmit` is the frontend gate.
- No on-disk format changes; drafts are in-memory only.
- `POST /api/worlds` must never overwrite an existing world; `PUT /api/world/:id` must never create one.
- No `DeleteWorld` endpoint or UI is added.
- Conventional Commits with a scope; subject under 72 characters.

---

## File Map

**Create**
- `pkg/gui/world_crud_test.go` — create/update guard tests.
- `frontend/src/components/launcher/DiscardDraftConfirm.tsx` — small confirmation overlay.

**Modify**
- `pkg/gui/service.go:2249-2291` — split `SaveWorld` into `CreateWorld`/`UpdateWorld` with sentinel errors.
- `pkg/gui/server.go:619-645, 753-774` — route POST to `CreateWorld` (409) and PUT to `UpdateWorld` (404).
- `frontend/src/types.ts` — `WorldSelection`, `WorldDraft`.
- `frontend/src/api/client.ts:276-287` — `createWorld`/`updateWorld`, parse `409` body.
- `frontend/src/components/WorldsStudio.tsx` — selection refactor, draft row, discard confirm, no implicit Ashen Reach lore.
- `frontend/src/components/LauncherHub.tsx:29, 113-165, 179-207` — studio mode state and entry points.

---

### Task 1: Split world create from update

**Files:**
- Modify: `pkg/gui/service.go:2249-2291`
- Test: `pkg/gui/world_crud_test.go`

**Interfaces:**
- Consumes: `core.LoadWorldManifest`, `core.WorldManifest`, `slugify`.
- Produces: `gui.ErrWorldExists`, `gui.ErrWorldNotFound`, `(*Service).CreateWorld(ctx, req CreateWorldRequestDTO) (*WorldDetailDTO, error)`, `(*Service).UpdateWorld(ctx, req CreateWorldRequestDTO) (*WorldDetailDTO, error)`.

- [x] **Step 1: Write the failing test**

Create `pkg/gui/world_crud_test.go`:
```go
package gui

import (
	"context"
	"errors"
	"testing"
)

func TestCreateWorldRefusesDuplicate(t *testing.T) {
	service := newWorldTestService(t)

	first, err := service.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: "Ember Peak"})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}
	if first.ID == "" {
		t.Fatal("CreateWorld returned an empty id")
	}

	if _, err := service.CreateWorld(context.Background(), CreateWorldRequestDTO{ID: first.ID, Name: "Ember Peak"}); !errors.Is(err, ErrWorldExists) {
		t.Fatalf("second CreateWorld err = %v, want ErrWorldExists", err)
	}
}

func TestUpdateWorldRequiresExisting(t *testing.T) {
	service := newWorldTestService(t)
	if _, err := service.UpdateWorld(context.Background(), CreateWorldRequestDTO{ID: "missing", Name: "Missing"}); !errors.Is(err, ErrWorldNotFound) {
		t.Fatalf("UpdateWorld err = %v, want ErrWorldNotFound", err)
	}

	created, err := service.CreateWorld(context.Background(), CreateWorldRequestDTO{Name: "Ember Peak"})
	if err != nil {
		t.Fatalf("CreateWorld: %v", err)
	}
	created.Description = "changed"
	updated, err := service.UpdateWorld(context.Background(), CreateWorldRequestDTO{ID: created.ID, Name: "Ember Peak", Description: "changed"})
	if err != nil {
		t.Fatalf("UpdateWorld: %v", err)
	}
	if updated.Description != "changed" {
		t.Fatalf("description = %q, want changed", updated.Description)
	}
}
```
The helper `newWorldTestService` must build a `Service` whose resolver points at a temp dir. Reuse whatever the existing `pkg/gui` tests use (search `func newTestService` / `TestService` in `pkg/gui/*_test.go`); if none exists, add it to this file:
```go
func newWorldTestService(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	resolver := core.NewPathResolver(root, root, root)
	manager := config.NewManagerWithDir(t.TempDir())
	return NewServiceWithResolver(resolver, manager, nil)
}
```
Adjust the constructor name to the real one used by `pkg/gui` (search `func NewService` in `pkg/gui/service.go` and the test helpers in `pkg/gui/server_test.go`).

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestCreateWorldRefusesDuplicate|TestUpdateWorldRequiresExisting' ./pkg/gui/`
Expected: FAIL (`undefined: ErrWorldExists`, `undefined: CreateWorld`).

- [x] **Step 3: Implement the split**

In `pkg/gui/service.go`, add the sentinels above `SaveWorld` and replace the method with three functions:
```go
// ErrWorldExists reports an attempt to create a world whose id is already taken.
var ErrWorldExists = errors.New("world already exists")

// ErrWorldNotFound reports an attempt to update a world that does not exist.
var ErrWorldNotFound = errors.New("world not found")

// writeWorld writes a world directory. It never decides create vs update; the
// caller does, so a create can refuse a duplicate and an update can require a
// target.
func (s *Service) writeWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("world name is required")
	}
	id := req.ID
	if id == "" {
		id = slugify(req.Name)
	}
	worldDir := s.resolver.WorldDir(id)
	if err := os.MkdirAll(filepath.Join(worldDir, "entities"), 0755); err != nil {
		return nil, fmt.Errorf("create world entities dir: %w", err)
	}
	manifest := core.WorldManifest{
		ID: id, Name: req.Name, Description: req.Description, Genre: req.Genre,
		DefaultSystem: req.DefaultSystem, ArtStyle: req.ArtStyle, Tags: req.Tags,
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal world manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), data, 0644); err != nil {
		return nil, fmt.Errorf("write world.yaml: %w", err)
	}
	if req.LorePrompt != "" {
		promptDir := filepath.Join(worldDir, "prompts")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			return nil, fmt.Errorf("create world prompts dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(promptDir, "lore.md"), []byte(req.LorePrompt), 0644); err != nil {
			return nil, fmt.Errorf("write lore.md: %w", err)
		}
	}
	return s.GetWorld(ctx, id)
}

// CreateWorld writes a new world and refuses an id that is already taken.
func (s *Service) CreateWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error) {
	id := req.ID
	if id == "" {
		id = slugify(req.Name)
	}
	if _, err := os.Stat(filepath.Join(s.resolver.WorldDir(id), "world.yaml")); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrWorldExists, id)
	}
	return s.writeWorld(ctx, req)
}

// UpdateWorld rewrites an existing world and refuses an unknown id.
func (s *Service) UpdateWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error) {
	if _, err := os.Stat(filepath.Join(s.resolver.WorldDir(req.ID), "world.yaml")); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrWorldNotFound, req.ID)
	}
	return s.writeWorld(ctx, req)
}
```
Add `errors` to the imports if absent. Remove `SaveWorld` (the server is its only caller; the next task updates it).

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestCreateWorldRefusesDuplicate|TestUpdateWorldRequiresExisting' ./pkg/gui/`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/service.go pkg/gui/world_crud_test.go
git commit -m "feat(gui): refuse duplicate world creation and missing updates"
```

---

### Task 2: Map create/update routes to statuses

**Files:**
- Modify: `pkg/gui/server.go:628-641, 762-774`
- Test: `pkg/gui/server_test.go` (extend) or `pkg/gui/world_crud_test.go`

**Interfaces:**
- Consumes: `ErrWorldExists`, `ErrWorldNotFound`, `CreateWorld`, `UpdateWorld`.
- Produces: `POST /api/worlds` -> 201 or 409; `PUT /api/world/{id}` -> 200 or 404.

- [x] **Step 1: Write the failing test**

Append to `pkg/gui/world_crud_test.go` a handler-level test using the existing server test harness (search `httptest.NewServer` / `NewServer(` in `pkg/gui/server_test.go` for the pattern):
```go
func TestWorldCreateDuplicateIsConflict(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	body := `{"name":"Ember Peak"}`

	rec := postJSON(t, server, "/api/worlds", body)
	if rec.Code != 201 {
		t.Fatalf("first create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(t, server, "/api/worlds", body)
	if rec.Code != 409 {
		t.Fatalf("duplicate create status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}
```
Use the real helper names from `server_test.go` (`newTestServer`, and whichever helper posts JSON). If the harness uses a different shape, mirror the closest existing test verbatim.

- [x] **Step 2: Run test to verify it fails**

Run: `go test -run TestWorldCreateDuplicateIsConflict ./pkg/gui/`
Expected: FAIL (duplicate returns 400 or overwrites with 201, not 409).

- [x] **Step 3: Implement the route changes**

In `handleWorldsRoutes`, replace the POST branch body (lines 628-641):
```go
	case http.MethodPost:
		var req CreateWorldRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		world, err := s.service.CreateWorld(r.Context(), req)
		if errors.Is(err, ErrWorldExists) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(world)
```
In `handleWorldRoutes`, replace the PUT branch (lines 762-774):
```go
	case http.MethodPut:
		var req CreateWorldRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.ID = worldID
		world, err := s.service.UpdateWorld(r.Context(), req)
		if errors.Is(err, ErrWorldNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, world)
```
Ensure `errors` is imported in `server.go`.

- [x] **Step 4: Run test to verify it passes**

Run: `go test -run TestWorldCreateDuplicateIsConflict ./pkg/gui/` and `go test ./pkg/gui/`
Expected: PASS. Update any existing test that calls `/api/worlds` twice with the same name or expects `SaveWorld`.

- [x] **Step 5: Commit**

```bash
git add pkg/gui/server.go pkg/gui/server_test.go pkg/gui/world_crud_test.go
git commit -m "feat(gui): return 409/404 for world create and update conflicts"
```

---

### Task 3: Frontend selection types and explicit create/update

**Files:**
- Modify: `frontend/src/types.ts:267-299` (near `WorldDetail`/`CreateWorldRequest`)
- Modify: `frontend/src/api/client.ts:276-287`

**Interfaces:**
- Consumes: the backend statuses from Task 2.
- Produces: `WorldSelection`, `WorldDraft`; `APIClient.createWorld`, `APIClient.updateWorld`; a `WorldExistsError extends HTTPError`.

- [x] **Step 1: Add the types**

In `frontend/src/types.ts`:
```ts
export type WorldSelection =
  | { kind: 'saved'; id: string }
  | { kind: 'draft' }
  | null;

export interface WorldDraft {
  localId: string;
  dirty: boolean;
}
```

- [x] **Step 2: Replace `saveWorld` with explicit methods**

In `frontend/src/api/client.ts`, add above `APIClient`:
```ts
// WorldExistsError signals a 409 from world creation, so the form can point the
// user at the slug field instead of showing a generic failure.
export class WorldExistsError extends HTTPError {
  constructor(message: string) {
    super(409, message);
    this.name = 'WorldExistsError';
  }
}
```
Replace `saveWorld` with:
```ts
  static async createWorld(req: CreateWorldRequest): Promise<WorldDetail> {
    const res = await fetch('/api/worlds', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (res.status === 409) throw new WorldExistsError(await res.text());
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }

  static async updateWorld(id: string, req: CreateWorldRequest): Promise<WorldDetail> {
    const res = await fetch(`/api/world/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    });
    if (!res.ok) throw new HTTPError(res.status, await res.text());
    return res.json();
  }
```

- [x] **Step 3: Verify it compiles**

Run: `mise run test:frontend`
Expected: PASS. `WorldsStudio.tsx` still calls `saveWorld`, so it will fail to compile; that is expected and fixed in Task 5. To keep this task self-contained, leave a temporary compatibility shim on the client:
```ts
  static async saveWorld(req: CreateWorldRequest): Promise<WorldDetail> {
    return req.id ? APIClient.updateWorld(req.id, req) : APIClient.createWorld(req);
  }
```
Remove the shim in Task 5 once `WorldsStudio` migrates.

- [x] **Step 4: Commit**

```bash
git add frontend/src/types.ts frontend/src/api/client.ts
git commit -m "feat(frontend): add world selection types and explicit create/update"
```

---

### Task 4: Discard-draft confirmation component

**Files:**
- Create: `frontend/src/components/launcher/DiscardDraftConfirm.tsx`

**Interfaces:**
- Consumes: nothing.
- Produces: `DiscardDraftConfirm` with props `{ isOpen: boolean; onCancel: () => void; onDiscard: () => void }`.

- [x] **Step 1: Create the component**

Create `frontend/src/components/launcher/DiscardDraftConfirm.tsx`:
```tsx
import React from 'react';
import { AlertCircle } from 'lucide-react';

interface DiscardDraftConfirmProps {
  isOpen: boolean;
  onCancel: () => void;
  onDiscard: () => void;
}

export const DiscardDraftConfirm: React.FC<DiscardDraftConfirmProps> = ({ isOpen, onCancel, onDiscard }) => {
  if (!isOpen) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm select-none">
      <div className="w-full max-w-md bg-stone-900 border border-white/15 rounded-2xl p-5 shadow-2xl space-y-4">
        <div className="flex items-center gap-2 text-amber-300">
          <AlertCircle className="w-4 h-4" />
          <h3 className="font-sans text-sm font-bold text-white">Discard unsaved world?</h3>
        </div>
        <p className="text-xs font-sans text-stone-400">
          This world has not been saved. Leaving now discards every field you entered.
        </p>
        <div className="flex justify-end gap-2">
          <button
            onClick={onCancel}
            className="text-xs font-sans px-3 py-1.5 rounded-lg border border-stone-700 text-stone-300 hover:text-white hover:bg-stone-800 transition-all cursor-pointer"
          >
            Keep editing
          </button>
          <button
            onClick={onDiscard}
            className="text-xs font-sans font-bold px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white transition-all cursor-pointer"
          >
            Discard
          </button>
        </div>
      </div>
    </div>
  );
};
```

- [x] **Step 2: Verify it compiles**

Run: `mise run test:frontend`
Expected: PASS (the component is unused until Task 5, but `noUnusedLocals` applies to locals, not exports, so an exported component is fine).

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/launcher/DiscardDraftConfirm.tsx
git commit -m "feat(frontend): add a discard-draft confirmation"
```

---

### Task 5: Refactor Worlds Studio onto the selection model

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx` (whole file; ~660 lines)

**Interfaces:**
- Consumes: `WorldSelection`, `WorldDraft`, `DiscardDraftConfirm`, `APIClient.createWorld`/`updateWorld`.
- Produces: `WorldsStudioProps` gains `startMode?: 'new' | 'browse'`; draft sidebar row; empty lore for saved worlds without a lore prompt.

- [x] **Step 1: Update props, imports, and state**

At the top of `WorldsStudio.tsx`:
```tsx
import { WorldInfo, SystemInfo, WorldEntitySummary, CreateWorldRequest, WorldSelection, WorldDraft } from '../types';
import { DiscardDraftConfirm } from './launcher/DiscardDraftConfirm';
import { WorldExistsError } from '../api/client';

interface WorldsStudioProps {
  onWorldSaved?: () => void;
  startMode?: 'new' | 'browse';
}

export const WorldsStudio: React.FC<WorldsStudioProps> = ({ onWorldSaved, startMode = 'browse' }) => {
```
Replace the state declarations:
```tsx
  const [selection, setSelection] = useState<WorldSelection>(null);
  const [draft, setDraft] = useState<WorldDraft | null>(null);
  const [pendingSelection, setPendingSelection] = useState<WorldSelection>(null);
```
Remove `const [selectedID, setSelectedID] = useState<string | null>(null);`. Add derived values after the state:
```tsx
  const isDraft = selection?.kind === 'draft';
  const savedID = selection?.kind === 'saved' ? selection.id : null;
  const markDirty = () => setDraft((d) => (d ? { ...d, dirty: true } : d));
```

- [x] **Step 2: Rewrite load/select/new functions**

Replace `useEffect`, `loadWorlds`, `loadWorldDetail`, `handleSelectEntity`'s API branch, `handleNewWorld`, and add selection helpers:
```tsx
  useEffect(() => {
    loadWorlds(undefined, startMode);
  }, []);

  const loadWorlds = async (selectID?: string, mode: 'new' | 'browse' = startMode) => {
    setIsLoading(true);
    try {
      const [wList, sList] = await Promise.all([APIClient.listWorlds(), APIClient.listSystems()]);
      setWorlds(wList);
      setSystems(sList);
      if (mode === 'new') {
        handleNewWorld(sList);
        return;
      }
      const target = selectID || (wList.length > 0 ? wList[0].id : null);
      if (target) {
        loadWorldDetail(target);
      } else {
        handleNewWorld(sList);
      }
    } catch (err) {
      setToast({ type: 'error', message: (err as Error).message || 'Failed to load worlds' });
    } finally {
      setIsLoading(false);
    }
  };

  const loadWorldDetail = async (id: string) => {
    try {
      const detail = await APIClient.getWorld(id);
      setSelection({ kind: 'saved', id: detail.id });
      setDraft(null);
      setName(detail.name);
      setSlugID(detail.id);
      setGenre(detail.genre || '');
      setDefaultSystem(detail.default_system || (systems[0]?.id ?? ''));
      setArtStyle(detail.art_style || '');
      setTags(detail.tags ? detail.tags.join(', ') : '');
      setDescription(detail.description || '');
      setLorePrompt(detail.lore_prompt || '');
      setEntities(detail.entities || []);
      // ...keep the existing artwork preview and entity-loading block unchanged
    } catch (err) {
      setToast({ type: 'error', message: (err as Error).message || 'Failed to load world details' });
    }
  };

  const applySelection = (target: WorldSelection) => {
    setPendingSelection(null);
    if (!target || target.kind === 'draft') {
      handleNewWorld();
      return;
    }
    loadWorldDetail(target.id);
  };

  const requestSelection = (target: WorldSelection) => {
    if (isDraft && draft?.dirty) {
      setPendingSelection(target);
      return;
    }
    applySelection(target);
  };

  const handleNewWorld = (sysList?: SystemInfo[]) => {
    setSelection({ kind: 'draft' });
    setDraft({ localId: crypto.randomUUID(), dirty: false });
    setName('');
    setSlugID('');
    setGenre('');
    const availableSys = sysList && sysList.length > 0 ? sysList : systems;
    setDefaultSystem(availableSys[0]?.id ?? '');
    setArtStyle('');
    setTags('');
    setDescription('');
    setLorePrompt('');
    setEntities([]);
    setSelectedEntityID(null);
    setEntityMarkdown('');
    setEntityDrafts({});
    setBannerFile(null);
    setIconFile(null);
    setBannerPreview(null);
    setIconPreview(null);
    setActiveTab('lore');
  };
```
The only change to `loadWorldDetail`'s unchanged block is line 99, now `setLorePrompt(detail.lore_prompt || '')`.

- [x] **Step 3: Replace every `selectedID` check**

Apply these substitutions throughout the file:
- `if (selectedID) { ... getWorldEntity(selectedID, ...) }` in `handleSelectEntity` -> `if (savedID)`.
- `if (!selectedID) setSlugID(REFERENCE_WORLD_TEMPLATE.id)` in `handleLoadReferenceTemplate` -> `if (isDraft)`.
- `if (selectedID) { generateWorldAsset(selectedID, ...) }` in `handleAIGenerate` -> `if (savedID)`; the `encodeURIComponent(selectedID)` uses become `savedID`.
- `if (!selectedID)` in `handleSaveEntity`/`handleDeleteEntity`/`handleCreateNewEntity` -> `if (isDraft)`.
- `APIClient.saveWorldEntity(selectedID, ...)` / `deleteWorldEntity(selectedID, ...)` -> `savedID` (guard with `if (!savedID) return;` where needed).
- `if (!selectedID)` in the two slug-derivation handlers -> `if (isDraft)`.
- `disabled={!!selectedID}` on the slug input -> `disabled={selection?.kind === 'saved'}`.
- Header `{selectedID ? name || 'Edit World' : 'Create New World'}` -> `{selection?.kind === 'saved' ? name || 'Edit World' : 'Create New World'}`.
- Sidebar `selectedID === w.id` -> `selection?.kind === 'saved' && selection.id === w.id`.
- Sidebar row `onClick={() => loadWorldDetail(w.id)}` -> `onClick={() => requestSelection({ kind: 'saved', id: w.id })}`.
- The New button `onClick={() => handleNewWorld()}` -> `onClick={() => requestSelection({ kind: 'draft' })}`.

Add `markDirty()` to the `onChange` of the name, genre, art_style, tags, description, and lorePrompt inputs, and to the entity create/delete/save handlers when `isDraft`.

- [x] **Step 4: Migrate save to create/update**

Replace the `payload` construction and save call in `handleSaveWorld`:
```tsx
      const payload: CreateWorldRequest = {
        id: isDraft ? slugID.trim() || undefined : undefined,
        name: name.trim(),
        description: description.trim(),
        genre: genre.trim(),
        default_system: defaultSystem || (systems[0]?.id ?? ''),
        art_style: artStyle.trim(),
        tags: parsedTags,
        lore_prompt: lorePrompt.trim(),
      };

      const saved = isDraft
        ? await APIClient.createWorld(payload)
        : await APIClient.updateWorld(savedID as string, payload);
```
In the `catch`, special-case the duplicate id:
```tsx
    } catch (err) {
      if (err instanceof WorldExistsError) {
        setToast({ type: 'error', message: 'A world with this id already exists. Change the name or slug.' });
      } else {
        setToast({ type: 'error', message: (err as Error).message || 'Failed to save world' });
      }
    } finally {
```
The success path already calls `loadWorlds(saved.id)`, which now sets a saved selection.

- [x] **Step 5: Render the draft row and confirmation**

In the sidebar list, before `worlds.map(...)`, add the draft row:
```tsx
          {draft && (
            <div
              onClick={() => requestSelection({ kind: 'draft' })}
              className={`p-3 rounded-xl border transition-all cursor-pointer text-left border-dashed ${
                selection?.kind === 'draft'
                  ? 'bg-purple-950/30 border-purple-500/50'
                  : 'bg-stone-900/40 border-stone-700 hover:bg-stone-800/40'
              }`}
            >
              <div className="flex items-center justify-between gap-2">
                <h4 className="font-sans text-xs font-bold text-stone-200 truncate">
                  {name.trim() || 'Untitled World'}
                </h4>
                <span className="text-[10px] font-mono text-amber-300 bg-amber-950/40 border border-amber-500/30 px-1.5 py-0.5 rounded">
                  unsaved
                </span>
              </div>
            </div>
          )}
```
Render the confirm overlay near the toast block:
```tsx
      <DiscardDraftConfirm
        isOpen={pendingSelection !== null}
        onCancel={() => setPendingSelection(null)}
        onDiscard={() => applySelection(pendingSelection)}
      />
```

- [x] **Step 6: Remove the client shim and verify**

Remove the temporary `saveWorld` shim added in Task 3. Run:
`mise run test:frontend && mise run build:frontend`
Expected: PASS. `noUnusedLocals` will flag any leftover `selectedID` reference; fix each.

- [x] **Step 7: Commit**

```bash
git add frontend/src/components/WorldsStudio.tsx frontend/src/api/client.ts
git commit -m "feat(frontend): add a persisted-on-save world draft entry"
```

---

### Task 6: Signal "new world" from the launcher

**Files:**
- Modify: `frontend/src/components/LauncherHub.tsx:29, 113-165, 179-207`

**Interfaces:**
- Consumes: `WorldsStudioProps.startMode`.
- Produces: `activeStudio` becomes a discriminated studio state; create tiles open `mode: 'new'`, the dock opens `mode: 'browse'`.

- [x] **Step 1: Change the studio state**

Replace `const [activeStudio, setActiveStudio] = useState<'worlds' | 'systems' | null>(null);` with:
```tsx
  type StudioState =
    | { studio: 'worlds'; mode: 'new' | 'browse' }
    | { studio: 'systems' }
    | null;
```
Declare the type above the component (module scope) and use:
```tsx
  const [activeStudio, setActiveStudio] = useState<StudioState>(null);
```

- [x] **Step 2: Update the render branches**

Worlds overlay condition:
```tsx
  if (activeStudio?.studio === 'worlds') {
    return (
      // ...unchanged header...
      <div className="flex-1 overflow-hidden">
        <WorldsStudio onWorldSaved={loadData} startMode={activeStudio.mode} />
      </div>
    );
  }
```
Systems overlay condition:
```tsx
  if (activeStudio?.studio === 'systems') {
```
(The `onWorldSaved` prop for the systems branch is unchanged.)

- [x] **Step 3: Update every entry point**

- Dock: `onOpenWorldsStudio={() => setActiveStudio({ studio: 'worlds', mode: 'browse' })}`.
- Hero `onCreateWorld`: `() => setActiveStudio({ studio: 'worlds', mode: 'new' })`.
- Flyout `onCreateWorld`: `() => { setIsFlyoutOpen(false); setActiveStudio({ studio: 'worlds', mode: 'new' }); }`.
- Gallery `onCreateWorld`: `() => { setIsGalleryOpen(false); setActiveStudio({ studio: 'worlds', mode: 'new' }); }`.
- Systems studio open: `onOpenSystemsStudio={() => setActiveStudio({ studio: 'systems' })}`.

Because the studio unmounts when `activeStudio` clears, `startMode` is read on mount and no nonce is needed.

- [x] **Step 4: Verify**

Run: `mise run test:frontend && mise run build:frontend`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add frontend/src/components/LauncherHub.tsx
git commit -m "feat(frontend): open Worlds Studio in new-world mode from create tiles"
```

---

### Task 7: Full verification

- [x] **Step 1: Run the whole suite**

Run: `mise run test` and `mise run lint`
Expected: PASS (`go test -v -count=1 ./...`, `npx tsc --noEmit`, `go vet ./...`).

- [x] **Step 2: Manual checklist**

1. With at least one saved world, open the dock's Worlds Studio and confirm it opens the first saved world (browse behaviour unchanged).
2. Open Worlds Studio from a "Create New World" tile and confirm a dashed "Untitled World / unsaved" row appears, forms are empty, and no saved world is highlighted.
3. Edit the draft, then press New or click a saved world; confirm the discard confirmation appears. Confirm it, then run `ls worlds/` and verify no new directory was created.
4. Fill in a name and Save; confirm the draft row becomes the saved world and its content persists across a reload.
5. Open a saved world whose `prompts/lore.md` is absent; confirm the lore field is empty and that "Load Reference Template" still fills Ashen Reach.
6. Create a world whose derived slug matches an existing world; confirm a clear "already exists" toast and that the existing world on disk is untouched.

- [x] **Step 3: Commit any fixups**

```bash
git add -A
git commit -m "test: verify world creation draft entry"
```
