# Design Specification: UI Responsive Layout & World Studio Entity Selection

**Date:** 2026-09-21  
**Status:** Approved — pending implementation plan  
**Topic:** Resolve cramped forms, overflow/clipping bugs, and fix world entity selection in Worlds Studio

---

## 1. Problem Statement & Motivation

Several UI and UX issues degrade the user experience across the desktop application:

1. **World Studio Entity Switching Bug:**
   - In `frontend/src/components/WorldsStudio.tsx`, clicking an entity template in the Templates list executes:
     ```tsx
     onClick={() => selectedID && loadEntityContent(selectedID, e.id)}
     ```
   - When creating a new world, or viewing/customizing the reference templates before the world is saved to disk, `selectedID` is `null`. The click handler fails silently, preventing the user from previewing or switching between starter templates (e.g., *Lord Corvus*, *The Ashen Veil*, *Dusk Bell Garrison*).
   - Furthermore, `handleCreateNewEntity` and `handleSaveEntity` early-return when `selectedID` is null, preventing entity template creation or modification while drafting.
   - `WorldsStudio` only stores a single `entityMarkdown` string in state with no in-memory mapping for unsaved drafts.

2. **Severely Cramped Global Settings in Campaign View:**
   - In `frontend/src/App.tsx`, clicking the **Settings** button in the campaign header opens `SettingsStudio` inside a side drawer (`Drawers.tsx`), which is fixed to `w-full max-w-md` (448px width, ~400px inner content).
   - `SettingsStudio` contains rich, multi-column forms (AI agent role routing, media engine parameters, directory paths, and voice profiles with pitch/speed sliders and tag lists) styled with Tailwind breakpoints (`md:grid-cols-2`, `sm:grid-cols-3`).
   - Because Tailwind's responsive breakpoints query *viewport width* rather than container width, a desktop browser window triggers the 2-column or 3-column layout inside the 400px drawer. Inputs are compressed to ~100px, sliders clip, buttons wrap awkwardly, and navigation tabs overflow horizontally.

3. **Modal Clipping on Constrained Viewports:**
   - The New Campaign Creation Wizard in `LauncherHub.tsx` and the New Starter Entity modal in `WorldsStudio.tsx` lack max-height boundaries (`max-h-[...]`) and internal scroll wrappers.
   - On displays with constrained height (or when the "Setup Required" banner expands), the dialog overflows the viewport, pushing form inputs and action buttons ("Cancel" and "Embark on Adventure") off-screen where they cannot be scrolled or clicked.

4. **Studio Header & Layout Collisions:**
   - In `WorldsStudio.tsx` and `SystemsStudio.tsx`, the editor header places the title, directory slug, sub-tabs (`Lore & Atmosphere`, `AI Prompt`, `Templates`), "Reference World" button, and "Save World" button on a single non-wrapping flex row.
   - On narrower displays or tiled windows, buttons collide and overflow off the right edge.

---

## 2. Decisions & Non-Goals

### Decisions Settled in Design
| Area | Decision |
| --- | --- |
| **Settings Presentation** | Render `SettingsStudio` in a centered, spacious modal dialog (`max-w-4xl max-h-[88vh]`) when opened from the campaign view in `App.tsx`, replacing the cramped 448px drawer. |
| **Entity Draft State** | In `WorldsStudio.tsx`, maintain an in-memory dictionary `entityDrafts: Record<string, string>`. Switching entities automatically syncs the active editor text to the draft store and loads the newly selected entity without losing edits. |
| **Draft-Time Entity Management** | Allow adding, deleting, and editing entity templates before the world is saved to disk. When the user clicks **Save World**, the manifest and all modified entity files are synchronized to disk together. |
| **Drawer Sizing System** | Enhance `Drawers.tsx` with a `size?: 'md' | 'lg' | 'xl'` prop. Keep `CharacterSheetDrawer` and `LivingWorldDrawer` at `md` (448px), while expanding `CodexDrawer` and `GraphDrawer` to `xl` (640px) to provide adequate space for note editing and graph visualization. |
| **Modal Shell Pattern** | Standardize all modal dialogs into a three-tier flexbox shell: fixed header (`shrink-0`), scrollable body (`flex-1 min-h-0 overflow-y-auto`), and fixed action footer (`shrink-0`), guaranteeing buttons are never pushed off-screen. |
| **Responsive Studio Headers** | Update `WorldsStudio` and `SystemsStudio` headers to `flex-wrap gap-3` so control clusters wrap cleanly on compact viewports. |
| **Action Console Polish** | Add `flex-wrap gap-2` to mode switcher tabs and enforce `min-w-0` on text input and `shrink-0` on buttons to prevent clipping. |

