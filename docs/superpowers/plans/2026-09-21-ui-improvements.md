# UI Responsive Layout & World Studio Entity Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve cramped UI layouts and overflow/clipping bugs across modals, drawers, and studios, and fix entity template switching and drafting in Worlds Studio.

**Architecture:** Maintain in-memory draft state in `WorldsStudio` (`entityDrafts`) to allow seamless entity template selection and authoring before or after saving. Replace the cramped 448px settings drawer in `App.tsx` with a centered `max-w-4xl` modal dialog. Add sizing support (`size?: 'md' | 'lg' | 'xl'`) to `Drawers.tsx`. Standardize dialogs (Campaign Wizard, New Entity modal) into a three-tier flexbox shell with pinned headers and action footers and scrollable form bodies within `max-h-[85vh]`. Add responsive flex-wrapping to studio topbars and action consoles.

**Tech Stack:** React 19, TypeScript, Tailwind CSS v4, Lucide React icons, Vite, Go.

---

### File Map

| Action | File | Responsibility |
| --- | --- | --- |
| Modify | `frontend/src/components/Drawers.tsx` | Add `size?: 'md' \| 'lg' \| 'xl'` prop with width class mappings. |
| Modify | `frontend/src/App.tsx` | Decouple Settings from drawer to modal; set `size="xl"` for Codex and Graph drawers. |
| Modify | `frontend/src/components/SettingsStudio.tsx` | Add responsive flex-wrapping to sub-tab navigation and header controls. |
| Modify | `frontend/src/components/WorldsStudio.tsx` | In-memory entity draft store, non-gated entity switching, draft add/delete/save, modal shell, responsive header. |
| Modify | `frontend/src/components/LauncherHub.tsx` | Three-tier flexbox shell for New Campaign Wizard (`max-h-[85vh]`). |
| Modify | `frontend/src/components/SystemsStudio.tsx` | Responsive flex-wrapping on top header and action buttons. |
| Modify | `frontend/src/components/ActionConsole.tsx` | Responsive wrapping on mode switcher tabs, non-shrinking action buttons. |

---

### Task 1: Drawer Sizing System & Campaign Settings Modal

**Files:**
- Modify: `frontend/src/components/Drawers.tsx`
- Modify: `frontend/src/App.tsx`
- Test: `frontend/src/App.tsx`, `frontend/src/components/Drawers.tsx` via `npx tsc --noEmit`

- [ ] **Step 1: Update `Drawers.tsx` with size prop**
  Update `frontend/src/components/Drawers.tsx`:
  - Extend `DrawersProps` to include `size?: 'md' | 'lg' | 'xl'`.
  - Map `size` to Tailwind classes (`md: 'max-w-md'`, `lg: 'max-w-lg'`, `xl: 'max-w-xl'`). Default to `'md'`.
  - Ensure the drawer shell uses `sizeClasses[size || 'md']`.

  ```tsx
  interface DrawersProps {
    isOpen: boolean;
    onClose: () => void;
    title: string;
    children: React.ReactNode;
    size?: 'md' | 'lg' | 'xl';
  }

  const sizeClasses: Record<'md' | 'lg' | 'xl', string> = {
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
  };
  ```