### Non-Goals
- Changing backend data schemas or API contracts (the current HTTP API routes `/api/world/:id/entity/:entityID` are fully functional and remain unchanged).
- Redesigning the core typography or color palette (the Twintail Launcher dark glass aesthetic is preserved).
- Introducing external state management libraries (Redux, Zustand) — standard React 19 hooks are sufficient and idiomatic.

---

## 3. Detailed Architecture & Design

### 3.1 World Studio Entity Draft Store & Selection

#### Component: `frontend/src/components/WorldsStudio.tsx`
Replace the single-string entity state with a draft-aware entity store:

```tsx
// In-memory entity markdown drafts mapped by entity slug ID
const [entityDrafts, setEntityDrafts] = useState<Record<string, string>>(() => {
  const initial: Record<string, string> = {};
  REFERENCE_WORLD_TEMPLATE.entities.forEach((e) => {
    initial[e.id] = e.markdown;
  });
  return initial;
});
```

#### Entity Switching Flow:
1. `handleSelectEntity(targetID: string)`:
   - If an entity is currently selected (`selectedEntityID`), save the current `entityMarkdown` buffer:
     ```ts
     setEntityDrafts((prev) => ({ ...prev, [selectedEntityID]: entityMarkdown }));
     ```
   - Update `selectedEntityID` to `targetID`.
   - If `entityDrafts[targetID]` exists in memory, load it into `entityMarkdown`.
   - If `selectedID` is present (saved world) and the draft is not yet in memory, fetch it via `APIClient.getWorldEntity(selectedID, targetID)`, cache it into `entityDrafts`, and set `entityMarkdown`.
2. Update the template list item click handler:
   ```tsx
   <div
     key={e.id}
     onClick={() => handleSelectEntity(e.id)}
     className={`... cursor-pointer ...`}
   >
     ...
   </div>
   ```
   *(Removes the `selectedID &&` guard that was silently blocking clicks)*.

#### Draft-Aware Template Creation & Deletion:
- **`handleCreateNewEntity`**:
  - If `!selectedID` (new world draft):
    - Compute clean slug from `newEntitySlug`.
    - Append `{ id: slug, name: slug, type: 'concept' }` to `entities`.
    - Store `STARTER_ENTITY_TEMPLATE` in `entityDrafts[slug]`.
    - Select `slug` and load `STARTER_ENTITY_TEMPLATE` into `entityMarkdown`.
    - Close modal and reset slug input.
  - If `selectedID` (saved world):
    - Call `APIClient.saveWorldEntity(selectedID, slug, STARTER_ENTITY_TEMPLATE)`.
    - Refresh world detail and load new entity.
- **`handleDeleteEntity(entityId: string)`**:
  - In draft mode: filter out of `entities`, delete from `entityDrafts`. If the deleted entity was active, switch selection to the first remaining entity or null.
  - In saved mode: call `APIClient.deleteWorldEntity(selectedID, entityId)`, then refresh.

#### Unified World Save:
- When `handleSaveWorld` is executed:
  - Save world manifest via `APIClient.saveWorld(payload)`.
  - Flush the active editor's contents into `entityDrafts`:
    ```ts
    const currentDrafts = { ...entityDrafts };
    if (selectedEntityID) {
      currentDrafts[selectedEntityID] = entityMarkdown;
    }
    ```
  - For each entity in `entities`, persist its markdown to disk:
    ```ts
    for (const ent of entities) {
      const md = currentDrafts[ent.id] || STARTER_ENTITY_TEMPLATE;
      await APIClient.saveWorldEntity(saved.id, ent.id, md).catch(console.error);
    }
    ```
  - Reload worlds list and select `saved.id`.

---

### 3.2 Global Settings Modal & Drawer Sizing System

#### 1. Settings Presentation in Campaign View (`frontend/src/App.tsx`)
- Decouple Settings from `activeDrawer`. Add state:
  ```tsx
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  ```
- Clicking the header "Settings" button triggers `setIsSettingsOpen(true)`.
- Render a dedicated modal dialog:
  ```tsx
  {isSettingsOpen && (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-6 bg-black/80 backdrop-blur-sm animate-fade-in">
      <div className="relative w-full max-w-4xl max-h-[88vh] bg-stone-900/95 border border-amber-500/30 rounded-2xl shadow-2xl flex flex-col overflow-hidden">
        <div className="flex items-center justify-between px-6 py-4 border-b border-stone-800 shrink-0">
          <div className="flex items-center gap-2">
            <Settings className="w-5 h-5 text-amber-400" />
            <h2 className="font-cinzel text-lg font-bold text-amber-400">Global Configuration</h2>
          </div>
          <button
            onClick={() => setIsSettingsOpen(false)}
            className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>
        <div className="flex-1 min-h-0 overflow-y-auto p-6">
          <SettingsStudio
            onSaved={() => APIClient.getSettings().then((res) => setConfig(res.config))}
          />
        </div>
      </div>
    </div>
  )}
  ```

#### 2. `SettingsStudio.tsx` Header & Container Polish
- In `frontend/src/components/SettingsStudio.tsx`:
  - Update the sub-tab navigation and save status bar to `flex flex-wrap items-center justify-between gap-3 border-b border-stone-800 pb-3`.
  - Clean up `isCompact` prop constraints so that inputs comfortably take advantage of the wider `max-w-4xl` layout.

#### 3. Drawer Sizing System (`frontend/src/components/Drawers.tsx`)
- Update `DrawersProps`:
  ```tsx
  interface DrawersProps {
    isOpen: boolean;
    onClose: () => void;
    title: string;
    children: React.ReactNode;
    size?: 'md' | 'lg' | 'xl';
  }
  ```
- Map size to width classes:
  ```tsx
  const sizeClasses: Record<'md' | 'lg' | 'xl', string> = {
    md: 'max-w-md', // 448px
    lg: 'max-w-lg', // 512px
    xl: 'max-w-xl', // 576px - 640px
  };
  ```
- In `App.tsx`:
  - `CharacterSheetDrawer` -> `size="md"`
  - `LivingWorldDrawer` -> `size="md"`
  - `GraphDrawer` -> `size="xl"` (allows canvas to render cleanly without edge clipping)
  - `CodexDrawer` -> `size="xl"` (allows wide markdown editing and backlink reading)

---

### 3.3 Modal Shell Pattern (Scroll Constraints & Pinned Footers)

#### Component: `frontend/src/components/LauncherHub.tsx` (New Campaign Wizard)
Refactor the wizard dialog from an unconstrained block into a three-tier flexbox shell:
```tsx
<div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-fade-in">
  <div className="relative w-full max-w-lg max-h-[85vh] flex flex-col rounded-2xl bg-stone-900/95 border border-amber-500/30 shadow-2xl overflow-hidden">
    {/* Fixed Header */}
    <div className="flex items-center justify-between border-b border-stone-800 px-6 py-4 shrink-0">
      <div className="flex items-center gap-2">
        <Sparkles className="w-5 h-5 text-amber-400" />
        <h3 className="font-cinzel text-lg font-bold text-amber-400">New Campaign Wizard</h3>
      </div>
      <button onClick={() => setIsWizardOpen(false)} className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors">
        <X className="w-5 h-5" />
      </button>
    </div>

    {/* Scrollable Form Body */}
    <form onSubmit={handleCreateGame} className="flex-1 min-h-0 flex flex-col overflow-hidden">
      <div className="flex-1 overflow-y-auto p-6 space-y-4">
        {/* Setup Required banner, Campaign Title, Rule System & World Setting selects, Protagonist Name */}
      </div>

      {/* Pinned Action Footer */}
      <div className="flex items-center justify-end gap-3 border-t border-stone-800 px-6 py-4 shrink-0 bg-stone-900/80">
        <button type="button" onClick={() => setIsWizardOpen(false)} className="px-4 py-2 text-xs font-cinzel text-stone-400 hover:text-white transition-colors cursor-pointer">
          Cancel
        </button>
        <button type="submit" disabled={isSubmitting || !newGameName.trim() || !newPlayerName.trim() || systems.length === 0 || worlds.length === 0} className="...">
          <Sparkles className="w-3.5 h-3.5" />
          <span>{isSubmitting ? 'Weaving World...' : 'Embark on Adventure'}</span>
        </button>
      </div>
    </form>
  </div>
</div>
```

#### Component: `frontend/src/components/WorldsStudio.tsx` (New Entity Modal)
Apply the same `max-h-[85vh] flex flex-col overflow-hidden` pattern to `isNewEntityModal`, with pinned header, scrollable body, and pinned action footer.

---

### 3.4 Responsive Studio Topbars & Console Polish