- [ ] **Step 2: Update `App.tsx` to use Settings Modal and sized Drawers**
  In `frontend/src/App.tsx`:
  - Add state `const [isSettingsOpen, setIsSettingsOpen] = useState(false);`.
  - Change the header "Settings" button click handler to `onClick={() => setIsSettingsOpen(true)}` and active state `className={... isSettingsOpen ? 'bg-amber-600 ...' : ...}`.
  - Remove `<Drawers isOpen={activeDrawer === 'settings'} ...><SettingsStudio isCompact ... /></Drawers>`.
  - Set `size="xl"` on the `codex` and `graph` drawers:
    ```tsx
    <Drawers
      isOpen={activeDrawer === 'graph'}
      onClose={() => setActiveDrawer(null)}
      title="Knowledge Graph"
      size="xl"
    >
      <GraphDrawer data={graph || undefined} onSelectNode={handleOpenWikilink} />
    </Drawers>

    <Drawers
      isOpen={activeDrawer === 'codex'}
      onClose={() => setActiveDrawer(null)}
      title="Codex Entity Notes"
      size="xl"
    >
      <CodexDrawer
        entity={selectedEntity || undefined}
        onSave={handleSaveEntity}
      />
    </Drawers>
    ```
  - Mount the Settings modal dialog at the root level of the active campaign layout:
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
              className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors cursor-pointer"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
          <div className="flex-1 min-h-0 overflow-y-auto p-6">
            <SettingsStudio
              onSaved={() =>
                APIClient.getSettings()
                  .then((res) => setConfig(res.config))
                  .catch(console.error)
              }
            />
          </div>
        </div>
      </div>
    )}
    ```
  - Import `X` icon from `lucide-react`.

- [ ] **Step 3: Run TypeScript typecheck**
  Run: `mise run test:frontend`
  Expected: PASS (`npx tsc --noEmit` exits with 0).

- [ ] **Step 4: Commit changes**
  ```bash
  git add frontend/src/components/Drawers.tsx frontend/src/App.tsx
  git commit -m "feat(frontend): add drawer sizing and campaign settings modal"
  ```

---

### Task 2: SettingsStudio Responsive Header & Layout Polish

**Files:**
- Modify: `frontend/src/components/SettingsStudio.tsx`
- Test: `frontend/src/components/SettingsStudio.tsx` via `npx tsc --noEmit`

- [ ] **Step 1: Add responsive flex-wrapping in `SettingsStudio.tsx`**
  In `frontend/src/components/SettingsStudio.tsx`:
  - Locate the header at lines 142-195.
  - Update container:
    ```tsx
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-800 pb-3">
    ```
  - Wrap the tabs cluster in `flex flex-wrap items-center gap-1 bg-stone-950/70 p-1 rounded-xl border border-stone-800`.
  - Wrap the config file path and "Save Settings" button in `flex flex-wrap items-center gap-2`.
  - Ensure the AI agent preset selector and role dropdown cluster in the Agents subtab (lines 276-316) uses `flex flex-wrap items-center gap-2` so it wraps cleanly without clipping on narrower viewports.
  - In the Media tab (lines 541-587), ensure the TTS preset selector and Auto-play checkbox use `flex flex-wrap items-center gap-3`.

- [ ] **Step 2: Run TypeScript typecheck**
  Run: `mise run test:frontend`
  Expected: PASS.

- [ ] **Step 3: Commit changes**
  ```bash
  git add frontend/src/components/SettingsStudio.tsx
  git commit -m "fix(frontend): make settings studio header and control bars responsive"
  ```

---

### Task 3: World Studio In-Memory Entity Store & Switching

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`
- Test: `frontend/src/components/WorldsStudio.tsx` via `npx tsc --noEmit`

- [ ] **Step 1: Add `entityDrafts` in-memory store in `WorldsStudio.tsx`**
  In `frontend/src/components/WorldsStudio.tsx`:
  - Define `entityDrafts`:
    ```tsx
    const [entityDrafts, setEntityDrafts] = useState<Record<string, string>>(() => {
      const initial: Record<string, string> = {};
      REFERENCE_WORLD_TEMPLATE.entities.forEach((e) => {
        initial[e.id] = e.markdown;
      });
      return initial;
    });
    ```
  - In `handleNewWorld` and `handleResetToReference`:
    Reset `entityDrafts` to the initial reference template mapping:
    ```tsx
    const initial: Record<string, string> = {};
    REFERENCE_WORLD_TEMPLATE.entities.forEach((e) => {
      initial[e.id] = e.markdown;
    });
    setEntityDrafts(initial);
    ```