#### `WorldsStudio.tsx` & `SystemsStudio.tsx`
- Replace non-wrapping header container with:
  ```tsx
  <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-800/80 pb-3">
    {/* Left title and directory slug badge */}
    <div className="flex items-center gap-3">...</div>

    {/* Right controls cluster */}
    <div className="flex flex-wrap items-center gap-2">
      {/* Sub-tabs */}
      {/* Template reference reset button */}
      {/* Save button */}
    </div>
  </div>
  ```
- In `WorldsStudio.tsx` Tab 3 (Entities):
  Ensure the two-column entity manager uses `w-56 shrink-0` on the template list and `flex-1 min-w-0` on the markdown editor, preventing textarea width calculation bugs.

#### `ActionConsole.tsx`
- In `frontend/src/components/ActionConsole.tsx`:
  - Mode switcher container: `flex flex-wrap items-center gap-2 mb-3`.
  - Input form: `flex items-center gap-2`, with `min-w-0 flex-1` on the text input and `shrink-0` on the mic and submit buttons.

---

## 4. File Map

| File | Change | Description |
| --- | --- | --- |
| `frontend/src/components/WorldsStudio.tsx` | Modify | Add `entityDrafts` in-memory store; fix entity switching `onClick`; draft-aware entity add/delete; synchronized multi-entity world save; responsive header. |
| `frontend/src/App.tsx` | Modify | Decouple Settings from `activeDrawer`; add `isSettingsOpen` modal state and `max-w-4xl` dialog; pass `size="xl"` to `CodexDrawer` and `GraphDrawer`. |
| `frontend/src/components/Drawers.tsx` | Modify | Add `size?: 'md' | 'lg' | 'xl'` prop with width class mappings (`max-w-md`, `max-w-lg`, `max-w-xl`). |
| `frontend/src/components/SettingsStudio.tsx` | Modify | Update top navigation bar to `flex-wrap gap-3`; optimize compact and full layout responsiveness. |
| `frontend/src/components/LauncherHub.tsx` | Modify | Refactor New Campaign Wizard to use fixed header, scrollable body, and pinned footer shell within `max-h-[85vh]`. |
| `frontend/src/components/SystemsStudio.tsx` | Modify | Add responsive wrapping (`flex-wrap gap-3`) to the top header and action button cluster. |
| `frontend/src/components/ActionConsole.tsx` | Modify | Add `flex-wrap gap-2` to mode switcher tabs; add `min-w-0` to text input and `shrink-0` to action buttons. |

---

## 5. Testing & Verification Strategy

1. **Entity Switching Verification**:
   - Open Worlds Studio with no saved worlds (or click "New").
   - Click each starter entity (*The Ashen Veil*, *Lord Corvus*, *Dusk Bell Garrison*) and verify the editor switches smoothly and renders the corresponding Markdown.
   - Edit the markdown of *The Ashen Veil*, switch to *Lord Corvus*, switch back to *The Ashen Veil*, and verify the modifications were preserved in memory.
   - Click "+ Add" in draft mode, enter a new entity slug, and verify it appears in the list and can be edited.
   - Click "Save World" and verify all drafted and edited entities are persisted to disk and reloadable.
2. **Settings Modal Verification**:
   - Launch or resume a campaign in `App.tsx`.
   - Click the "Settings" button in the header. Verify it opens as a spacious `max-w-4xl` modal dialog rather than a cramped 448px drawer.
   - Verify all subtabs (Paths, AI Agents, Media Engines, Preferences) render with comfortable multi-column spacing, sliders are fully usable, and test buttons are aligned.
   - Click the close button (or outside/Escape) to return to the chronicle.
3. **Modal Height & Overflow Verification**:
   - Resize browser window to a constrained height (e.g., 600px).
   - Open the New Campaign Wizard in `LauncherHub`.
   - Verify the header and the "Cancel" / "Embark on Adventure" buttons remain pinned on-screen, and the inner form fields scroll smoothly.
4. **Drawers Sizing Verification**:
   - Open Codex drawer and Knowledge Graph drawer; verify both open at `max-w-xl` (640px) providing ample room for prose and canvas rendering.
   - Open Character Sheet and World Arcs drawers; verify they open at standard `max-w-md` (448px).
5. **Lint and Typecheck Verification**:
   - Run `mise run test:frontend` (`npx tsc --noEmit`) to ensure zero TypeScript errors and no unused imports.
   - Run `mise run test:backend` to confirm all Go tests pass.