- [ ] **Step 2: Implement `handleSelectEntity` for non-gated switching**
  Implement `handleSelectEntity`:
  ```tsx
  const handleSelectEntity = async (targetId: string) => {
    // 1. Flush currently open editor content to draft store
    if (selectedEntityID) {
      setEntityDrafts((prev) => ({ ...prev, [selectedEntityID]: entityMarkdown }));
    }

    setSelectedEntityID(targetId);

    // 2. Check if we already have the markdown in drafts
    if (entityDrafts[targetId] !== undefined) {
      setEntityMarkdown(entityDrafts[targetId]);
      return;
    }

    // 3. If world is saved, fetch from backend and populate draft store
    if (selectedID) {
      try {
        const ent = await APIClient.getWorldEntity(selectedID, targetId);
        setEntityDrafts((prev) => ({ ...prev, [targetId]: ent.markdown }));
        setEntityMarkdown(ent.markdown);
      } catch (err: any) {
        setToast({ type: 'error', message: err.message || 'Failed to load entity markdown' });
      }
    } else {
      setEntityMarkdown('');
    }
  };
  ```
  Update line 546 in the entity template list:
  ```tsx
  <div
    key={e.id}
    onClick={() => handleSelectEntity(e.id)}
    className={`group p-2 rounded-lg border text-left cursor-pointer flex items-center justify-between transition-all ${
      selectedEntityID === e.id
        ? 'bg-amber-950/50 border-amber-500/60 text-amber-300'
        : 'bg-stone-900/40 border-stone-800/60 text-stone-300 hover:bg-stone-800'
    }`}
  >
  ```

- [ ] **Step 3: Update `handleCreateNewEntity`, `handleDeleteEntity`, and `handleSaveEntity`**
  - In `handleCreateNewEntity`:
    ```tsx
    const handleCreateNewEntity = async () => {
      if (!newEntitySlug.trim()) return;
      const slug = newEntitySlug.toLowerCase().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, '');

      if (!selectedID) {
        // Draft mode: add to in-memory state
        if (entities.some((e) => e.id === slug)) {
          setToast({ type: 'error', message: `Entity "${slug}" already exists` });
          return;
        }
        const newSummary: WorldEntitySummary = { id: slug, name: slug, type: 'concept' };
        setEntities((prev) => [...prev, newSummary]);
        setEntityDrafts((prev) => ({
          ...prev,
          ...(selectedEntityID ? { [selectedEntityID]: entityMarkdown } : {}),
          [slug]: STARTER_ENTITY_TEMPLATE,
        }));
        setSelectedEntityID(slug);
        setEntityMarkdown(STARTER_ENTITY_TEMPLATE);
        setIsNewEntityModal(false);
        setNewEntitySlug('');
        return;
      }

      // Saved world mode: call API
      try {
        await APIClient.saveWorldEntity(selectedID, slug, STARTER_ENTITY_TEMPLATE);
        setIsNewEntityModal(false);
        setNewEntitySlug('');
        await loadWorldDetail(selectedID);
        await handleSelectEntity(slug);
      } catch (err: any) {
        setToast({ type: 'error', message: err.message || 'Failed to create entity' });
      }
    };
    ```
  - In `handleDeleteEntity`:
    ```tsx
    const handleDeleteEntity = async (entityId: string) => {
      if (!selectedID) {
        // Draft mode: remove from state
        const remaining = entities.filter((e) => e.id !== entityId);
        setEntities(remaining);
        const updatedDrafts = { ...entityDrafts };
        delete updatedDrafts[entityId];
        setEntityDrafts(updatedDrafts);

        if (selectedEntityID === entityId) {
          const next = remaining[0];
          setSelectedEntityID(next ? next.id : null);
          setEntityMarkdown(next ? (updatedDrafts[next.id] || '') : '');
        }
        setToast({ type: 'success', message: `Entity "${entityId}" removed from draft.` });
        return;
      }

      // Saved mode: call API
      try {
        await APIClient.deleteWorldEntity(selectedID, entityId);
        setToast({ type: 'success', message: `Entity "${entityId}" deleted!` });
        await loadWorldDetail(selectedID);
      } catch (err: any) {
        setToast({ type: 'error', message: err.message || 'Failed to delete entity' });
      }
    };
    ```
  - In `handleSaveEntity`:
    ```tsx
    const handleSaveEntity = async () => {
      if (!selectedEntityID) return;

      // Update in-memory drafts
      setEntityDrafts((prev) => ({ ...prev, [selectedEntityID]: entityMarkdown }));

      if (!selectedID) {
        setToast({ type: 'success', message: `Draft entity "${selectedEntityID}" updated! (Will be saved when you click Save World)` });
        return;
      }

      try {
        await APIClient.saveWorldEntity(selectedID, selectedEntityID, entityMarkdown);
        setToast({ type: 'success', message: `Entity "${selectedEntityID}" saved!` });
        await loadWorldDetail(selectedID);
      } catch (err: any) {
        setToast({ type: 'error', message: err.message || 'Failed to save entity' });
      }
    };
    ```

- [ ] **Step 4: Update `handleSaveWorld` to persist all drafted and modified entities**
  In `handleSaveWorld`:
  ```tsx
  const saved = await APIClient.saveWorld(payload);

  // Sync all draft entity markdowns to disk
  const allDrafts = { ...entityDrafts };
  if (selectedEntityID) {
    allDrafts[selectedEntityID] = entityMarkdown;
  }

  for (const ent of entities) {
    const md = allDrafts[ent.id] || STARTER_ENTITY_TEMPLATE;
    await APIClient.saveWorldEntity(saved.id, ent.id, md).catch(console.error);
  }

  setToast({ type: 'success', message: `World "${saved.name}" and templates saved successfully!` });
  await loadWorlds(saved.id);
  if (onWorldSaved) onWorldSaved();
  ```

- [ ] **Step 5: Update Topbar & New Entity Modal Shell in `WorldsStudio.tsx`**
  - Update top header:
    ```tsx
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-800/80 pb-3">
      <div className="flex items-center gap-3">...</div>
      <div className="flex flex-wrap items-center gap-2">...</div>
    </div>
    ```
  - In Tab 3:
    Ensure the template list has `w-56 shrink-0` and markdown container has `flex-1 min-w-0`.
  - Refactor `isNewEntityModal` into three-tier flexbox shell:
    ```tsx
    {isNewEntityModal && (
      <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-fade-in">
        <div className="w-full max-w-sm max-h-[85vh] flex flex-col rounded-2xl bg-stone-900 border border-amber-500/30 shadow-2xl overflow-hidden">
          <div className="px-6 py-4 border-b border-stone-800 shrink-0">
            <h3 className="font-cinzel text-sm font-bold text-amber-400">
              New Starter Entity Template
            </h3>
          </div>
          <div className="p-6 space-y-4 flex-1 overflow-y-auto">
            <div className="space-y-1">
              <label className="text-xs text-stone-300">Entity Slug (filename without .md)</label>
              <input
                type="text"
                placeholder="e.g. the_iron_bastion"
                value={newEntitySlug}
                onChange={(e) => setNewEntitySlug(e.target.value)}
                className="w-full bg-stone-950 border border-stone-800 rounded-xl px-3 py-2 text-xs text-stone-100 font-mono focus:outline-none focus:border-amber-500/60"
              />
            </div>
          </div>
          <div className="flex items-center justify-end gap-2 px-6 py-3 border-t border-stone-800 shrink-0 bg-stone-900/80">
            <button
              type="button"
              onClick={() => setIsNewEntityModal(false)}
              className="px-3 py-1.5 text-xs text-stone-400 hover:text-white cursor-pointer"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={handleCreateNewEntity}
              disabled={!newEntitySlug.trim()}
              className="px-4 py-1.5 text-xs font-cinzel font-bold bg-amber-600 text-stone-950 rounded-lg disabled:opacity-50 cursor-pointer"
            >
              Create
            </button>
          </div>
        </div>
      </div>
    )}
    ```

- [ ] **Step 6: Run TypeScript typecheck**
  Run: `mise run test:frontend`
  Expected: PASS.

- [ ] **Step 7: Commit changes**
  ```bash
  git add frontend/src/components/WorldsStudio.tsx
  git commit -m "feat(frontend): fix world studio entity switching and add in-memory drafts"
  ```

---

### Task 4: Campaign Wizard Modal Shell & Studio Headers

**Files:**
- Modify: `frontend/src/components/LauncherHub.tsx`
- Modify: `frontend/src/components/SystemsStudio.tsx`
- Modify: `frontend/src/components/ActionConsole.tsx`
- Test: All via `npx tsc --noEmit`

- [ ] **Step 1: Refactor New Campaign Wizard in `LauncherHub.tsx`**
  In `frontend/src/components/LauncherHub.tsx`:
  - Locate `isWizardOpen` modal (lines 323-485).
  - Convert dialog to the three-tier shell:
    ```tsx
    {isWizardOpen && (
      <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-fade-in">
        <div className="relative w-full max-w-lg max-h-[85vh] flex flex-col rounded-2xl bg-stone-900/95 border border-amber-500/30 shadow-2xl overflow-hidden">
          {/* Fixed Header */}
          <div className="flex items-center justify-between border-b border-stone-800 px-6 py-4 shrink-0">
            <div className="flex items-center gap-2">
              <Sparkles className="w-5 h-5 text-amber-400" />
              <h3 className="font-cinzel text-lg font-bold text-amber-400">
                New Campaign Wizard
              </h3>
            </div>
            <button
              onClick={() => setIsWizardOpen(false)}
              className="text-stone-400 hover:text-white p-1 rounded-lg hover:bg-stone-800 transition-colors cursor-pointer"
            >
              <X className="w-5 h-5" />
            </button>
          </div>

          {/* Form with scrollable body and pinned footer */}
          <form onSubmit={handleCreateGame} className="flex-1 min-h-0 flex flex-col overflow-hidden">
            <div className="flex-1 overflow-y-auto p-6 space-y-4">
              {/* Setup Required banner */}
              {(!systems.length || !worlds.length) && ( ... )}

              {/* Campaign Title */}
              {/* Rule System & World Setting Grid */}
              {/* Selected World Description */}
              {/* Protagonist Name */}
            </div>

            {/* Pinned Action Footer */}
            <div className="flex items-center justify-end gap-3 border-t border-stone-800 px-6 py-4 shrink-0 bg-stone-900/90">
              <button
                type="button"
                onClick={() => setIsWizardOpen(false)}
                className="px-4 py-2 text-xs font-cinzel text-stone-400 hover:text-white transition-colors cursor-pointer"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={
                  isSubmitting ||
                  !newGameName.trim() ||
                  !newPlayerName.trim() ||
                  systems.length === 0 ||
                  worlds.length === 0
                }
                className="flex items-center gap-2 text-xs font-cinzel font-bold px-5 py-2.5 rounded-xl bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-stone-950 shadow-lg transition-all cursor-pointer"
              >
                <Sparkles className="w-3.5 h-3.5" />
                <span>{isSubmitting ? 'Weaving World...' : 'Embark on Adventure'}</span>
              </button>
            </div>
          </form>
        </div>
      </div>
    )}
    ```

- [ ] **Step 2: Update topbar in `SystemsStudio.tsx`**
  In `frontend/src/components/SystemsStudio.tsx`:
  - Update top header:
    ```tsx
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-800/80 pb-3">
      <div className="flex items-center gap-3">
        <h2 className="font-cinzel text-lg font-bold text-amber-400">
          {selectedID ? name || 'Edit System' : 'Create New System'}
        </h2>
        {slugID && (
          <span className="text-xs font-mono text-stone-400 bg-stone-950 px-2 py-0.5 rounded border border-stone-800">
            systems/{slugID}
          </span>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex bg-stone-950/80 p-1 rounded-xl border border-stone-800">
          ...
        </div>
        ...
      </div>
    </div>
    ```

- [ ] **Step 3: Update `ActionConsole.tsx` responsive wrapping**
  In `frontend/src/components/ActionConsole.tsx`:
  - Change line 32 mode switcher container to `flex flex-wrap items-center gap-2 mb-3`.
  - In line 80 form container, ensure the text input has `min-w-0 flex-1` and buttons have `shrink-0`.

- [ ] **Step 4: Run TypeScript typecheck**
  Run: `mise run test:frontend`
  Expected: PASS.

- [ ] **Step 5: Commit changes**
  ```bash
  git add frontend/src/components/LauncherHub.tsx frontend/src/components/SystemsStudio.tsx frontend/src/components/ActionConsole.tsx
  git commit -m "fix(frontend): add pinned modal shell and responsive studio layouts"
  ```

---

### Task 5: Full Build & Verification

**Files:**
- None (verification across all touched frontend and backend components)

- [ ] **Step 1: Run frontend test check**
  Run: `mise run test:frontend`
  Expected: `npx tsc --noEmit` exits with 0.

- [ ] **Step 2: Run backend tests**
  Run: `mise run test:backend`
  Expected: All Go packages pass with 0 failures.

- [ ] **Step 3: Run full production build**
  Run: `mise run build`
  Expected: Vite compiles bundle into `pkg/gui/dist/`, Go compiles binary `bin/localrpg` without error.

- [ ] **Step 4: Verify git status and tracked files**
  Run: `git status`
  Expected: Working tree clean (ignoring tracked `.gitkeep` as documented in AGENTS.md).
